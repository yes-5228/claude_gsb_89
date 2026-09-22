package acceptance

import (
	"time"

	"github.com/drainage/desilting/internal/shared/date"
)

// 评分方案的分值上限之和固定为 100 分，合格线（passScoreThreshold）才有统一口径。
const schemeTotalScore = 100

// ScoreScheme 验收评分方案：一套评分项的集合。
//
// 方案按生效日期版本化，每次调整评分项都新增一个版本并指定生效日期，
// 旧版本保留不动。登记验收时按验收日期匹配「生效日期不晚于验收日期的最新版本」，
// 因此调整评分项只会影响生效日期之后登记的验收，历史记录的总分与明细不受影响。
type ScoreScheme struct {
	ID            uint        `gorm:"primaryKey" json:"id"`
	Title         string      `gorm:"size:64;not null" json:"title"`
	EffectiveFrom date.Date   `gorm:"type:date;uniqueIndex;not null" json:"effectiveFrom"`
	Remark        string      `gorm:"type:text" json:"remark"`
	CreatedAt     time.Time   `json:"createdAt"`
	Items         []ScoreItem `gorm:"foreignKey:SchemeID" json:"items"`
}

// TableName 指定表名。
func (ScoreScheme) TableName() string {
	return "acceptance_score_schemes"
}

// ScoreItem 评分项定义：分值上限 + 扣分说明。
type ScoreItem struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	SchemeID  uint   `gorm:"index;not null" json:"schemeId"`
	Name      string `gorm:"size:64;not null" json:"name"`
	MaxScore  int    `gorm:"not null" json:"maxScore"`
	Deduction string `gorm:"size:255" json:"deduction"`
	Sort      int    `gorm:"not null;default:0" json:"sort"`
}

// TableName 指定表名。
func (ScoreItem) TableName() string {
	return "acceptance_score_items"
}

// AcceptanceScoreDetail 验收登记时的逐项评分快照。
//
// 评分项的名称、分值上限、扣分说明在登记时原样复制到快照里，
// 之后即使调整评分方案，历史验收记录的明细与总分也保持登记时的样子。
type AcceptanceScoreDetail struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	AcceptanceID uint   `gorm:"index;not null" json:"acceptanceId"`
	ItemID       uint   `gorm:"not null" json:"itemId"`
	Name         string `gorm:"size:64;not null" json:"name"`
	MaxScore     int    `gorm:"not null" json:"maxScore"`
	Deduction    string `gorm:"size:255" json:"deduction"`
	Score        int    `gorm:"not null" json:"score"`
	Sort         int    `gorm:"not null;default:0" json:"sort"`
}

// TableName 指定表名。
func (AcceptanceScoreDetail) TableName() string {
	return "acceptance_score_details"
}
