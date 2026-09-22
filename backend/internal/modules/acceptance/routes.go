package acceptance

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Register 注册验收记录路由，并返回 service 供看板模块读取。
func Register(router fiber.Router, db *gorm.DB, tasks TaskGateway, segments SegmentGateway, records RecordGateway) *Service {
	svc := NewService(NewRepository(db), tasks, segments, records)
	RegisterWith(router, svc)
	return svc
}

// RegisterWith 用已装配好的 service 注册验收记录路由。
func RegisterWith(router fiber.Router, svc *Service) {
	handler := NewHandler(svc)

	group := router.Group("/acceptances")
	// 静态路径需注册在 /:id 之前，避免 "score-templates" 被当成验收记录 ID。
	group.Get("/score-templates", handler.ListTemplates)
	group.Post("/score-templates", handler.CreateTemplateVersion)
	group.Get("/score-templates/effective", handler.EffectiveTemplate)
	group.Get("", handler.List)
	group.Post("", handler.Create)
	group.Get("/:id", handler.Detail)
	group.Delete("/:id", handler.Delete)
	group.Post("/:id/rectify", handler.Rectify)
}
