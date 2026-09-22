package acceptance

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/modules/cleaningtask"
	"github.com/drainage/desilting/internal/modules/pipesegment"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/shared/option"
	"github.com/drainage/desilting/internal/shared/refx"
)

// 验收评分与结论的一致性要求：合格的任务评分不应低于该阈值。
const passScoreThreshold = 60

// TaskGateway 清淤任务模块对外提供的能力（由 cleaningtask.Service 实现）。
type TaskGateway interface {
	FindByID(ctx context.Context, id uint) (*cleaningtask.CleaningTask, error)
	TransitionInTx(ctx context.Context, tx *gorm.DB, id uint, from, to string, extra map[string]any) error
}

// SegmentGateway 管段台账对外提供的能力（由 pipesegment.Service 实现）。
type SegmentGateway interface {
	FindByID(ctx context.Context, id uint) (*pipesegment.PipeSegment, error)
	MarkCleaned(ctx context.Context, tx *gorm.DB, segmentID uint, cleanedAt date.Date) error
}

// RecordGateway 清淤记录模块对外提供的能力（由 cleaningrecord.Service 实现）。
type RecordGateway interface {
	TotalsByTask(ctx context.Context, taskID uint) (refx.RecordTotals, error)
}

// Service 验收业务逻辑。
type Service struct {
	repo     *Repository
	tasks    TaskGateway
	segments SegmentGateway
	records  RecordGateway
}

// NewService 构造服务。
func NewService(repo *Repository, tasks TaskGateway, segments SegmentGateway, records RecordGateway) *Service {
	return &Service{repo: repo, tasks: tasks, segments: segments, records: records}
}

// Create 登记验收记录。
//
// 验收合格会把任务推进到"已验收"并回写管段清淤统计；
// 验收需整改会把任务退回"清淤中"，等待整改完成后复验。
// 记录的写入与任务、管段的联动更新在同一个事务内完成。
func (s *Service) Create(ctx context.Context, req SaveRequest) (*AcceptanceRecord, error) {
	task, err := s.tasks.FindByID(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	if task.Status != cleaningtask.StatusCompleted {
		return nil, httpx.InvalidState(fmt.Sprintf(
			"任务当前状态为「%s」，只有待验收的任务才能登记验收", cleaningtask.StatusLabel(task.Status),
		))
	}

	totals, err := s.records.TotalsByTask(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	if totals.RecordCount == 0 {
		return nil, httpx.InvalidState("该任务还没有清淤记录，不能登记验收")
	}

	previous, err := s.repo.LatestByTask(ctx, req.TaskID)
	if err != nil {
		return nil, httpx.WrapInternal("查询历史验收记录失败", err)
	}
	if previous != nil {
		if previous.Result == ResultPass {
			return nil, httpx.InvalidState(fmt.Sprintf(
				"该任务已由 %s 于 %s 验收合格，无需重复验收",
				previous.InspectorName, previous.AcceptedAt.String(),
			))
		}
		if previous.RectifiedAt == nil {
			return nil, httpx.InvalidState("上一次验收结论为需整改且尚未登记整改完成，请先完成整改再复验")
		}
	}

	if err := validate(req); err != nil {
		return nil, err
	}
	if req.CleaningRecordID != nil {
		belongs, err := s.repo.CleaningRecordBelongsToTask(ctx, *req.CleaningRecordID, req.TaskID)
		if err != nil {
			return nil, httpx.WrapInternal("校验清淤记录失败", err)
		}
		if !belongs {
			return nil, httpx.BadRequest("关联的清淤记录不属于该任务")
		}
	}

	// 按验收日期解析适用的评分模板并逐项打分，总分由各评分项自动汇总。
	template, templateItems, scoreItems, total, err := s.resolveScoreItems(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := validateScoreConsistency(req.Result, total); err != nil {
		return nil, err
	}

	targetStatus := cleaningtask.StatusAccepted
	if req.Result == ResultRework {
		targetStatus = cleaningtask.StatusInProgress
	}
	if !cleaningtask.CanTransition(task.Status, targetStatus) {
		return nil, cleaningtask.TransitionError(task.Status, targetStatus)
	}

	record := &AcceptanceRecord{}
	apply(req, record)
	record.Score = total
	templateID := template.ID
	record.ScoreTemplateID = &templateID

	for attempt := 0; attempt < 5; attempt++ {
		record.Code = s.nextCode(ctx, record.AcceptedAt)
		err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
			if err := s.repo.CreateInTx(ctx, tx, record); err != nil {
				return err
			}
			for i := range scoreItems {
				scoreItems[i].ID = 0
				scoreItems[i].AcceptanceID = record.ID
			}
			if err := s.repo.CreateScoreItemsInTx(ctx, tx, scoreItems); err != nil {
				return err
			}
			return s.applyOutcome(ctx, tx, task, record, totals)
		})
		if err == nil {
			return record, nil
		}
		var appErr *httpx.AppError
		if errors.As(err, &appErr) {
			return nil, err
		}
		if !errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, httpx.WrapInternal("登记验收记录失败", err)
		}
	}
	return nil, httpx.Conflict("验收编号生成冲突，请稍后重试")
}

