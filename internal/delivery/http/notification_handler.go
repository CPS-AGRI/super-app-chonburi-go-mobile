package http

import (
	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/jwtutil"

	"github.com/gofiber/fiber/v3"
)

type notificationHandler struct {
	uc domain.NotificationUseCase
}

func NewNotificationHandler(app *fiber.App, uc domain.NotificationUseCase, cfg *config.Config) {
	handler := &notificationHandler{uc: uc}

	for _, prefix := range []string{"/api/v1/notifications", "/api/v1/mobile/notifications"} {
		group := app.Group(prefix, jwtutil.RequireAuth(cfg))
		group.Get("/", handler.GetNotifications)
		group.Post("/read-all", handler.MarkAllAsRead)
		group.Post("/:id/read", handler.MarkAsRead)
	}
}

func (h *notificationHandler) GetNotifications(c fiber.Ctx) error {
	userID, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	notifications, err := h.uc.GetNotifications(userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": notifications})
}

func (h *notificationHandler) MarkAsRead(c fiber.Ctx) error {
	userID, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	id := c.Params("id")

	if err := h.uc.MarkAsRead(userID, id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "marked as read successfully"})
}

func (h *notificationHandler) MarkAllAsRead(c fiber.Ctx) error {
	userID, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.uc.MarkAllAsRead(userID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "all marked as read successfully"})
}


