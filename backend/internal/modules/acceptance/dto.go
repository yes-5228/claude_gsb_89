package acceptance

import (
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/shared/refx"
)

// SaveRequest 登记验收记录的请求体。
//
// 验收日期已有生效评分方案时，必须携带 ScoreItems 逐项打分，总分由评分项自动汇总；
// 验收日期早于首个方案生效日期时沿用 Score 手工总分（历史登记口径）。
type SaveRequest struct {
	TaskID           uint             `json:"taskId" label:"关联任务" validate:"required"`
	CleaningRecordID *uint            `json:"cleaningRecordId" label:"关联清淤记录"`
	AcceptedAt       date.Date        `json:"acceptedAt" label:"验收日期"`
	InspectorName    string           `json:"inspectorName" label:"验收人" validate:"required,max=32"`
	InspectorOrg     string           `json:"inspectorOrg" label:"验收单位" validate:"max=128"`
	Result           string           `json:"result" label:"验收结论" validate:"required"`
	Score            int              `json:"score" label:"验收评分" validate:"gte=0,lte=100"`
	ScoreItems       []ScoreItemInput `json:"scoreItems" label:"评分项" validate:"max=20,dive"`
	ResidualSludgeMm float64          `json:"residualSludgeMm" label:"残留淤积厚度(mm)" validate:"gte=0,lte=1000"`
	Issues           string           `json:"issues" label:"存在问题" validate:"max=1000"`
	Rectification    string           `json:"rectification" label:"整改要求" validate:"max=1000"`
	RectifyDeadline  *date.Date       `json:"rectifyDeadline" label:"整改期限"`
	Remark           string           `json:"remark" label:"备注" validate:"max=1000"`
}

// ScoreItemInput 单个评分项的打分。
type ScoreItemInput struct {
	ItemID uint `json:"itemId" label:"评分项" validate:"required"`
	Score  int  `json:"score" label:"评分项得分" validate:"gte=0,lte=100"`
}

// SaveSchemeRequest 调整评分项（新增方案版本）的请求体。
type SaveSchemeRequest struct {
	Title         string            `json:"title" label:"方案名称" validate:"required,max=64"`
	EffectiveFrom date.Date         `json:"effectiveFrom" label:"生效日期"`
	Remark        string            `json:"remark" label:"备注" validate:"max=500"`
	Items         []SchemeItemInput `json:"items" label:"评分项" validate:"required,min=1,max=10,dive"`
}

// SchemeItemInput 方案中单个评分项的定义。
type SchemeItemInput struct {
	Name      string `json:"name" label:"评分项名称" validate:"required,max=64"`
	MaxScore  int    `json:"maxScore" label:"分值上限" validate:"gt=0,lte=100"`
	Deduction string `json:"deduction" label:"扣分说明" validate:"max=255"`
}

// RectifyRequest 登记整改完成的请求体。
type RectifyRequest struct {
	RectifiedAt   date.Date `json:"rectifiedAt" label:"整改完成日期"`
	Rectification string    `json:"rectification" label:"整改情况说明" validate:"max=1000"`
	Remark        string    `json:"remark" label:"备注" validate:"max=1000"`
}

// ListQuery 验收记录列表查询条件。
type ListQuery struct {
	Keyword        string
	TaskID         uint
	SegmentID      uint
	Result         string
	InspectorName  string
	DateFrom       *date.Date
	DateTo         *date.Date
	PendingRectify bool
	Page           httpx.PageQuery
}

// ParseListQuery 解析列表查询条件。
func ParseListQuery(c *fiber.Ctx) (ListQuery, error) {
	query := ListQuery{
		Keyword:        httpx.TrimmedQuery(c, "keyword"),
		TaskID:         uint(c.QueryInt("taskId", 0)),
		SegmentID:      uint(c.QueryInt("segmentId", 0)),
		Result:         httpx.TrimmedQuery(c, "result"),
		InspectorName:  httpx.TrimmedQuery(c, "inspectorName"),
		PendingRectify: c.QueryBool("pendingRectify", false),
		Page:           httpx.ParsePage(c),
	}
	from, err := parseDateParam(c, "dateFrom", "验收日期起")
	if err != nil {
		return ListQuery{}, err
	}
	to, err := parseDateParam(c, "dateTo", "验收日期止")
	if err != nil {
		return ListQuery{}, err
	}
	query.DateFrom = from
	query.DateTo = to
	return query, nil
}

func parseDateParam(c *fiber.Ctx, key, label string) (*date.Date, error) {
	raw := httpx.TrimmedQuery(c, key)
	if raw == "" {
		return nil, nil
	}
	parsed, err := date.Parse(raw)
	if err != nil {
		return nil, httpx.BadRequest(fmt.Sprintf("%s格式不正确，应为 YYYY-MM-DD", label))
	}
	return &parsed, nil
}

// ListItem 验收记录列表项。
type ListItem struct {
	AcceptanceRecord
	Task       *refx.TaskBrief         `json:"task"`
	ScoreItems []AcceptanceScoreDetail `json:"scoreItems"`
}

// DetailResponse 验收详情：验收记录 + 任务与管段 + 该任务的清淤汇总 + 逐项评分明细。
type DetailResponse struct {
	Acceptance   *AcceptanceRecord       `json:"acceptance"`
	Task         *refx.TaskBrief         `json:"task"`
	RecordTotals refx.RecordTotals       `json:"recordTotals"`
	ScoreItems   []AcceptanceScoreDetail `json:"scoreItems"`
	Scheme       *ScoreScheme            `json:"scheme"`
}