// Rectify 登记整改完成，之后任务可以重新报验。
func (s *Service) Rectify(ctx context.Context, id uint, req RectifyRequest) (*AcceptanceRecord, error) {
	record, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	if record.Result == ResultPass {
		return nil, httpx.InvalidState("该验收结论为合格，无需登记整改")
	}
	if record.RectifiedAt != nil {
		return nil, httpx.InvalidState("该验收记录已登记过整改完成，请勿重复登记")
	}

	rectifiedAt := req.RectifiedAt
	if rectifiedAt.IsZero() {
		rectifiedAt = date.Today()
	}
	if rectifiedAt.After(date.Today()) {
		return nil, httpx.Validation("整改完成日期不能晚于今天")
	}
	if rectifiedAt.Before(record.AcceptedAt) {
		return nil, httpx.Validation("整改完成日期不能早于验收日期")
	}

	rectification := strings.TrimSpace(req.Rectification)
	if record.RectifyDeadline != nil && rectifiedAt.After(*record.RectifyDeadline) && rectification == "" {
		return nil, httpx.Validation("整改已超过整改期限，请填写整改情况说明")
	}

	record.RectifiedAt = &rectifiedAt
	if rectification != "" {
		record.Rectification = rectification
	}
	if remark := strings.TrimSpace(req.Remark); remark != "" {
		record.Remark = remark
	}
	if err := s.repo.Save(ctx, record); err != nil {
		return nil, httpx.WrapInternal("登记整改完成失败", err)
	}
	return record, nil
}

// Delete 删除验收记录。验收结论为合格或任务已验收的，不允许删除。
func (s *Service) Delete(ctx context.Context, id uint) error {
	record, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return notFound(err)
	}
	if record.Result == ResultPass {
		return httpx.InvalidState("验收结论为合格的记录不允许删除")
	}
	task, err := s.tasks.FindByID(ctx, record.TaskID)
	if err != nil {
		return err
	}
	if task.Status == cleaningtask.StatusAccepted {
		return httpx.InvalidState("任务已验收合格，不能再删除验收记录")
	}
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if err := s.repo.DeleteScoreItemsInTx(ctx, tx, id); err != nil {
			return err
		}
		result := tx.WithContext(ctx).Delete(&AcceptanceRecord{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return notFound(err)
	}
	return nil
}

// FindByID 查询验收记录。
func (s *Service) FindByID(ctx context.Context, id uint) (*AcceptanceRecord, error) {
	record, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	return record, nil
}

