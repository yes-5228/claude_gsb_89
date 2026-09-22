package acceptance

import (
	"context"

	"github.com/drainage/desilting/internal/shared/num"
	"github.com/drainage/desilting/internal/shared/refx"
)

// ScoreSummary 验收记录总分汇总：平均总分直接取主表固化的 score。
func (r *Repository) ScoreSummary(ctx context.Context) (refx.AcceptanceScoreSummary, error) {
	var summary refx.AcceptanceScoreSummary
	type row struct {
		Total   int64
		Average float64
	}
	var stats row
	err := r.db.WithContext(ctx).Table(refx.TableAcceptanceRecords).
		Select("COUNT(*) AS total, COALESCE(AVG(score), 0) AS average").
		Scan(&stats).Error
	if err != nil {
		return summary, err
	}
	summary.AcceptanceTotal = stats.Total
	summary.AverageScore = num.Round2(stats.Average)
	return summary, nil
}

// ItemScoreStats 按评分项（模板版本 + 排序号）汇总实际得分与扣分情况。
func (r *Repository) ItemScoreStats(ctx context.Context) ([]refx.AcceptanceItemScoreStat, error) {
	type row struct {
		TemplateID    uint
		SortOrder     int
		Name          string
		MaxScore      int
		SampleCount   int64
		AverageScore  float64
		DeductedCount int64
	}
	rows := make([]row, 0)
	err := r.db.WithContext(ctx).Table(refx.TableAcceptanceScoreItems).
		Select(`template_id, sort_order, MAX(name) AS name, MAX(max_score) AS max_score,
			COUNT(*) AS sample_count,
			COALESCE(AVG(actual_score), 0) AS average_score,
			COALESCE(SUM(CASE WHEN deduction > 0 THEN 1 ELSE 0 END), 0) AS deducted_count`).
		Group("template_id, sort_order").
		Order("template_id DESC, sort_order ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	stats := make([]refx.AcceptanceItemScoreStat, 0, len(rows))
	for _, item := range rows {
		var deductedRate float64
		if item.SampleCount > 0 {
			deductedRate = num.Round2(float64(item.DeductedCount) / float64(item.SampleCount) * 100)
		}
		stats = append(stats, refx.AcceptanceItemScoreStat{
			TemplateID:    item.TemplateID,
			SortOrder:     item.SortOrder,
			Name:          item.Name,
			MaxScore:      item.MaxScore,
			SampleCount:   item.SampleCount,
			AverageScore:  num.Round2(item.AverageScore),
			DeductedCount: item.DeductedCount,
			DeductedRate:  deductedRate,
		})
	}
	return stats, nil
}
