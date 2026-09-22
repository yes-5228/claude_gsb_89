package acceptance_test

import (
	"context"
	"testing"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/modules/acceptance"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/testsupport"
)

func TestItemizedAcceptanceSumsScoreItems(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	scheme := fixture.CreateScheme(t, testsupport.SchemeRequest())
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "逐项评分的任务")

	// 35 + 18 + 13 + 12 + 8 = 86，总分应由评分项自动汇总，而不是取请求里的 score 字段
	req := testsupport.ItemizedRequest(task.ID, scheme, 35, 18, 13, 12, 8)
	req.Score = 1
	record, err := fixture.Acceptances.Create(context.Background(), req)
	testsupport.RequireNoError(t, err)
	if record.Score != 86 {
		t.Fatalf("总分应为评分项之和 86，实际 %d", record.Score)
	}
	if record.SchemeID == nil || *record.SchemeID != scheme.ID {
		t.Fatalf("验收记录应关联评分方案 %d，实际 %+v", scheme.ID, record.SchemeID)
	}

	detail, err := fixture.Acceptances.Detail(context.Background(), record.ID)
	testsupport.RequireNoError(t, err)
	if len(detail.ScoreItems) != len(scheme.Items) {
		t.Fatalf("详情应包含 %d 条评分明细，实际 %d", len(scheme.Items), len(detail.ScoreItems))
	}
	sum := 0
	for i, item := range detail.ScoreItems {
		sum += item.Score
		if item.Name != scheme.Items[i].Name || item.MaxScore != scheme.Items[i].MaxScore {
			t.Fatalf("明细应快照评分项名称与分值上限，实际 %+v", item)
		}
		if item.Deduction == "" {
			t.Fatal("明细应快照扣分说明")
		}
	}
	if sum != detail.Acceptance.Score {
		t.Fatalf("明细合计 %d 应与总分 %d 一致", sum, detail.Acceptance.Score)
	}
	if detail.Scheme == nil || detail.Scheme.Title != scheme.Title {
		t.Fatalf("详情应带出评分方案信息，实际 %+v", detail.Scheme)
	}

	items, total, err := fixture.Acceptances.List(context.Background(), acceptance.ListQuery{
		Page: httpx.PageQuery{Page: 1, PageSize: 10},
	})
	testsupport.RequireNoError(t, err)
	if total != 1 || len(items) != 1 {
		t.Fatalf("列表应返回 1 条记录，实际 total=%d len=%d", total, len(items))
	}
	if len(items[0].ScoreItems) != len(scheme.Items) {
		t.Fatalf("列表项应包含 %d 条评分明细，实际 %d", len(scheme.Items), len(items[0].ScoreItems))
	}
}

func TestItemizedScoreRejectsOverMax(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	scheme := fixture.CreateScheme(t, testsupport.SchemeRequest())
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "超出分值上限的任务")

	req := testsupport.ItemizedRequest(task.ID, scheme, 41, 18, 13, 12, 8)
	_, err := fixture.Acceptances.Create(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)

	// 负分同样不允许
	req = testsupport.ItemizedRequest(task.ID, scheme, 35, 18, 13, 12, -1)
	_, err = fixture.Acceptances.Create(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)
}

func TestItemizedScoreRequiresAllItems(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	scheme := fixture.CreateScheme(t, testsupport.SchemeRequest())
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "缺项打分的任务")

	// 方案生效后必须逐项打分，不能只给总分
	req := testsupport.PassRequest(task.ID, 90)
	_, err := fixture.Acceptances.Create(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)

	// 缺少一个评分项也不行
	partial := testsupport.ItemizedRequest(task.ID, scheme, 35, 18, 13, 12, 8)
	partial.ScoreItems = partial.ScoreItems[:len(partial.ScoreItems)-1]
	_, err = fixture.Acceptances.Create(context.Background(), partial)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)

	// 混入不属于现行方案的评分项同样拒绝
	stale := testsupport.ItemizedRequest(task.ID, scheme, 35, 18, 13, 12, 8)
	stale.ScoreItems[0].ItemID = scheme.Items[0].ID + 10000
	_, err = fixture.Acceptances.Create(context.Background(), stale)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)
}