// List 分页查询验收记录。
func (s *Service) List(ctx context.Context, query ListQuery) ([]ListItem, int64, error) {
	records, total, err := s.repo.List(ctx, query)
	if err != nil {
		return nil, 0, httpx.WrapInternal("查询验收记录失败", err)
	}
	if len(records) == 0 {
		return []ListItem{}, total, nil
	}

	taskIDs := make([]uint, 0, len(records))
	acceptanceIDs := make([]uint, 0, len(records))
	for i := range records {
		taskIDs = append(taskIDs, records[i].TaskID)
		acceptanceIDs = append(acceptanceIDs, records[i].ID)
	}
	briefs, err := refx.TaskBriefsByIDs(ctx, s.repo.DB(), taskIDs)
	if err != nil {
		return nil, 0, httpx.WrapInternal("查询任务信息失败", err)
	}
	scoreItems, err := s.repo.ScoreItemsByAcceptances(ctx, acceptanceIDs)
	if err != nil {
		return nil, 0, httpx.WrapInternal("查询评分项明细失败", err)
	}

	items := make([]ListItem, 0, len(records))
	for i := range records {
		record := records[i]
		item := ListItem{
			AcceptanceRecord: record,
			ScoreItems:       scoreItems[record.ID],
		}
		if item.ScoreItems == nil {
			item.ScoreItems = []AcceptanceScoreItem{}
		}
		if brief, ok := briefs[record.TaskID]; ok {
			item.Task = &brief
		}
		items = append(items, item)
	}
	return items, total, nil
}

// Detail 验收详情。
func (s *Service) Detail(ctx context.Context, id uint) (*DetailResponse, error) {
	record, err := s.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	detail := &DetailResponse{Acceptance: record}
	scoreItems, err := s.repo.ScoreItemsByAcceptance(ctx, record.ID)
	if err != nil {
		return nil, httpx.WrapInternal("查询评分项明细失败", err)
	}
	if scoreItems == nil {
		scoreItems = []AcceptanceScoreItem{}
	}
	detail.ScoreItems = scoreItems
	if record.ScoreTemplateID != nil {
		template, err := s.repo.FindTemplate(ctx, *record.ScoreTemplateID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, httpx.WrapInternal("查询评分模板失败", err)
		}
		detail.Template = template
	}
	briefs, err := refx.TaskBriefsByIDs(ctx, s.repo.DB(), []uint{record.TaskID})
	if err != nil {
		return nil, httpx.WrapInternal("查询任务信息失败", err)
	}
	if brief, ok := briefs[record.TaskID]; ok {
		detail.Task = &brief
	}
	totals, err := s.records.TotalsByTask(ctx, record.TaskID)
	if err != nil {
		return nil, err
	}
	detail.RecordTotals = totals
	return detail, nil
}

// CountByResult 按结论统计（供看板使用）。
func (s *Service) CountByResult(ctx context.Context) (map[string]int64, error) {
	counts, err := s.repo.CountByResult(ctx)
	if err != nil {
		return nil, httpx.WrapInternal("统计验收结论失败", err)
	}
	return counts, nil
}

// CountPendingRectify 统计待整改数量（供看板使用）。
func (s *Service) CountPendingRectify(ctx context.Context) (int64, error) {
	count, err := s.repo.CountPendingRectify(ctx)
	if err != nil {
		return 0, httpx.WrapInternal("统计待整改数量失败", err)
	}
	return count, nil
}

// ScoreSummary 验收总分汇总（供看板使用）。
func (s *Service) ScoreSummary(ctx context.Context) (refx.AcceptanceScoreSummary, error) {
	summary, err := s.repo.ScoreSummary(ctx)
	if err != nil {
		return refx.AcceptanceScoreSummary{}, httpx.WrapInternal("统计验收总分失败", err)
	}
	return summary, nil
}

// ItemScoreStats 评分项得分统计（供看板使用）。
func (s *Service) ItemScoreStats(ctx context.Context) ([]refx.AcceptanceItemScoreStat, error) {
	stats, err := s.repo.ItemScoreStats(ctx)
	if err != nil {
		return nil, httpx.WrapInternal("统计评分项得分失败", err)
	}
	return stats, nil
}

