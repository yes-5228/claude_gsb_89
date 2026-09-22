package acceptance

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/shared/date"
)

// EnsureDefaultTemplate 保证系统至少存在一版评分模板。
//
// 在数据库迁移后执行：全新环境与从旧版本升级的环境都会补齐默认模板。
// 已存在模板版本（含用户自定义版本）时直接返回，不重复写入。
func (s *Service) EnsureDefaultTemplate(ctx context.Context) error {
	count, err := s.repo.CountTemplates(ctx)
	if err != nil {
		return httpx.WrapInternal("查询评分模板失败", err)
	}
	if count > 0 {
		return nil
	}

	effectiveFrom := date.MustParse(DefaultTemplateEffectiveFrom)
	version := &ScoreTemplateVersion{EffectiveFrom: effectiveFrom, Remark: "系统默认评分模板"}
	items := DefaultTemplateItems()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		exists, err := s.hasTemplateEffectiveOnInTx(ctx, tx, effectiveFrom)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		return s.repo.CreateTemplateInTx(ctx, tx, version, items)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			// 并发启动等场景下另一进程已写入，视为幂等成功。
			return nil
		}
		return httpx.WrapInternal("初始化默认评分模板失败", err)
	}
	return nil
}

// ListTemplates 列出全部评分模板版本（按生效日期倒序），并标注当前生效版本。
func (s *Service) ListTemplates(ctx context.Context) ([]TemplateResponse, error) {
	versions, err := s.repo.ListTemplates(ctx)
	if err != nil {
		return nil, httpx.WrapInternal("查询评分模板失败", err)
	}
	current, err := s.repo.EffectiveTemplate(ctx, date.Today())
	if err != nil {
		return nil, httpx.WrapInternal("查询当前评分模板失败", err)
	}
	currentID := uint(0)
	if current != nil {
		currentID = current.ID
	}

	result := make([]TemplateResponse, 0, len(versions))
	for i := range versions {
		version := versions[i]
		items, err := s.repo.TemplateItems(ctx, version.ID)
		if err != nil {
			return nil, httpx.WrapInternal("查询评分项失败", err)
		}
		result = append(result, TemplateResponse{
			ScoreTemplateVersion: version,
			Items:                items,
			Active:               version.ID == currentID,
		})
	}
	return result, nil
}

// EffectiveTemplate 查询指定日期（默认今天）适用的评分模板。
func (s *Service) EffectiveTemplate(ctx context.Context, day date.Date) (*EffectiveTemplateResponse, error) {
	version, err := s.repo.EffectiveTemplate(ctx, day)
	if err != nil {
		return nil, httpx.WrapInternal("查询生效评分模板失败", err)
	}
	if version == nil {
		return nil, httpx.InvalidState("该验收日期还没有生效的评分模板，请先在评分项设置中发布模板版本")
	}
	items, err := s.repo.TemplateItems(ctx, version.ID)
	if err != nil {
		return nil, httpx.WrapInternal("查询评分项失败", err)
	}
	return &EffectiveTemplateResponse{EffectiveFrom: version.EffectiveFrom, Items: items}, nil
}

// CreateTemplateVersion 发布新的评分模板版本（调整评分项）。
//
// 模板版本一经创建不可修改：调整评分项只能新增版本，生效日期相同会冲突；
// 版本互不覆盖，已登记验收记录引用的历史版本与评分快照保持不变。
func (s *Service) CreateTemplateVersion(ctx context.Context, req CreateTemplateRequest) (*TemplateResponse, error) {
	if err := validateTemplateRequest(req); err != nil {
		return nil, err
	}
	if exists, err := s.repo.HasTemplateEffectiveOn(ctx, req.EffectiveFrom); err != nil {
		return nil, httpx.WrapInternal("校验评分模板生效日期失败", err)
	} else if exists {
		return nil, httpx.Conflict("该生效日期已存在评分模板版本，模板不可修改，请换一个生效日期发布新版本")
	}

	version := &ScoreTemplateVersion{
		EffectiveFrom: req.EffectiveFrom,
		Remark:        strings.TrimSpace(req.Remark),
	}
	items := make([]ScoreTemplateItem, 0, len(req.Items))
	for _, input := range req.Items {
		items = append(items, ScoreTemplateItem{
			Name:      strings.TrimSpace(input.Name),
			MaxScore:  input.MaxScore,
			SortOrder: input.SortOrder,
			Criteria:  strings.TrimSpace(input.Criteria),
		})
	}

	if err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		return s.repo.CreateTemplateInTx(ctx, tx, version, items)
	}); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, httpx.Conflict("该生效日期已存在评分模板版本，或评分项排序号重复")
		}
		return nil, httpx.WrapInternal("发布评分模板版本失败", err)
	}
	return &TemplateResponse{ScoreTemplateVersion: *version, Items: items}, nil
}

// hasTemplateEffectiveOnInTx 事务内判断指定日期是否已有模板版本。
func (s *Service) hasTemplateEffectiveOnInTx(ctx context.Context, tx *gorm.DB, day date.Date) (bool, error) {
	var count int64
	err := tx.WithContext(ctx).Model(&ScoreTemplateVersion{}).
		Where("effective_from = ?", day.Time).
		Count(&count).Error
	return count > 0, err
}

// validateTemplateRequest 校验模板版本：评分项名称、排序号不重复，分值上限合计 100。
func validateTemplateRequest(req CreateTemplateRequest) error {
	if req.EffectiveFrom.IsZero() {
		return httpx.Validation("生效日期不能为空")
	}
	if len(req.Items) == 0 {
		return httpx.Validation("至少需要一个评分项")
	}
	names := make(map[string]struct{}, len(req.Items))
	orders := make(map[int]struct{}, len(req.Items))
	total := 0
	for _, item := range req.Items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return httpx.Validation("评分项名称不能为空")
		}
		if _, ok := names[name]; ok {
			return httpx.Validation("评分项名称不能重复：" + name)
		}
		names[name] = struct{}{}
		if _, ok := orders[item.SortOrder]; ok {
			return httpx.Validation("评分项排序号不能重复，请检查「" + name + "」")
		}
		orders[item.SortOrder] = struct{}{}
		if item.MaxScore < 1 {
			return httpx.Validation("评分项「" + name + "」的分值上限必须大于 0")
		}
		total += item.MaxScore
	}
	if total != 100 {
		return httpx.Validation("各评分项分值上限合计必须为 100 分，当前合计为 " + strconv.Itoa(total) + " 分")
	}
	return nil
}