func TestLowItemizedTotalForcesRework(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	scheme := fixture.CreateScheme(t, testsupport.SchemeRequest())
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "汇总分过低的任务")

	// 20 + 10 + 10 + 8 + 5 = 53，低于合格线，结论只能选需整改
	req := testsupport.ItemizedRequest(task.ID, scheme, 20, 10, 10, 8, 5)
	_, err := fixture.Acceptances.Create(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)

	req.Result = acceptance.ResultRework
	_, err = fixture.Acceptances.Create(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)

	req.Issues = "残留淤积厚度超标，检查井内遗留杂物"
	deadline := date.Today().AddDays(5)
	req.RectifyDeadline = &deadline
	record, err := fixture.Acceptances.Create(context.Background(), req)
	testsupport.RequireNoError(t, err)
	if record.Score != 53 {
		t.Fatalf("总分应为 53，实际 %d", record.Score)
	}
	if record.Result != acceptance.ResultRework {
		t.Fatalf("低于合格线时结论应为需整改，实际 %s", record.Result)
	}
}

func TestAcceptanceBeforeSchemeEffectiveUsesManualScore(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	req := testsupport.SchemeRequest()
	req.EffectiveFrom = date.Today().AddDays(7)
	fixture.CreateScheme(t, req)
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "方案生效前的任务")

	// 方案尚未生效：沿用手工总分
	record, err := fixture.Acceptances.Create(context.Background(), testsupport.PassRequest(task.ID, 90))
	testsupport.RequireNoError(t, err)
	if record.Score != 90 {
		t.Fatalf("手工总分为 90，实际 %d", record.Score)
	}
	if record.SchemeID != nil {
		t.Fatal("方案生效前登记的记录不应关联评分方案")
	}

	// 方案尚未生效：不允许按评分项打分
	task2 := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "方案生效前误传评分项的任务")
	scheme, err := fixture.Acceptances.EffectiveScheme(context.Background(), date.Today().AddDays(7))
	testsupport.RequireNoError(t, err)
	itemized := testsupport.ItemizedRequest(task2.ID, scheme, 35, 18, 13, 12, 8)
	_, err = fixture.Acceptances.Create(context.Background(), itemized)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)
}

func TestSchemeAdjustmentKeepsHistoryTotals(t *testing.T) {
	fixture := testsupport.NewFixture(t)

	// V1 方案 10 天前生效
	v1req := testsupport.SchemeRequest()
	v1req.EffectiveFrom = date.Today().AddDays(-10)
	schemeV1 := fixture.CreateScheme(t, v1req)

	// 按 V1 方案验收（5 天前登记）：35 + 18 + 13 + 12 + 8 = 86
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "方案调整前验收的任务")
	reqV1 := testsupport.ItemizedRequest(task.ID, schemeV1, 35, 18, 13, 12, 8)
	reqV1.AcceptedAt = date.Today().AddDays(-5)
	record, err := fixture.Acceptances.Create(context.Background(), reqV1)
	testsupport.RequireNoError(t, err)

	// 调整评分项：V2 方案改变分值上限与扣分说明，今天生效
	v2 := testsupport.SchemeRequest()
	v2.Title = "测试评分方案 V2"
	v2.EffectiveFrom = date.Today()
	v2.Items = []acceptance.SchemeItemInput{
		{Name: "清淤洁净度", MaxScore: 50, Deduction: "新版扣分说明"},
		{Name: "过水断面恢复", MaxScore: 30},
		{Name: "安全文明施工", MaxScore: 20},
	}
	fixture.CreateScheme(t, v2)

	// 历史记录在详情里的总分与明细保持登记时的样子
	detail, err := fixture.Acceptances.Detail(context.Background(), record.ID)
	testsupport.RequireNoError(t, err)
	if detail.Acceptance.Score != 86 {
		t.Fatalf("方案调整后历史记录总分应保持 86，实际 %d", detail.Acceptance.Score)
	}
	if len(detail.ScoreItems) != len(schemeV1.Items) {
		t.Fatalf("历史记录明细应保持 %d 项，实际 %d", len(schemeV1.Items), len(detail.ScoreItems))
	}
	if detail.ScoreItems[0].MaxScore != 40 || detail.ScoreItems[0].Deduction != "残留淤积超标扣分" {
		t.Fatalf("历史记录明细应保持 V1 快照，实际 %+v", detail.ScoreItems[0])
	}

	// 列表里的总分与明细必须与详情一致，不能算出第二个总分
	items, _, err := fixture.Acceptances.List(context.Background(), acceptance.ListQuery{
		Page: httpx.PageQuery{Page: 1, PageSize: 10},
	})
	testsupport.RequireNoError(t, err)
	if len(items) != 1 || items[0].Score != detail.Acceptance.Score {
		t.Fatalf("列表总分应与详情一致（86），实际 %+v", items)
	}
	if len(items[0].ScoreItems) != len(detail.ScoreItems) {
		t.Fatalf("列表明细应与详情一致，实际 %d 项", len(items[0].ScoreItems))
	}

	// 生效日期到达后，新登记的验收使用 V2 方案
	task2 := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "方案调整后验收的任务")
	schemeV2, err := fixture.Acceptances.EffectiveScheme(context.Background(), date.Today())
	testsupport.RequireNoError(t, err)
	recordV2, err := fixture.Acceptances.Create(context.Background(),
		testsupport.ItemizedRequest(task2.ID, schemeV2, 45, 25, 18))
	testsupport.RequireNoError(t, err)
	if recordV2.Score != 88 {
		t.Fatalf("V2 方案下总分应为 88，实际 %d", recordV2.Score)
	}
	if recordV2.SchemeID == nil || *recordV2.SchemeID != schemeV2.ID {
		t.Fatalf("V2 验收应关联 V2 方案，实际 %+v", recordV2.SchemeID)
	}
}

