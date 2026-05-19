package http

import (
	"strconv"
	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/gofiber/fiber/v3"
)

type publicRelationMobileHandler struct {
	uc domain.PublicRelationMobileUseCase
}

func NewPublicRelationMobileHandler(app *fiber.App, uc domain.PublicRelationMobileUseCase) {
	handler := &publicRelationMobileHandler{uc: uc}

	group := app.Group("/api/v1/public-relations")

	// Global Screens
	group.Get("/welcome-screen", handler.GetWelcomeScreen)

	// News Modules
	group.Get("/modules/:moduleId", handler.GetNewsFeed)
	group.Get("/modules/:moduleId/news/:id", handler.GetNewsDetail)

	// Interactions
	group.Post("/news/:id/like", handler.ToggleLike)
	group.Post("/news/:id/comments", handler.AddComment)
	group.Get("/news/:id/comments", handler.GetComments)
	group.Delete("/comments/:commentId", handler.DeleteComment)
	group.Post("/comments/:commentId/report", handler.ReportComment)
	group.Post("/comments/:commentId/hide", handler.HideComment)
}

func (h *publicRelationMobileHandler) GetWelcomeScreen(c fiber.Ctx) error {
	screen, err := h.uc.GetWelcomeScreen()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
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
	userId := c.Query("user_id")

	news, err := h.uc.GetNewsFeed(moduleId, page, limit, userId)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": news})
}

func (h *publicRelationMobileHandler) GetNewsDetail(c fiber.Ctx) error {
	moduleId := c.Params("moduleId")
	id := c.Params("id")
	userId := c.Query("user_id") // Mock user_id if passed, or extract from JWT if available.

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
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	liked, err := h.uc.ToggleLike(id, req.UserID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"liked":   liked,
	})
}

func (h *publicRelationMobileHandler) AddComment(c fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		ModuleID string `json:"module_id"`
		UserID   string `json:"user_id"`
		Comment  string `json:"comment"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	comment, err := h.uc.AddComment(req.ModuleID, id, req.UserID, req.Comment)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"data": comment})
}

func (h *publicRelationMobileHandler) GetComments(c fiber.Ctx) error {
	id := c.Params("id")

	comments, err := h.uc.GetComments(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": comments})
}

func (h *publicRelationMobileHandler) DeleteComment(c fiber.Ctx) error {
	commentId := c.Params("commentId")
	userId := c.Query("user_id")

	if err := h.uc.DeleteComment(commentId, userId); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "comment deleted successfully"})
}

func (h *publicRelationMobileHandler) ReportComment(c fiber.Ctx) error {
	commentId := c.Params("commentId")

	if err := h.uc.ReportComment(commentId); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "comment reported successfully"})
}

func (h *publicRelationMobileHandler) HideComment(c fiber.Ctx) error {
	commentId := c.Params("commentId")

	if err := h.uc.HideComment(commentId); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "comment hidden successfully"})
}
