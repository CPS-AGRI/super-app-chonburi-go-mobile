package http

import (
	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/database"
	"super-app-chonburi-go-mobile/pkg/jwtutil"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type notificationHandler struct {
	uc domain.NotificationUseCase
}

func NewNotificationHandler(app *fiber.App, uc domain.NotificationUseCase, cfg *config.Config) {
	handler := &notificationHandler{uc: uc}

	group := app.Group("/api/v1/notifications")

	group.Get("/", handler.GetNotifications)
	group.Post("/read-all", handler.MarkAllAsRead)
	group.Post("/:id/read", handler.MarkAsRead)
	group.Post("/token", jwtutil.RequireAuth(cfg), handler.RegisterToken)
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

func (h *notificationHandler) RegisterToken(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ID ผู้ใช้งานไม่ถูกต้อง"})
	}

	var req struct {
		Token      string `json:"token"`
		DeviceType string `json:"device_type"`
	}

	if err := c.Bind().JSON(&req); err != nil || req.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "กรุณาระบุ Token ให้ถูกต้อง"})
	}

	if req.DeviceType == "" {
		req.DeviceType = "android"
	}

	db := database.DB
	if db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "ระบบฐานข้อมูลไม่ได้เชื่อมต่อ"})
	}

	var deviceToken domain.ModuleDeviceToken

	err = db.Where("token = ?", req.Token).First(&deviceToken).Error
	if err == nil {

		deviceToken.UserID = userUUID
		deviceToken.DeviceType = req.DeviceType
		deviceToken.UpdatedDate = time.Now()
		if err := db.Save(&deviceToken).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "ไม่สามารถบันทึก Token ได้"})
		}
	} else {

		newDeviceToken := domain.ModuleDeviceToken{
			ID:          uuid.New(),
			UserID:      userUUID,
			Token:       req.Token,
			DeviceType:  req.DeviceType,
			CreatedDate: time.Now(),
			UpdatedDate: time.Now(),
		}
		if err := db.Create(&newDeviceToken).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "ไม่สามารถลงทะเบียน Token ใหม่ได้"})
		}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "บันทึกและจับคู่ Device Token สำเร็จ",
	})
}
