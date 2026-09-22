package cleaningrecord

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Register 注册清淤记录路由，并返回 service 供其他模块装配依赖。
func Register(router fiber.Router, db *gorm.DB, tasks TaskGateway) *Service {
	svc := NewService(NewRepository(db), tasks)
	RegisterWith(router, svc)
	return svc
}

// RegisterWith 用已装配好的 service 注册清淤记录路由。
func RegisterWith(router fiber.Router, svc *Service) {
	handler := NewHandler(svc)

	group := router.Group("/cleaning-records")
	group.Get("", handler.List)
	group.Post("", handler.Create)
	group.Get("/:id", handler.Detail)
	group.Put("/:id", handler.Update)
	group.Delete("/:id", handler.Delete)
}
