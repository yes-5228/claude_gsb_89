package acceptance_test

import (
	"context"
	"testing"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/modules/acceptance"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/testsupport"
)

// TestScoreItemsArePersistedAndTotalMatches 登记验收后评分项明细落库，
// 且各评分项实际得分之和等于主表总分；详情与统计读取同一份固化数据。
func TestScoreItemsArePersistedAndTotalMatches(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "评分项明细的任务")

	created, err := fixture.Acceptances.Create(context.Background(), testsupport.PassRequest(task.ID, 92))
	testsupport.RequireNoError(t, err)
	if created.Score != 92 {
		t.Fatalf("期望总分由评分项汇总为 92，实际 %d", created.Score)
	}
	if created.ScoreTemplateID == nil {
		t.Fatal("验收记录应记录所使用的评分模板版本")
	}

	detail, err := fixture.Acceptances.Detail(context.Background(), created.ID)
	testsupport.RequireNoError(t, err)
	if len(detail.ScoreItems) == 0 {
		t.Fatal("详情应包含评分项明细")
	}
	sum := 0
	for _, item := range detail.ScoreItems {
		if item.MaxScore-item.ActualScore != item.Deduction {
			t.Fatalf("评分项 %s 的扣分分值与得分不一致", item.Name)
		}
		if item.Deduction > 0 && item.DeductionReason == "" {
			t.Fatalf("评分项 %s 有扣分但缺少扣分说明", item.Name)
		}
		sum += item.ActualScore
	}
	if sum != created.Score {
		t.Fatalf("评分项明细合计 %d 与主表总分 %d 不一致", sum, created.Score)
	}

	items, total, err := fixture.Acceptances.List(context.Background(), acceptance.ListQuery{
		Page: httpx.PageQuery{Page: 1, PageSize: 10},
	})
	testsupport.RequireNoError(t, err)
	if total != 1 || len(items[0].ScoreItems) != len(detail.ScoreItems) {
		t.Fatal("列表也应带出评分项明细")
	}
	listSum := 0
	for _, item := range items[0].ScoreItems {
		listSum += item.ActualScore
	}
	if listSum != items[0].Score {
		t.Fatalf("列表评分项合计 %d 与列表总分 %d 不一致", listSum, items[0].Score)
	}

	// 统计口径：平均总分取主表 score，逐项统计取评分项快照，二者必须与详情同源。
	summary, err := fixture.Acceptances.ScoreSummary(context.Background())
	testsupport.RequireNoError(t, err)
	if summary.AcceptanceTotal != 1 || summary.AverageScore != 92 {
		t.Fatalf("总分汇总异常：%+v", summary)
	}
	itemStats, err := fixture.Acceptances.ItemScoreStats(context.Background())
	testsupport.RequireNoError(t, err)
	statSum := 0.0
	for _, stat := range itemStats {
		if stat.SampleCount != 1 {
			t.Fatalf("评分项 %s 样本数应为 1，实际 %d", stat.Name, stat.SampleCount)
		}
		statSum += stat.AverageScore
	}
	if statSum != 92 {
		t.Fatalf("统计中各评分项平均分之和 %v 与验收总分 92 不一致", statSum)
	}
}

// TestScoreExceedingItemLimitIsRejected 单项实际得分超过上限时拒绝登记。
func TestScoreExceedingItemLimitIsRejected(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "单项超分的任务")

	req := testsupport.PassRequest(task.ID, 90)
	req.ScoreItems[0].ActualScore = intPtr(999)
	_, err := fixture.Acceptances.Create(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)
}

// TestDeductionRequiresReason 扣分但未填写扣分说明时拒绝登记。
func TestDeductionRequiresReason(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "缺扣分说明的任务")

	req := testsupport.PassRequest(task.ID, 90)
	// fillScoreItems 已为扣分项填好说明；清空全部说明后，有扣分的项应触发校验。
	for i := range req.ScoreItems {
		req.ScoreItems[i].DeductionReason = ""
	}
	_, err := fixture.Acceptances.Create(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)
}

// TestReworkWithPassingScoreIsRejected 汇总总分达到合格线时不允许判需整改。
func TestReworkWithPassingScoreIsRejected(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "高分却需整改的任务")

	req := testsupport.PassRequest(task.ID, 85)
	req.Result = acceptance.ResultRework
	req.Issues = "存在轻微外观问题"
	deadline := date.Today().AddDays(3)
	req.RectifyDeadline = &deadline
	_, err := fixture.Acceptances.Create(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)
}

