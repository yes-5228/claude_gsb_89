package acceptance

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/shared/refx"
)

// ErrNotFound 验收记录不存在。
var ErrNotFound = errors.New("验收记录不存在")

// Repository 验收记录数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 构造仓储。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// DB 暴露底层连接。
func (r *Repository) DB() *gorm.DB {
	return r.db
}

// Transaction 在事务中执行验收写入与联动更新，保证两者同时成功或同时失败。
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// CreateInTx 在给定事务中新增验收记录。
func (r *Repository) CreateInTx(ctx context.Context, tx *gorm.DB, record *AcceptanceRecord) error {
	return tx.WithContext(ctx).Create(record).Error
}

// Save 保存验收记录全部字段。
func (r *Repository) Save(ctx context.Context, record *AcceptanceRecord) error {
	record.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(record).Error
}

// Delete 物理删除验收记录，并一并清理其评分明细，避免孤立明细混入评分项统计。
func (r *Repository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("acceptance_id = ?", id).Delete(&AcceptanceScoreDetail{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&AcceptanceRecord{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// FindByID 按主键查询。
func (r *Repository) FindByID(ctx context.Context, id uint) (*AcceptanceRecord, error) {
	var record AcceptanceRecord
	err := r.db.WithContext(ctx).First(&record, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// LatestByTask 查询任务最近一次验收记录，没有时返回 nil。
func (r *Repository) LatestByTask(ctx context.Context, taskID uint) (*AcceptanceRecord, error) {
	var record AcceptanceRecord
	err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("id DESC").
		Limit(1).
		Find(&record).Error
	if err != nil {
		return nil, err
	}
	if record.ID == 0 {
		return nil, nil
	}
	return &record, nil
}

// MaxCodeWithPrefix 查询前缀下已使用的最大验收编号。
func (r *Repository) MaxCodeWithPrefix(ctx context.Context, prefix string) (string, error) {
	var code string
	err := r.db.WithContext(ctx).Model(&AcceptanceRecord{}).
		Where("code LIKE ?", prefix+"-%").
		Order("code DESC").
		Limit(1).
		Pluck("code", &code).Error
	return code, err
}

// CleaningRecordBelongsToTask 校验清淤记录是否属于指定任务。
func (r *Repository) CleaningRecordBelongsToTask(ctx context.Context, recordID, taskID uint) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table(refx.TableCleaningRecords).
		Where("id = ? AND task_id = ?", recordID, taskID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ---------- 评分方案与评分明细 ----------

// CreateScheme 保存新的评分方案版本及其评分项。
func (r *Repository) CreateScheme(ctx context.Context, scheme *ScoreScheme) error {
	return r.db.WithContext(ctx).Create(scheme).Error
}

// LatestScheme 查询生效日期最新的方案版本，没有时返回 nil。
func (r *Repository) LatestScheme(ctx context.Context) (*ScoreScheme, error) {
	var scheme ScoreScheme
	err := r.db.WithContext(ctx).
		Order("effective_from DESC, id DESC").
		Limit(1).
		Find(&scheme).Error
	if err != nil {
		return nil, err
	}
	if scheme.ID == 0 {
		return nil, nil
	}
	return &scheme, nil
}

// SchemeEffectiveAt 查询在指定日期生效的评分方案（生效日期不晚于该日的最新版本），
// 没有时返回 nil。评分项按 sort 升序预加载。
func (r *Repository) SchemeEffectiveAt(ctx context.Context, day date.Date) (*ScoreScheme, error) {
	var scheme ScoreScheme
	err := r.db.WithContext(ctx).
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("sort ASC, id ASC") }).
		Where("effective_from <= ?", day.Time).
		Order("effective_from DESC, id DESC").
		Limit(1).
		Find(&scheme).Error
	if err != nil {
		return nil, err
	}
	if scheme.ID == 0 {
		return nil, nil
	}
	return &scheme, nil
}

// SchemeByID 按主键查询方案（不含评分项，详情页只需要方案名称与生效日期）。
func (r *Repository) SchemeByID(ctx context.Context, id uint) (*ScoreScheme, error) {
	var scheme ScoreScheme
	err := r.db.WithContext(ctx).First(&scheme, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &scheme, nil
}

// ListSchemes 查询全部方案版本，按生效日期倒序（最新版本在前）。
func (r *Repository) ListSchemes(ctx context.Context) ([]ScoreScheme, error) {
	schemes := make([]ScoreScheme, 0)
	err := r.db.WithContext(ctx).
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("sort ASC, id ASC") }).
		Order("effective_from DESC, id DESC").
		Find(&schemes).Error
	return schemes, err
}

// CreateDetailsInTx 在事务中写入验收记录的逐项评分快照。
func (r *Repository) CreateDetailsInTx(ctx context.Context, tx *gorm.DB, details []AcceptanceScoreDetail) error {
	return tx.WithContext(ctx).Create(&details).Error
}

// DetailsByAcceptanceIDs 批量查询验收记录的评分明细快照，按验收记录 ID 分组。
func (r *Repository) DetailsByAcceptanceIDs(ctx context.Context, acceptanceIDs []uint) (map[uint][]AcceptanceScoreDetail, error) {
	result := make(map[uint][]AcceptanceScoreDetail, len(acceptanceIDs))
	if len(acceptanceIDs) == 0 {
		return result, nil
	}
	details := make([]AcceptanceScoreDetail, 0)
	err := r.db.WithContext(ctx).
		Where("acceptance_id IN ?", acceptanceIDs).
		Order("sort ASC, id ASC").
		Find(&details).Error
	if err != nil {
		return nil, err
	}
	for _, item := range details {
		result[item.AcceptanceID] = append(result[item.AcceptanceID], item)
	}
	return result, nil
}

// List 分页查询验收记录。
func (r *Repository) List(ctx context.Context, query ListQuery) ([]AcceptanceRecord, int64, error) {
	query.Page.Normalize()
	var total int64
	if err := r.filtered(ctx, query).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	records := make([]AcceptanceRecord, 0)
	err := r.filtered(ctx, query).
		Order("accepted_at DESC, id DESC").
		Offset(query.Page.Offset()).
		Limit(query.Page.PageSize).
		Find(&records).Error
	if err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func (r *Repository) filtered(ctx context.Context, query ListQuery) *gorm.DB {
	tx := r.db.WithContext(ctx).Model(&AcceptanceRecord{})
	if keyword := strings.ToLower(strings.TrimSpace(query.Keyword)); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where(
			"LOWER(code) LIKE ? OR LOWER(inspector_name) LIKE ? OR LOWER(inspector_org) LIKE ? OR LOWER(issues) LIKE ?",
			like, like, like, like,
		)
	}
	if query.TaskID > 0 {
		tx = tx.Where("task_id = ?", query.TaskID)
	}
	if query.SegmentID > 0 {
		subQuery := r.db.WithContext(ctx).Table(refx.TableCleaningTasks).
			Select("id").
			Where("pipe_segment_id = ?", query.SegmentID)
		tx = tx.Where("task_id IN (?)", subQuery)
	}
	if query.Result != "" {
		tx = tx.Where("result = ?", query.Result)
	}
	if query.InspectorName != "" {
		tx = tx.Where("inspector_name = ?", query.InspectorName)
	}
	if query.DateFrom != nil {
		tx = tx.Where("accepted_at >= ?", query.DateFrom.Time)
	}
	if query.DateTo != nil {
		tx = tx.Where("accepted_at <= ?", query.DateTo.Time)
	}
	if query.PendingRectify {
		tx = tx.Where("result = ? AND rectified_at IS NULL", ResultRework)
	}
	return tx
}

// CountByResult 按验收结论统计数量。
func (r *Repository) CountByResult(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Result string
		Total  int64
	}
	rows := make([]row, 0)
	err := r.db.WithContext(ctx).Model(&AcceptanceRecord{}).
		Select("result, COUNT(*) AS total").
		Group("result").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, item := range rows {
		result[item.Result] = item.Total
	}
	return result, nil
}

// CountPendingRectify 统计尚未完成整改的验收记录数。
func (r *Repository) CountPendingRectify(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&AcceptanceRecord{}).
		Where("result = ? AND rectified_at IS NULL", ResultRework).
		Count(&count).Error
	return count, err
}
