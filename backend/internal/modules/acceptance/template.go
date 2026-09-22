package acceptance

import (
	"time"

	"github.com/drainage/desilting/internal/shared/date"
)

// DefaultTemplateEffectiveFrom 默认评分模板的生效日期。
//
// 取一个足够早的固定日期，保证升级前已登记的验收记录也能回溯到同一版模板口径；
// 但这些历史记录没有评分项快照，总分仍以登记时写入的 score 为准。
const DefaultTemplateEffectiveFrom = "2020-01-01"

// ScoreTemplateVersion 验收评分项模板版本。
//
// 模板一经生效即为不可变的历史档案：调整评分项时新增一个带生效日期的新版本，
// 已登记的验收记录继续引用当时的版本，因此历史总分不会随模板调整而变化。
// 验收记录按"验收日期落在哪个版本的生效区间"确定使用的版本。
type ScoreTemplateVersion struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	EffectiveFrom date.Date `gorm:"type:date;uniqueIndex;not null" json:"effectiveFrom"`
	Remark        string    `gorm:"type:text" json:"remark"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// TableName 指定表名。
func (ScoreTemplateVersion) TableName() string {
	return "acceptance_score_templates"
}

// ScoreTemplateItem 评分模板版本下的单个评分项定义。
type ScoreTemplateItem struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	TemplateID uint   `gorm:"index;uniqueIndex:idx_template_item_order,priority:1;not null" json:"templateId"`
	Name       string `gorm:"size:64;not null" json:"name"`
	// MaxScore 该评分项的分值上限，同一版本下所有评分项上限之和必须为 100。
	MaxScore  int    `gorm:"not null" json:"maxScore"`
	SortOrder int    `gorm:"uniqueIndex:idx_template_item_order,priority:2;not null" json:"sortOrder"`
	Criteria  string `gorm:"size:255" json:"criteria"`
}

// TableName 指定表名。
func (ScoreTemplateItem) TableName() string {
	return "acceptance_score_template_items"
}

// AcceptanceScoreItem 验收记录的评分项明细（快照）。
//
// 登记验收时按当时模板逐项打分并固化到本表：实际得分、扣分分值与扣分说明都留痕。
// 验收总分 = 各评分项实际得分之和，与验收记录主表 score 字段一致；
// 详情、列表与统计一律读取这份快照（或主表 score），不实时回算模板，保证三处总分同源。
type AcceptanceScoreItem struct {
	ID                uint   `gorm:"primaryKey" json:"id"`
	AcceptanceID      uint   `gorm:"index;uniqueIndex:idx_acceptance_item_order,priority:1;not null" json:"acceptanceId"`
	TemplateID        uint   `gorm:"index;not null" json:"templateId"`
	Name              string `gorm:"size:64;not null" json:"name"`
	MaxScore          int    `gorm:"not null" json:"maxScore"`
	ActualScore       int    `gorm:"not null" json:"actualScore"`
	Deduction         int    `gorm:"not null" json:"deduction"`
	DeductionReason   string `gorm:"size:255" json:"deductionReason"`
	SortOrder         int    `gorm:"uniqueIndex:idx_acceptance_item_order,priority:2;not null" json:"sortOrder"`
}

// TableName 指定表名。
func (AcceptanceScoreItem) TableName() string {
	return "acceptance_score_items"
}

// DefaultTemplateItems 内置的默认评分项，各评分项上限合计 100 分。
func DefaultTemplateItems() []ScoreTemplateItem {
	return []ScoreTemplateItem{
		{Name: "管道淤积清理", MaxScore: 30, SortOrder: 1, Criteria: "管内淤积清除彻底，无明显残留"},
		{Name: "残留淤积厚度", MaxScore: 20, SortOrder: 2, Criteria: "残留淤积厚度不超过 20mm"},
		{Name: "过水断面恢复", MaxScore: 20, SortOrder: 3, Criteria: "过水断面恢复满足设计要求"},
		{Name: "井室与附属设施", MaxScore: 15, SortOrder: 4, Criteria: "检查井、井室清洁，附属设施完好"},
		{Name: "现场与资料规范", MaxScore: 15, SortOrder: 5, Criteria: "现场清理到位、记录资料完整规范"},
	}
}
