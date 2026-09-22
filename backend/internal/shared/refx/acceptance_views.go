package refx

// AcceptanceScoreSummary 验收总分汇总（供运行看板）。
//
// 总分一律取验收记录主表的 score 字段（登记时由评分项自动汇总并固化），
// 不根据评分模板实时回算，因此与验收详情、列表展示的总分始终同源一致。
type AcceptanceScoreSummary struct {
	AcceptanceTotal int64   `json:"acceptanceTotal"`
	AverageScore    float64 `json:"averageScore"`
}

// AcceptanceItemScoreStat 单个评分项的统计（供运行看板）。
//
// 统计口径来自评分项快照表 acceptance_score_items：模板版本调整只影响之后登记的记录，
// 历史记录沿用当时快照，保证"同一份验收记录在详情和统计里算出的总分一致"。
// 升级前没有评分项明细的历史记录不参与逐项统计（但其总分仍计入 AcceptanceScoreSummary）。
type AcceptanceItemScoreStat struct {
	TemplateID      uint    `json:"templateId"`
	SortOrder       int     `json:"sortOrder"`
	Name            string  `json:"name"`
	MaxScore        int     `json:"maxScore"`
	SampleCount     int64   `json:"sampleCount"`
	AverageScore    float64 `json:"averageScore"`
	DeductedCount   int64   `json:"deductedCount"`
	DeductedRate    float64 `json:"deductedRate"`
}
