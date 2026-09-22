package dashboard_test

import (
	"context"
	"testing"

	"github.com/drainage/desilting/internal/modules/dashboard"
	"github.com/drainage/desilting/internal/testsupport"
)

// 统计里的评分项明细与总分必须和验收详情同源：
// 总分取验收记录上的快照列，评分项取登记时的明细快照。
func TestScoreItemStatsMatchAcceptanceDetail(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	scheme := fixture.CreateScheme(t, testsupport.SchemeRequest())

	taskA := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "评分任务A")
	recordA, err := fixture.Acceptances.Create(context.Background(),
		testsupport.ItemizedRequest(taskA.ID, scheme, 40, 20, 15, 15, 10))
	testsupport.RequireNoError(t, err)

	taskB := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "评分任务B")
	_, err = fixture.Acceptances.Create(context.Background(),
		testsupport.ItemizedRequest(taskB.ID, scheme, 30, 15, 10, 10, 5))
	testsupport.RequireNoError(t, err)

	// 再登记一条方案生效前的老记录（手工总分）：方案今天生效，验收日期取昨天即走手工总分
	legacyTask := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "评分任务C")
	legacyReq := testsupport.PassRequest(legacyTask.ID, 77)
	legacyReq.AcceptedAt = legacyReq.AcceptedAt.AddDays(-1)
	legacy, err := fixture.Acceptances.Create(context.Background(), legacyReq)
	testsupport.RequireNoError(t, err)

	svc := dashboard.NewService(fixture.DB)
	overview, err := svc.Overview(context.Background())
	testsupport.RequireNoError(t, err)

	// 平均分 = (100 + 70 + 77) / 3 = 82.33，与详情里的总分同出一列
	detailA, err := fixture.Acceptances.Detail(context.Background(), recordA.ID)
	testsupport.RequireNoError(t, err)
	if detailA.Acceptance.Score != 100 {
		t.Fatalf("任务A 总分应为 100，实际 %d", detailA.Acceptance.Score)
	}
	wantAvg := float64(100+70+77) / 3
	if overview.AcceptanceAvgScore < wantAvg-0.01 || overview.AcceptanceAvgScore > wantAvg+0.01 {
		t.Fatalf("验收平均分应为 %.2f，实际 %.2f", wantAvg, overview.AcceptanceAvgScore)
	}

	stats, err := svc.ScoreItemStats(context.Background())
	testsupport.RequireNoError(t, err)
	if len(stats) != len(scheme.Items) {
		t.Fatalf("评分项统计应有 %d 行，实际 %d", len(scheme.Items), len(stats))
	}
	first := stats[0]
	if first.Name != "清淤洁净度" || first.MaxScore != 40 {
		t.Fatalf("第一行应为清淤洁净度（上限 40），实际 %+v", first)
	}
	if first.SampleCount != 2 {
		t.Fatalf("清淤洁净度应有 2 次评分（老记录无明细不参与），实际 %d", first.SampleCount)
	}
	// 两次得分 40 与 30：平均 35，平均扣分 5
	if first.AvgScore != 35 || first.AvgDeduct != 5 {
		t.Fatalf("清淤洁净度平均得分 35 / 平均扣分 5，实际 %+v", first)
	}

	// 老记录没有评分明细，但总分保持原值
	if legacy.Score != 77 {
		t.Fatalf("老记录总分应保持 77，实际 %d", legacy.Score)
	}
}