// resolveScoreItems 按验收日期解析适用模板，并把请求中的逐项打分校验、换算成快照。
//
// 返回的 scoreItems 已按模板排序号排好、AcceptanceID 留空（在事务中补写）。
// 总分 = 各评分项实际得分之和。
func (s *Service) resolveScoreItems(
	ctx context.Context,
	req SaveRequest,
) (*ScoreTemplateVersion, []ScoreTemplateItem, []AcceptanceScoreItem, int, error) {
	template, err := s.repo.EffectiveTemplate(ctx, req.AcceptedAt)
	if err != nil {
		return nil, nil, nil, 0, httpx.WrapInternal("查询生效评分模板失败", err)
	}
	if template == nil {
		return nil, nil, nil, 0, httpx.InvalidState("该验收日期还没有生效的评分模板，请先在评分项设置中发布模板版本")
	}
	templateItems, err := s.repo.TemplateItems(ctx, template.ID)
	if err != nil {
		return nil, nil, nil, 0, httpx.WrapInternal("查询评分项失败", err)
	}

	if len(req.ScoreItems) != len(templateItems) {
		return nil, nil, nil, 0, httpx.BadRequest(fmt.Sprintf(
			"评分项明细不完整：适用模板共 %d 个评分项，本次提交 %d 个", len(templateItems), len(req.ScoreItems),
		))
	}

	byName := make(map[string]ScoreItemInput, len(req.ScoreItems))
	for _, input := range req.ScoreItems {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return nil, nil, nil, 0, httpx.Validation("评分项名称不能为空")
		}
		if _, ok := byName[name]; ok {
			return nil, nil, nil, 0, httpx.Validation("评分项明细存在重复项：" + name)
		}
		byName[name] = input
	}

	scoreItems := make([]AcceptanceScoreItem, 0, len(templateItems))
	total := 0
	for _, def := range templateItems {
		input, ok := byName[def.Name]
		if !ok {
			return nil, nil, nil, 0, httpx.BadRequest("缺少评分项「" + def.Name + "」的打分结果")
		}
		if input.ActualScore == nil {
			return nil, nil, nil, 0, httpx.Validation("评分项「" + def.Name + "」的实际得分不能为空")
		}
		actual := *input.ActualScore
		if actual < 0 || actual > def.MaxScore {
			return nil, nil, nil, 0, httpx.Validation(fmt.Sprintf(
				"评分项「%s」的实际得分必须在 0 ~ %d 分之间", def.Name, def.MaxScore,
			))
		}
		deduction := def.MaxScore - actual
		reason := strings.TrimSpace(input.DeductionReason)
		if deduction > 0 && reason == "" {
			return nil, nil, nil, 0, httpx.Validation(fmt.Sprintf(
				"评分项「%s」扣减了 %d 分，必须填写扣分说明", def.Name, deduction,
			))
		}
		if deduction == 0 && reason != "" {
			return nil, nil, nil, 0, httpx.Validation(fmt.Sprintf(
				"评分项「%s」得分为满分时不允许填写扣分说明", def.Name,
			))
		}
		scoreItems = append(scoreItems, AcceptanceScoreItem{
			TemplateID:      template.ID,
			Name:            def.Name,
			MaxScore:        def.MaxScore,
			ActualScore:     actual,
			Deduction:       deduction,
			DeductionReason: reason,
			SortOrder:       def.SortOrder,
		})
		total += actual
	}
	return template, templateItems, scoreItems, total, nil
}

// validateScoreConsistency 校验验收结论与自动汇总总分是否一致：
// 总分低于合格线只能判需整改；合格要求总分不低于合格线。
func validateScoreConsistency(result string, total int) error {
	if result == ResultPass && total < passScoreThreshold {
		return httpx.Validation(fmt.Sprintf(
			"评分项自动汇总总分为 %d 分，低于合格线 %d 分，验收结论只能选择「需整改」", total, passScoreThreshold,
		))
	}
	if result == ResultRework && total >= passScoreThreshold {
		return httpx.Validation(fmt.Sprintf(
			"评分项自动汇总总分为 %d 分，已达到合格线 %d 分，结论不能选择「需整改」，请核对扣分", total, passScoreThreshold,
		))
	}
	return nil
}

