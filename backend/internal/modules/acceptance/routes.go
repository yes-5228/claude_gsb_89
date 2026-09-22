package acceptance

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Register 注册验收记录路由，并返回 service 供看板模块读取。
func Register(router fiber.Router, db *gorm.DB, tasks TaskGateway, segments SegmentGateway, records RecordGateway) *Service {
	svc := NewService(NewRepository(db), tasks, segments, records)
	handler := NewHandler(svc)

	group := router.Group("/acceptances")
	// 评分方案路由登记在 /:id 之前，避免被当成验收记录 ID。
	group.Get("/score-schemes", handler.ListSchemes)
	group.Post("/score-schemes", handler.CreateScheme)
	group.Get("/score-schemes/effective", handler.EffectiveScheme)
	group.Get("", handler.List)
	group.Post("", handler.Create)
	group.Get("/:id", handler.Detail)
	group.Delete("/:id", handler.Delete)
	group.Post("/:id/rectify", handler.Rectify)

	return svc
}
