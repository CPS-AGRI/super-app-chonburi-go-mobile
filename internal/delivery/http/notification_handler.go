package http

import (
	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/gofiber/fiber/v3"
)

type notificationHandler struct {
	uc domain.NotificationUseCase
}

func NewNotificationHandler(app *fiber.App, uc domain.NotificationUseCase) {
	handler := &notificationHandler{uc: uc}

	group := app.Group("/api/v1/notifications")

	group.Get("/", handler.GetNotifications)
	group.Post("/read-all", handler.MarkAllAsRead)
	group.Post("/:id/read", handler.MarkAsRead)
}

func (h *notificationHandler) GetNotifications(c fiber.Ctx) error {
	userID := c.Query("user_id")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "user_id query param is required"})
	}

	notifications, err := h.uc.GetNotifications(userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": notifications})
}

func (h *notificationHandler) MarkAsRead(c fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		UserID string `json:"user_id"`
	}

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.UserID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "user_id is required"})
	}

	if err := h.uc.MarkAsRead(req.UserID, id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "marked as read successfully"})
}

func (h *notificationHandler) MarkAllAsRead(c fiber.Ctx) error {
	var req struct {
		UserID string `json:"user_id"`
	}

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.UserID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "user_id is required"})
	}

	if err := h.uc.MarkAllAsRead(req.UserID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "all marked as read successfully"})
}
