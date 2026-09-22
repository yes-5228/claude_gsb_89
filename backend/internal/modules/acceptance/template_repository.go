package acceptance

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/shared/date"
)

// ---------- 评分模板版本 ----------

// CountTemplates 统计评分模板版本数量，用于默认模板幂等初始化。
func (r *Repository) CountTemplates(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ScoreTemplateVersion{}).Count(&count).Error
	return count, err
}

// CreateTemplateInTx 在事务中创建模板版本及其评分项。
func (r *Repository) CreateTemplateInTx(
	ctx context.Context,
	tx *gorm.DB,
	version *ScoreTemplateVersion,
	items []ScoreTemplateItem,
) error {
	if err := tx.WithContext(ctx).Create(version).Error; err != nil {
		return err
	}
	for i := range items {
		items[i].TemplateID = version.ID
	}
	if len(items) > 0 {
		if err := tx.WithContext(ctx).Create(&items).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListTemplates 按生效日期倒序列出全部模板版本。
func (r *Repository) ListTemplates(ctx context.Context) ([]ScoreTemplateVersion, error) {
	versions := make([]ScoreTemplateVersion, 0)
	err := r.db.WithContext(ctx).
		Order("effective_from DESC, id DESC").
		Find(&versions).Error
	return versions, err
}

// EffectiveTemplate 查询验收日期当天适用的模板版本：
// 生效日期不晚于该日期的最新一版。不存在时返回 nil。
func (r *Repository) EffectiveTemplate(ctx context.Context, day date.Date) (*ScoreTemplateVersion, error) {
	var version ScoreTemplateVersion
	err := r.db.WithContext(ctx).
		Where("effective_from <= ?", day.Time).
		Order("effective_from DESC, id DESC").
		First(&version).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &version, nil
}

// FindTemplate 按主键查询模板版本。
func (r *Repository) FindTemplate(ctx context.Context, id uint) (*ScoreTemplateVersion, error) {
	var version ScoreTemplateVersion
	err := r.db.WithContext(ctx).First(&version, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &version, nil
}

// TemplateItems 查询某版本下的全部评分项（按排序号升序）。
func (r *Repository) TemplateItems(ctx context.Context, templateID uint) ([]ScoreTemplateItem, error) {
	items := make([]ScoreTemplateItem, 0)
	err := r.db.WithContext(ctx).
		Where("template_id = ?", templateID).
		Order("sort_order ASC, id ASC").
		Find(&items).Error
	return items, err
}

// HasTemplateEffectiveOn 判断是否已存在指定日期生效的模板版本。
func (r *Repository) HasTemplateEffectiveOn(ctx context.Context, day date.Date) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ScoreTemplateVersion{}).
		Where("effective_from = ?", day.Time).
		Count(&count).Error
	return count > 0, err
}

// ---------- 验收评分项快照 ----------

// CreateScoreItemsInTx 在事务中批量写入评分项快照。
func (r *Repository) CreateScoreItemsInTx(ctx context.Context, tx *gorm.DB, items []AcceptanceScoreItem) error {
	if len(items) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Create(&items).Error
}

// DeleteScoreItemsInTx 在事务中删除某验收记录的评分项快照。
func (r *Repository) DeleteScoreItemsInTx(ctx context.Context, tx *gorm.DB, acceptanceID uint) error {
	return tx.WithContext(ctx).
		Where("acceptance_id = ?", acceptanceID).
		Delete(&AcceptanceScoreItem{}).Error
}

// ScoreItemsByAcceptance 查询单条验收记录的评分项快照。
func (r *Repository) ScoreItemsByAcceptance(ctx context.Context, acceptanceID uint) ([]AcceptanceScoreItem, error) {
	items := make([]AcceptanceScoreItem, 0)
	err := r.db.WithContext(ctx).
		Where("acceptance_id = ?", acceptanceID).
		Order("sort_order ASC, id ASC").
		Find(&items).Error
	return items, err
}

// ScoreItemsByAcceptances 批量查询多条验收记录的评分项快照，按验收 ID 分组，避免列表 N+1。
func (r *Repository) ScoreItemsByAcceptances(ctx context.Context, acceptanceIDs []uint) (map[uint][]AcceptanceScoreItem, error) {
	result := make(map[uint][]AcceptanceScoreItem, len(acceptanceIDs))
	if len(acceptanceIDs) == 0 {
		return result, nil
	}
	items := make([]AcceptanceScoreItem, 0)
	err := r.db.WithContext(ctx).
		Where("acceptance_id IN ?", acceptanceIDs).
		Order("sort_order ASC, id ASC").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	for i := range items {
		result[items[i].AcceptanceID] = append(result[items[i].AcceptanceID], items[i])
	}
	return result, nil
}
