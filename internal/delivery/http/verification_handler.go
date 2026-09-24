package http

import (
	"errors"
	"strings"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/jwtutil"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type verificationHandler struct {
	useCase domain.VerificationUseCase
	cfg     *config.Config
}

func NewVerificationHandler(app *fiber.App, useCase domain.VerificationUseCase, cfg *config.Config) {
	handler := &verificationHandler{
		useCase: useCase,
		cfg:     cfg,
	}

	vGroup := app.Group("/api/v1/verification", jwtutil.RequireAuth(cfg))
	vGroup.Post("/submit", handler.Submit)
	vGroup.Put("/address", handler.UpdateAddress)
	vGroup.Get("/status", handler.GetStatus)
	vGroup.Post("/fcm-token", handler.RegisterFCMToken)

	app.Get("/api/v1/auth/me", jwtutil.RequireAuth(cfg), handler.Me)
}

func (h *verificationHandler) Me(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id format"})
	}

	res, err := h.useCase.GetMe(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(strings.ToLower(err.Error()), "record not found") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "user not found or session expired"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "data": res})
}

func (h *verificationHandler) Submit(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id format"})
	}

	var req domain.SubmitVerificationRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.IdentityNumber == "" || req.IdCardPhotoUrl == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "identity_number and id_card_photo_url are required"})
	}

	if err := h.useCase.SubmitVerification(userID, &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "verification request submitted successfully"})
}

func (h *verificationHandler) UpdateAddress(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id format"})
	}

	var req domain.UpdateAddressRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := h.useCase.UpdateAddress(userID, &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "address updated successfully"})
}

func (h *verificationHandler) GetStatus(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id format"})
	}

	res, err := h.useCase.GetVerificationStatus(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "data": res})
}

func (h *verificationHandler) RegisterFCMToken(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id format"})
	}

	var req domain.RegisterFCMTokenRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.DeviceID == "" || req.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "device_id and token are required"})
	}

	if err := h.useCase.RegisterFCMToken(userID, &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "FCM token registered successfully"})
}
