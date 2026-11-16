package http

import (
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/delivery/http/middleware"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/jwtutil"

	"github.com/gofiber/fiber/v3"
)

type publicRelationMobileHandler struct {
	uc  domain.PublicRelationMobileUseCase
	cfg *config.Config
}

func NewPublicRelationMobileHandler(app *fiber.App, uc domain.PublicRelationMobileUseCase, cfgs ...*config.Config) {
	var cfg *config.Config
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	handler := &publicRelationMobileHandler{uc: uc, cfg: cfg}
	authMiddleware := jwtutil.RequireAuth(cfg)
	telemetryLimiter := middleware.NewAPIRateLimiter(cfg, 180, 240, time.Minute)

	for _, prefix := range []string{"/api/v1/public-relations", "/api/v1/mobile/public-relations"} {
		group := app.Group(prefix)

		group.Get("/welcome-screen", handler.GetWelcomeScreen)

		group.Get("/modules/:moduleId", handler.GetNewsFeed)
		group.Get("/modules/:moduleId/news/:id", handler.GetNewsDetail)

		group.Post("/news/:id/like", authMiddleware, handler.ToggleLike)
		group.Post("/news/:id/view", telemetryLimiter, handler.RecordNewsView)
		group.Post("/news/:id/comments", authMiddleware, handler.AddComment)
		group.Get("/news/:id/comments", handler.GetComments)
		group.Delete("/comments/:commentId", authMiddleware, handler.DeleteComment)
		group.Post("/comments/:commentId/report", authMiddleware, handler.ReportComment)
		group.Post("/comments/:commentId/hide", authMiddleware, handler.HideComment)
	}
}

func (h *publicRelationMobileHandler) GetWelcomeScreen(c fiber.Ctx) error {
	screen, err := h.uc.GetWelcomeScreen()
	if err != nil {
		log.Printf("⚠️ [PublicRelationHandler] GetWelcomeScreen failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to load welcome screen"})
	}
	if screen == nil {
		return c.JSON(fiber.Map{"data": nil})
	}
	return c.JSON(fiber.Map{"data": screen})
}

func (h *publicRelationMobileHandler) GetNewsFeed(c fiber.Ctx) error {
	moduleId := c.Params("moduleId")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	userId := c.Query("user_id")

	news, err := h.uc.GetNewsFeed(moduleId, page, limit, userId)
	if err != nil {
		log.Printf("⚠️ [PublicRelationHandler] GetNewsFeed failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to load news feed"})
	}

	return c.JSON(fiber.Map{"data": news})
}

func (h *publicRelationMobileHandler) GetNewsDetail(c fiber.Ctx) error {
	moduleId := c.Params("moduleId")
	id := c.Params("id")
	userId := c.Query("user_id")

	news, liked, err := h.uc.GetNewsDetail(moduleId, id, userId)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  news,
		"liked": liked,
	})
}

func (h *publicRelationMobileHandler) ToggleLike(c fiber.Ctx) error {
	id := c.Params("id")
	authUserID, err := jwtutil.ExtractUserID(c)
	if err != nil || authUserID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		UserID string `json:"user_id"`
	}
	_ = c.Bind().JSON(&req)
	if req.UserID != "" && req.UserID != authUserID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "user_id mismatch with authenticated user"})
	}

	liked, err := h.uc.ToggleLike(id, authUserID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"liked":   liked,
	})
}

func (h *publicRelationMobileHandler) RecordNewsView(c fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		UserID    string `json:"user_id"`
		SessionID string `json:"session_id"`
	}
	_ = c.Bind().JSON(&req)

	if err := h.uc.RecordNewsView(id, req.UserID, req.SessionID); err != nil {
		log.Printf("⚠️ [PublicRelationHandler] RecordNewsView failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to record news view"})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *publicRelationMobileHandler) AddComment(c fiber.Ctx) error {
	id := c.Params("id")
	authUserID, err := jwtutil.ExtractUserID(c)
	if err != nil || authUserID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		ModuleID string `json:"module_id"`
		UserID   string `json:"user_id"`
		Comment  string `json:"comment"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.UserID != "" && req.UserID != authUserID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "user_id mismatch with authenticated user"})
	}
	if strings.TrimSpace(req.Comment) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "comment is required"})
	}

	comment, err := h.uc.AddComment(req.ModuleID, id, authUserID, req.Comment)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"data": comment})
}

func (h *publicRelationMobileHandler) GetComments(c fiber.Ctx) error {
	id := c.Params("id")

	comments, err := h.uc.GetComments(id)
	if err != nil {
		log.Printf("⚠️ [PublicRelationHandler] GetComments failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to fetch comments"})
	}

	return c.JSON(fiber.Map{"data": comments})
}

func (h *publicRelationMobileHandler) DeleteComment(c fiber.Ctx) error {
	commentId := c.Params("commentId")
	authUserID, err := jwtutil.ExtractUserID(c)
	if err != nil || authUserID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	queryUserID := c.Query("user_id")
	if queryUserID != "" && queryUserID != authUserID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "user_id mismatch with authenticated user"})
	}

	if err := h.uc.DeleteComment(commentId, authUserID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "unable to delete comment"})
	}

	return c.JSON(fiber.Map{"message": "comment deleted successfully"})
}

func (h *publicRelationMobileHandler) ReportComment(c fiber.Ctx) error {
	commentId := c.Params("commentId")
	authUserID, err := jwtutil.ExtractUserID(c)
	if err != nil || authUserID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	if err := h.uc.ReportComment(commentId, authUserID); err != nil {
		switch {
		case errors.Is(err, domain.ErrCommentNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "comment not found"})
		case errors.Is(err, domain.ErrCannotReportOwnComment):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot report your own comment"})
		case errors.Is(err, domain.ErrInvalidCommentInput):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid comment id"})
		default:
			log.Printf("⚠️ [PublicRelationHandler] ReportComment failed: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to report comment"})
		}
	}

	return c.JSON(fiber.Map{"message": "comment reported successfully"})
}

func (h *publicRelationMobileHandler) HideComment(c fiber.Ctx) error {
	commentId := c.Params("commentId")
	authUserID, err := jwtutil.ExtractUserID(c)
	if err != nil || authUserID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	if err := h.uc.HideComment(commentId, authUserID); err != nil {
		if strings.Contains(err.Error(), "forbidden") {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "comment not found or forbidden"})
		}
		log.Printf("⚠️ [PublicRelationHandler] HideComment failed: %v", err)
		return c.Status(400).JSON(fiber.Map{"error": "unable to hide comment"})
	}

	return c.JSON(fiber.Map{"message": "comment hidden successfully"})
}