// applyOutcome 根据验收结论联动更新任务状态，验收合格时同步回写管段清淤统计。
func (s *Service) applyOutcome(
	ctx context.Context,
	tx *gorm.DB,
	task *cleaningtask.CleaningTask,
	record *AcceptanceRecord,
	totals refx.RecordTotals,
) error {
	if record.Result == ResultRework {
		return s.tasks.TransitionInTx(ctx, tx, task.ID,
			cleaningtask.StatusCompleted, cleaningtask.StatusInProgress,
			map[string]any{"finished_at": nil, "accepted_at": nil})
	}

	acceptedTime := time.Now()
	if !record.AcceptedAt.IsZero() {
		acceptedTime = record.AcceptedAt.Time
	}
	if err := s.tasks.TransitionInTx(ctx, tx, task.ID,
		cleaningtask.StatusCompleted, cleaningtask.StatusAccepted,
		map[string]any{"accepted_at": acceptedTime}); err != nil {
		return err
	}

	cleanedAt := totals.LatestCleanedAt
	if cleanedAt.IsZero() {
		cleanedAt = record.AcceptedAt
	}
	return s.segments.MarkCleaned(ctx, tx, task.PipeSegmentID, cleanedAt)
}

// validate 校验验收字段，并保证验收结论与评分、整改要求相互一致。
func validate(req SaveRequest) error {
	if req.AcceptedAt.IsZero() {
		return httpx.Validation("验收日期不能为空")
	}
	if req.AcceptedAt.After(date.Today()) {
		return httpx.Validation("验收日期不能晚于今天")
	}
	result := strings.TrimSpace(req.Result)
	if !option.Has(ResultOptions(), result) {
		return httpx.Validation(fmt.Sprintf("验收结论只能是：%s", option.Labels(ResultOptions())))
	}
	if result != ResultRework {
		return nil
	}
	if strings.TrimSpace(req.Issues) == "" {
		return httpx.Validation("验收结论为需整改时，必须填写存在问题")
	}
	if req.RectifyDeadline == nil {
		return httpx.Validation("验收结论为需整改时，必须填写整改期限")
	}
	if req.RectifyDeadline.Before(req.AcceptedAt) {
		return httpx.Validation("整改期限不能早于验收日期")
	}
	return nil
}

func apply(req SaveRequest, target *AcceptanceRecord) {
	target.TaskID = req.TaskID
	target.CleaningRecordID = req.CleaningRecordID
	target.AcceptedAt = req.AcceptedAt
	target.InspectorName = strings.TrimSpace(req.InspectorName)
	target.InspectorOrg = strings.TrimSpace(req.InspectorOrg)
	target.Result = strings.TrimSpace(req.Result)
	target.ResidualSludgeMm = req.ResidualSludgeMm
	target.Issues = strings.TrimSpace(req.Issues)
	target.Rectification = strings.TrimSpace(req.Rectification)
	target.Remark = strings.TrimSpace(req.Remark)
	if target.Result == ResultRework {
		target.RectifyDeadline = req.RectifyDeadline
	}
}

// nextCode 生成形如 YS20260914-0001 的验收编号。
func (s *Service) nextCode(ctx context.Context, acceptedAt date.Date) string {
	prefix := "YS" + acceptedAt.Format("20060102")
	sequence := 1
	if latest, err := s.repo.MaxCodeWithPrefix(ctx, prefix); err == nil && latest != "" {
		if idx := strings.LastIndex(latest, "-"); idx >= 0 {
			if parsed, err := strconv.Atoi(latest[idx+1:]); err == nil {
				sequence = parsed + 1
			}
		}
	}
	return fmt.Sprintf("%s-%04d", prefix, sequence)
}

func notFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("验收记录不存在")
	}
	return httpx.WrapInternal("查询验收记录失败", err)
}