// TestTemplateVersionEffectiveFrom 模板版本带生效时间：
// 新版本只影响生效日期之后登记的验收，历史记录仍引用旧版本、总分不变。
func TestTemplateVersionEffectiveFrom(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "模板版本切换的任务")

	before, err := fixture.Acceptances.Create(context.Background(), testsupport.PassRequest(task.ID, 80))
	testsupport.RequireNoError(t, err)
	originalTemplateID := *before.ScoreTemplateID
	originalItems, err := fixture.Acceptances.Detail(context.Background(), before.ID)
	testsupport.RequireNoError(t, err)

	// 明天生效的新版本：两项各 50 分，与默认模板的五项完全不同。
	tomorrow := date.Today().AddDays(1)
	_, err = fixture.Acceptances.CreateTemplateVersion(context.Background(), acceptance.CreateTemplateRequest{
		EffectiveFrom: tomorrow,
		Items: []acceptance.TemplateItemInput{
			{Name: "新评分项A", MaxScore: 50, SortOrder: 1},
			{Name: "新评分项B", MaxScore: 50, SortOrder: 2},
		},
	})
	testsupport.RequireNoError(t, err)

	// 今天仍适用旧版本。
	effective, err := fixture.Acceptances.EffectiveTemplate(context.Background(), date.Today())
	testsupport.RequireNoError(t, err)
	if len(effective.Items) != 5 {
		t.Fatalf("今天应仍适用五项的旧版本，实际 %d 项", len(effective.Items))
	}
	effectiveTomorrow, err := fixture.Acceptances.EffectiveTemplate(context.Background(), tomorrow)
	testsupport.RequireNoError(t, err)
	if len(effectiveTomorrow.Items) != 2 {
		t.Fatalf("明天应适用两项的新版本，实际 %d 项", len(effectiveTomorrow.Items))
	}

	// 历史验收记录仍引用旧版本、评分项明细与总分保持不变。
	stored, err := fixture.Acceptances.Detail(context.Background(), before.ID)
	testsupport.RequireNoError(t, err)
	if stored.Acceptance.Score != 80 {
		t.Fatalf("模板调整不应改变历史总分，实际 %d", stored.Acceptance.Score)
	}
	if stored.Template == nil || stored.Template.ID != originalTemplateID {
		t.Fatal("历史验收记录应继续引用登记时的模板版本")
	}
	if len(stored.ScoreItems) != len(originalItems.ScoreItems) || stored.ScoreItems[0].Name != originalItems.ScoreItems[0].Name {
		t.Fatal("模板调整不应改变历史评分项快照")
	}

	// 同一生效日期不能重复发布。
	_, err = fixture.Acceptances.CreateTemplateVersion(context.Background(), acceptance.CreateTemplateRequest{
		EffectiveFrom: tomorrow,
		Items: []acceptance.TemplateItemInput{
			{Name: "重复项", MaxScore: 100, SortOrder: 1},
		},
	})
	testsupport.RequireAppError(t, err, httpx.CodeConflict)
}

// TestCreateTemplateVersionRequiresTotal100 各评分项分值上限合计必须为 100。
func TestCreateTemplateVersionRequiresTotal100(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	_, err := fixture.Acceptances.CreateTemplateVersion(context.Background(), acceptance.CreateTemplateRequest{
		EffectiveFrom: date.Today().AddDays(7),
		Items: []acceptance.TemplateItemInput{
			{Name: "甲", MaxScore: 60, SortOrder: 1},
			{Name: "乙", MaxScore: 30, SortOrder: 2},
		},
	})
	testsupport.RequireAppError(t, err, httpx.CodeValidation)
}

