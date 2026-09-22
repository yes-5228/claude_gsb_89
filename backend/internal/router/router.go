// Package router 负责把各业务模块的路由装配到 Fiber 应用上。
package router

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/config"
	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/modules/acceptance"
	"github.com/drainage/desilting/internal/modules/cleaningrecord"
	"github.com/drainage/desilting/internal/modules/cleaningtask"
	"github.com/drainage/desilting/internal/modules/dashboard"
	"github.com/drainage/desilting/internal/modules/meta"
	"github.com/drainage/desilting/internal/modules/pipesegment"
)

// startedAt 记录进程启动时间，用于健康检查展示运行时长。
var startedAt = time.Now()

// Services 按生产依赖顺序装配好的各模块服务。
type Services struct {
	Segments    *pipesegment.Service
	Tasks       *cleaningtask.Service
	Records     *cleaningrecord.Service
	Acceptances *acceptance.Service
	Dashboard   *dashboard.Service
}

// BuildServices 显式装配各模块服务，不绑定路由。
// 供启动阶段（写入演示数据前初始化默认评分模板）与路由注册复用。
func BuildServices(db *gorm.DB) *Services {
	segmentService := pipesegment.NewService(pipesegment.NewRepository(db))
	taskService := cleaningtask.NewService(cleaningtask.NewRepository(db), segmentService)
	recordService := cleaningrecord.NewService(cleaningrecord.NewRepository(db), taskService)
	acceptanceService := acceptance.NewService(acceptance.NewRepository(db), taskService, segmentService, recordService)
	dashboardService := dashboard.NewService(db, acceptanceService)
	return &Services{
		Segments:    segmentService,
		Tasks:       taskService,
		Records:     recordService,
		Acceptances: acceptanceService,
		Dashboard:   dashboardService,
	}
}

// EnsureDefaultScoreTemplate 保证至少存在一版验收评分模板。
func EnsureDefaultScoreTemplate(svc *acceptance.Service) {
	if err := svc.EnsureDefaultTemplate(context.Background()); err != nil {
		slog.Error("初始化验收评分默认模板失败", "error", err)
	}
}

// Setup 注册健康检查与全部业务模块路由。
//
// 模块之间的依赖在这里显式装配：清淤记录依赖清淤任务，验收依赖任务、
// 清淤记录与管段台账，看板只读依赖全部模块。
func Setup(app *fiber.App, db *gorm.DB, cfg *config.Config) {
	SetupWithServices(app, db, cfg, BuildServices(db))
}

// SetupWithServices 用启动阶段已装配好的服务注册健康检查与全部业务模块路由。
func SetupWithServices(app *fiber.App, db *gorm.DB, cfg *config.Config, services *Services) {
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return httpx.OK(c, fiber.Map{
			"status": "ok",
			"env":    cfg.AppEnv,
			"db":     cfg.Describe(),
			"uptime": time.Since(startedAt).Round(time.Second).String(),
		})
	})

	api := app.Group("/api/v1")
	meta.Register(api)

	pipesegment.RegisterWith(api, services.Segments)
	cleaningtask.RegisterWith(api, services.Tasks)
	cleaningrecord.RegisterWith(api, services.Records)
	acceptance.RegisterWith(api, services.Acceptances)
	dashboard.RegisterWith(api, services.Dashboard)
}
