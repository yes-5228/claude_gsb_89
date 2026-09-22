package acceptance

import (
	"github.com/gofiber/fiber/v2"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/shared/date"
)

// Handler 验收记录 HTTP 接口。
type Handler struct {
	svc *Service
}

// NewHandler 构造处理器。
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// List 验收记录列表。
func (h *Handler) List(c *fiber.Ctx) error {
	query, err := ParseListQuery(c)
	if err != nil {
		return err
	}
	items, total, err := h.svc.List(c.UserContext(), query)
	if err != nil {
		return err
	}
	return httpx.OKPage(c, items, total, query.Page.Page, query.Page.PageSize)
}

// Create 登记验收。
func (h *Handler) Create(c *fiber.Ctx) error {
	var req SaveRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	record, err := h.svc.Create(c.UserContext(), req)
	if err != nil {
		return err
	}
	return httpx.Created(c, record)
}

// Detail 验收详情。
func (h *Handler) Detail(c *fiber.Ctx) error {
	id, err := httpx.PathID(c, "id", "验收记录")
	if err != nil {
		return err
	}
	detail, err := h.svc.Detail(c.UserContext(), id)
	if err != nil {
		return err
	}
	return httpx.OK(c, detail)
}

// Rectify 登记整改完成。
func (h *Handler) Rectify(c *fiber.Ctx) error {
	id, err := httpx.PathID(c, "id", "验收记录")
	if err != nil {
		return err
	}
	var req RectifyRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	record, err := h.svc.Rectify(c.UserContext(), id, req)
	if err != nil {
		return err
	}
	return httpx.Message(c, "整改完成已登记，可重新提交完工报验", record)
}

// ListSchemes 评分方案版本列表。
func (h *Handler) ListSchemes(c *fiber.Ctx) error {
	schemes, err := h.svc.ListSchemes(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, schemes)
}

// EffectiveScheme 查询指定日期生效的评分方案（默认今天），无生效方案时 data 为 null。
func (h *Handler) EffectiveScheme(c *fiber.Ctx) error {
	day := date.Today()
	if raw := httpx.TrimmedQuery(c, "date"); raw != "" {
		parsed, err := date.Parse(raw)
		if err != nil {
			return httpx.BadRequest("日期格式不正确，应为 YYYY-MM-DD")
		}
		day = parsed
	}
	scheme, err := h.svc.EffectiveScheme(c.UserContext(), day)
	if err != nil {
		return err
	}
	return httpx.OK(c, scheme)
}

// CreateScheme 调整评分项：新增方案版本，自生效日期起生效。
func (h *Handler) CreateScheme(c *fiber.Ctx) error {
	var req SaveSchemeRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	scheme, err := h.svc.CreateScheme(c.UserContext(), req)
	if err != nil {
		return err
	}
	return httpx.Created(c, scheme)
}

// Delete 删除验收记录。
func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := httpx.PathID(c, "id", "验收记录")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.UserContext(), id); err != nil {
		return err
	}
	return httpx.Message(c, "验收记录已删除", fiber.Map{"id": id})
}
