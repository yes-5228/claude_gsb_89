package dashboard

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/shared/refx"
)

// AcceptanceScoreGateway 验收模块对外提供的评分统计能力（由 acceptance.Service 实现）。
//
// 放在 dashboard 侧定义接口，依赖方向保持为 dashboard -> acceptance，不产生循环依赖。
type AcceptanceScoreGateway interface {
	ScoreSummary(ctx context.Context) (refx.AcceptanceScoreSummary, error)
	ItemScoreStats(ctx context.Context) ([]refx.AcceptanceItemScoreStat, error)
}

// Register 注册看板路由，并返回 service。
func Register(router fiber.Router, db *gorm.DB, acceptanceScores AcceptanceScoreGateway) *Service {
	svc := NewService(db, acceptanceScores)
	RegisterWith(router, svc)
	return svc
}

// RegisterWith 用已装配好的 service 注册看板路由。
func RegisterWith(router fiber.Router, svc *Service) {
	handler := NewHandler(svc)

	group := router.Group("/dashboard")
	group.Get("/overview", handler.Overview)
	group.Get("/district-stats", handler.DistrictStats)
	group.Get("/pending-acceptance", handler.PendingAcceptance)
	group.Get("/recent-records", handler.RecentRecords)
	group.Get("/acceptance-score-stats", handler.AcceptanceScoreStats)
}
