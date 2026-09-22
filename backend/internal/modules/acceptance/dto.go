package acceptance

import (
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/shared/refx"
)

// SaveRequest 登记验收记录的请求体。
type SaveRequest struct {
	TaskID           uint             `json:"taskId" label:"关联任务" validate:"required"`
	CleaningRecordID *uint            `json:"cleaningRecordId" label:"关联清淤记录"`
	AcceptedAt       date.Date        `json:"acceptedAt" label:"验收日期"`
	InspectorName    string           `json:"inspectorName" label:"验收人" validate:"required,max=32"`
	InspectorOrg     string           `json:"inspectorOrg" label:"验收单位" validate:"max=128"`
	Result           string           `json:"result" label:"验收结论" validate:"required"`
	ResidualSludgeMm float64          `json:"residualSludgeMm" label:"残留淤积厚度(mm)" validate:"gte=0,lte=1000"`
	// ScoreItems 按验收日期所适用模板的评分项逐项打分，总分由后端自动汇总。
	ScoreItems      []ScoreItemInput `json:"scoreItems" label:"评分项明细" validate:"required,min=1,dive"`
	Issues          string           `json:"issues" label:"存在问题" validate:"max=1000"`
	Rectification   string           `json:"rectification" label:"整改要求" validate:"max=1000"`
	RectifyDeadline *date.Date       `json:"rectifyDeadline" label:"整改期限"`
	Remark          string           `json:"remark" label:"备注" validate:"max=1000"`
}

// ScoreItemInput 登记验收时单个评分项的打分结果。
type ScoreItemInput struct {
	Name            string `json:"name" label:"评分项" validate:"required,max=64"`
	ActualScore     *int   `json:"actualScore" label:"实际得分" validate:"required"`
	DeductionReason string `json:"deductionReason" label:"扣分说明" validate:"max=255"`
}

// CreateTemplateRequest 新增评分模板版本的请求体（调整评分项 = 发布新版本）。
type CreateTemplateRequest struct {
	EffectiveFrom date.Date           `json:"effectiveFrom" label:"生效日期"`
	Items         []TemplateItemInput `json:"items" label:"评分项" validate:"required,min=1,max=20,dive"`
	Remark        string              `json:"remark" label:"备注" validate:"max=1000"`
}

// TemplateItemInput 评分模板版本下单个评分项定义。
type TemplateItemInput struct {
	Name      string `json:"name" label:"评分项名称" validate:"required,max=64"`
	MaxScore  int    `json:"maxScore" label:"分值上限" validate:"gte=1,lte=100"`
	Criteria  string `json:"criteria" label:"评分标准" validate:"max=255"`
	SortOrder int    `json:"sortOrder" label:"排序号" validate:"gte=1,lte=20"`
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
	Task       *refx.TaskBrief       `json:"task"`
	ScoreItems []AcceptanceScoreItem `json:"scoreItems"`
}

// DetailResponse 验收详情：验收记录 + 评分项明细 + 任务与清淤汇总。
type DetailResponse struct {
	Acceptance   *AcceptanceRecord     `json:"acceptance"`
	ScoreItems   []AcceptanceScoreItem `json:"scoreItems"`
	Template     *ScoreTemplateVersion `json:"template"`
	Task         *refx.TaskBrief       `json:"task"`
	RecordTotals refx.RecordTotals     `json:"recordTotals"`
}

// TemplateResponse 评分模板版本及其评分项。
type TemplateResponse struct {
	ScoreTemplateVersion
	Items []ScoreTemplateItem `json:"items"`
	// Active 表示该版本是否覆盖当前日期（列表接口附带，便于前端标注当前版本）。
	Active bool `json:"active"`
}

// EffectiveTemplateResponse 指定日期适用的评分模板。
type EffectiveTemplateResponse struct {
	EffectiveFrom date.Date           `json:"effectiveFrom"`
	Items         []ScoreTemplateItem `json:"items"`
}