func TestSchemeValidation(t *testing.T) {
	fixture := testsupport.NewFixture(t)

	// 分值上限合计必须等于 100
	req := testsupport.SchemeRequest()
	req.Items[0].MaxScore = 39
	_, err := fixture.Acceptances.CreateScheme(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)

	// 评分项名称不能重复
	req = testsupport.SchemeRequest()
	req.Items[1].Name = req.Items[0].Name
	_, err = fixture.Acceptances.CreateScheme(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)

	// 生效日期不能为空
	req = testsupport.SchemeRequest()
	req.EffectiveFrom = date.Date{}
	_, err = fixture.Acceptances.CreateScheme(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)

	// 新版本生效日期必须晚于现行版本
	fixture.CreateScheme(t, testsupport.SchemeRequest())
	req = testsupport.SchemeRequest()
	_, err = fixture.Acceptances.CreateScheme(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)

	req.EffectiveFrom = date.Today().AddDays(-1)
	_, err = fixture.Acceptances.CreateScheme(context.Background(), req)
	testsupport.RequireAppError(t, err, httpx.CodeValidation)
}

func TestEffectiveSchemeFallsBackToLatestBeforeDate(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	fixture.CreateScheme(t, testsupport.SchemeRequest())

	v2 := testsupport.SchemeRequest()
	v2.Title = "测试评分方案 V2"
	v2.EffectiveFrom = date.Today().AddDays(7)
	fixture.CreateScheme(t, v2)

	// 今天仍适用 V1，7 天后才切换到 V2
	today, err := fixture.Acceptances.EffectiveScheme(context.Background(), date.Today())
	testsupport.RequireNoError(t, err)
	if today == nil || today.Title != "测试评分方案" {
		t.Fatalf("今天应适用 V1 方案，实际 %+v", today)
	}
	future, err := fixture.Acceptances.EffectiveScheme(context.Background(), date.Today().AddDays(7))
	testsupport.RequireNoError(t, err)
	if future == nil || future.Title != "测试评分方案 V2" {
		t.Fatalf("7 天后应适用 V2 方案，实际 %+v", future)
	}
	// 首个方案生效前没有可用方案
	past, err := fixture.Acceptances.EffectiveScheme(context.Background(), date.Today().AddDays(-1))
	testsupport.RequireNoError(t, err)
	if past != nil {
		t.Fatalf("方案生效前应返回 nil，实际 %+v", past)
	}
}

func TestDeleteRemovesScoreDetails(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	scheme := fixture.CreateScheme(t, testsupport.SchemeRequest())
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "待删除的评分任务")

	// 需整改且任务未验收的记录允许删除
	req := testsupport.ItemizedRequest(task.ID, scheme, 20, 10, 10, 8, 5)
	req.Result = acceptance.ResultRework
	req.Issues = "残留淤积厚度超标"
	deadline := date.Today().AddDays(3)
	req.RectifyDeadline = &deadline
	record, err := fixture.Acceptances.Create(context.Background(), req)
	testsupport.RequireNoError(t, err)

	testsupport.RequireNoError(t, fixture.Acceptances.Delete(context.Background(), record.ID))

	// 明细应随记录一并删除，不会混入评分项统计
	var count int64
	testsupport.RequireNoError(t, fixture.DB.Model(&acceptance.AcceptanceScoreDetail{}).
		Where("acceptance_id = ?", record.ID).Count(&count).Error)
	if count != 0 {
		t.Fatalf("删除验收记录后不应残留评分明细，实际 %d 条", count)
	}
}
