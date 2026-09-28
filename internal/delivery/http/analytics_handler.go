package http

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/jwtutil"
)

type analyticsHandler struct {
	uc  domain.AnalyticsUseCase
	cfg *config.Config
}

func NewAnalyticsHandler(app *fiber.App, uc domain.AnalyticsUseCase, cfg *config.Config) {
	handler := &analyticsHandler{uc: uc, cfg: cfg}

	// Register on both base path and api/v1 for maximum client compatibility
	app.Post("/analytics/module-usage", handler.RecordModuleUsage)
	app.Post("/api/v1/analytics/module-usage", handler.RecordModuleUsage)
}

func (h *analyticsHandler) RecordModuleUsage(c fiber.Ctx) error {
	var req domain.RecordModuleUsageDTO
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid payload format"})
	}

	if req.ModuleCode == "" {
		return c.Status(400).JSON(fiber.Map{"error": "moduleId is required"})
	}

	// Extract User ID from Authorization Token if available
	var userUUID *uuid.UUID
	authHeader := c.Get("Authorization")
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if claims, err := jwtutil.ParseMobileToken(tokenStr, h.cfg.JWTSecret); err == nil && claims != nil {
			if parsedID, parseErr := uuid.Parse(claims.UserID); parseErr == nil {
				userUUID = &parsedID
			}
		}
	}

	ip := c.IP()
	userAgent := c.Get("User-Agent")

	if err := h.uc.TrackModuleUsage(&req, userUUID, ip, userAgent); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Module usage tracked successfully",
	})
}
