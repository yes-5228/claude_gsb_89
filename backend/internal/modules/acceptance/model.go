// Package acceptance 验收记录模块：对已完工的清淤任务做质量验收，并驱动任务闭环。
package acceptance

import (
	"time"

	"github.com/drainage/desilting/internal/shared/date"
)

// 验收结论。
const (
	ResultPass   = "pass"   // 合格
	ResultRework = "rework" // 需整改
)

// AcceptanceRecord 验收记录。验收结论一经登记不可修改，只能补充整改情况，
// 以保证验收过程的严肃性与可追溯性。
//
// Score 是登记时确定的总分快照：启用评分方案后由评分项得分自动汇总写入，
// 启用前登记的记录保留当时手工填写的总分。详情、列表与统计一律读取该字段，
// 不会按最新评分方案重算，因此同一条记录在任何页面看到的总分都一致。
type AcceptanceRecord struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	Code             string     `gorm:"size:32;uniqueIndex;not null" json:"code"`
	TaskID           uint       `gorm:"index;not null" json:"taskId"`
	CleaningRecordID *uint      `gorm:"index" json:"cleaningRecordId"`
	AcceptedAt       date.Date  `gorm:"type:date;index;not null" json:"acceptedAt"`
	InspectorName    string     `gorm:"size:32;not null" json:"inspectorName"`
	InspectorOrg     string     `gorm:"size:128" json:"inspectorOrg"`
	Result           string     `gorm:"size:16;index;not null" json:"result"`
	Score            int        `gorm:"not null" json:"score"`
	SchemeID         *uint      `gorm:"index" json:"schemeId"`
	ResidualSludgeMm float64    `json:"residualSludgeMm"`
	Issues           string     `gorm:"type:text" json:"issues"`
	Rectification    string     `gorm:"type:text" json:"rectification"`
	RectifyDeadline  *date.Date `gorm:"type:date" json:"rectifyDeadline"`
	RectifiedAt      *date.Date `gorm:"type:date" json:"rectifiedAt"`
	Remark           string     `gorm:"type:text" json:"remark"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// TableName 指定表名。
func (AcceptanceRecord) TableName() string {
	return "acceptance_records"
}