// TestLegacyAcceptanceKeepsOriginalTotal 升级前登记的历史验收记录没有评分项明细，
// 总分保持原值不变，并正常出现在详情、列表与统计中。
func TestLegacyAcceptanceKeepsOriginalTotal(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "历史验收记录的任务")

	// 直接写主表模拟升级前的存量记录：有总分、无模板版本、无评分项快照。
	legacy := acceptance.AcceptanceRecord{
		Code:             "YS20190101-0001",
		TaskID:           task.ID,
		AcceptedAt:       date.MustParse("2019-01-01"),
		InspectorName:    "历史验收人",
		Result:           acceptance.ResultPass,
		Score:            77,
		ResidualSludgeMm: 20,
	}
	testsupport.RequireNoError(t, fixture.DB.Create(&legacy).Error)

	detail, err := fixture.Acceptances.Detail(context.Background(), legacy.ID)
	testsupport.RequireNoError(t, err)
	if detail.Acceptance.Score != 77 {
		t.Fatalf("历史记录总分应保持 77，实际 %d", detail.Acceptance.Score)
	}
	if detail.Template != nil || len(detail.ScoreItems) != 0 {
		t.Fatal("历史记录不应被回填模板或评分项明细")
	}

	items, total, err := fixture.Acceptances.List(context.Background(), acceptance.ListQuery{
		Page: httpx.PageQuery{Page: 1, PageSize: 10},
	})
	testsupport.RequireNoError(t, err)
	if total != 1 || items[0].Score != 77 || len(items[0].ScoreItems) != 0 {
		t.Fatal("历史记录在列表中应保持原总分且无评分项明细")
	}

	summary, err := fixture.Acceptances.ScoreSummary(context.Background())
	testsupport.RequireNoError(t, err)
	if summary.AcceptanceTotal != 1 || summary.AverageScore != 77 {
		t.Fatalf("历史记录总分应计入统计，实际 %+v", summary)
	}
	itemStats, err := fixture.Acceptances.ItemScoreStats(context.Background())
	testsupport.RequireNoError(t, err)
	if len(itemStats) != 0 {
		t.Fatal("历史记录没有评分项明细，不应出现在逐项统计中")
	}
}

// TestDetailAndStatsUseSamePersistedTotal 发布新版本后再登记一条验收，
// 验证新旧两份记录在详情与统计中各自的总分互不串扰。
func TestDetailAndStatsUseSamePersistedTotal(t *testing.T) {
	fixture := testsupport.NewFixture(t)

	firstTask := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "旧版本验收任务")
	first, err := fixture.Acceptances.Create(context.Background(), testsupport.PassRequest(firstTask.ID, 90))
	testsupport.RequireNoError(t, err)

	// 发布今天生效的新版本后，新建另一个管段/任务再验收。
	otherSegment := fixture.CreateSegment(t, "PS-TEST-002", "城西片区")
	secondTask := fixture.TaskReadyForAcceptance(t, otherSegment.ID, "新版本验收任务")
	_, err = fixture.Acceptances.CreateTemplateVersion(context.Background(), acceptance.CreateTemplateRequest{
		EffectiveFrom: date.Today(),
		Items: []acceptance.TemplateItemInput{
			{Name: "综合质量", MaxScore: 70, SortOrder: 1},
			{Name: "资料规范", MaxScore: 30, SortOrder: 2},
		},
	})
	testsupport.RequireNoError(t, err)

	secondReq := acceptance.SaveRequest{
		TaskID:        secondTask.ID,
		AcceptedAt:    date.Today(),
		InspectorName: "新模板验收人",
		Result:        acceptance.ResultPass,
		ScoreItems: []acceptance.ScoreItemInput{
			{Name: "综合质量", ActualScore: intPtr(60), DeductionReason: "局部清理不到位扣 10 分"},
			{Name: "资料规范", ActualScore: intPtr(25), DeductionReason: "记录缺签字扣 5 分"},
		},
	}
	second, err := fixture.Acceptances.Create(context.Background(), secondReq)
	testsupport.RequireNoError(t, err)
	if second.Score != 85 {
		t.Fatalf("新模板总分应为 85，实际 %d", second.Score)
	}

	firstDetail, err := fixture.Acceptances.Detail(context.Background(), first.ID)
	testsupport.RequireNoError(t, err)
	secondDetail, err := fixture.Acceptances.Detail(context.Background(), second.ID)
	testsupport.RequireNoError(t, err)
	if firstDetail.Acceptance.Score != 90 || secondDetail.Acceptance.Score != 85 {
		t.Fatalf("详情总分串扰：first=%d second=%d", firstDetail.Acceptance.Score, secondDetail.Acceptance.Score)
	}

	summary, err := fixture.Acceptances.ScoreSummary(context.Background())
	testsupport.RequireNoError(t, err)
	if summary.AcceptanceTotal != 2 || summary.AverageScore != 87.5 {
		t.Fatalf("统计平均分应取两份固化总分的均值 87.5，实际 %+v", summary)
	}
}

func intPtr(value int) *int {
	return &value
}
