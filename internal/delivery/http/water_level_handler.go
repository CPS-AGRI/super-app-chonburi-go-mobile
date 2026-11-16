package http

import (
	"errors"
	"log"
	"strconv"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/delivery/http/middleware"
	"super-app-chonburi-go-mobile/internal/usecase"
	"super-app-chonburi-go-mobile/pkg/jwtutil"

	"github.com/gofiber/fiber/v3"
)

type WaterLevelHandler struct {
	useCase usecase.WaterLevelUseCase
	cfg     *config.Config
}

func NewWaterLevelHandler(app *fiber.App, useCase usecase.WaterLevelUseCase, cfgs ...*config.Config) {
	var cfg *config.Config
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	h := &WaterLevelHandler{
		useCase: useCase,
		cfg:     cfg,
	}

	var streamAuth fiber.Handler
	if cfg != nil {
		streamAuth = jwtutil.RequireAuth(cfg)
	} else {
		streamAuth = func(c fiber.Ctx) error {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
	}
	streamLimiter := middleware.NewAPIRateLimiter(cfg, 120, 120, time.Minute)

	registerRoutes := func(g fiber.Router) {
		g.Get("/devices", h.GetStations)
		g.Get("/stations", h.GetStations)
		g.Get("/devices/:serial", h.GetDeviceDetail)
		g.Get("/stations/:serial", h.GetDeviceDetail)
		g.Get("/devices/:serial/history", h.GetDeviceHistory)
		g.Get("/stations/:serial/history", h.GetDeviceHistory)
		g.Get("/devices/:serial/forecast", h.GetDeviceForecast)
		g.Get("/stations/:serial/forecast", h.GetDeviceForecast)
		g.Get("/devices/:serial/stats", h.GetDeviceHistory)
		g.Get("/devices/:serial/snapshots", h.GetSnapshots)
		g.Get("/stations/:serial/snapshots", h.GetSnapshots)
		g.Post("/devices/:serial/stream", streamAuth, streamLimiter, h.InitStream)
		g.Post("/stations/:serial/stream", streamAuth, streamLimiter, h.InitStream)
		g.Post("/devices/:serial/webrtc-offer", streamAuth, streamLimiter, h.StartWebRTC)
		g.Post("/stations/:serial/webrtc-offer", streamAuth, streamLimiter, h.StartWebRTC)
		g.Delete("/devices/:serial/stream", streamAuth, streamLimiter, h.TerminateStream)
		g.Delete("/stations/:serial/stream", streamAuth, streamLimiter, h.TerminateStream)
		g.Get("/alerts/active", h.GetActiveAlerts)
	}

	// 1. /api/v1/water-level
	registerRoutes(app.Group("/api/v1/water-level"))

	// 2. /api/v1/mobile/water-level
	registerRoutes(app.Group("/api/v1/mobile/water-level"))

	// 3. /api/v1/flood (compatibility alias)
	registerRoutes(app.Group("/api/v1/flood"))

	// 4. /api/v1/mobile/flood (compatibility alias)
	registerRoutes(app.Group("/api/v1/mobile/flood"))
}

func (h *WaterLevelHandler) GetStations(c fiber.Ctx) error {
	ctx := c.Context()
	stations, err := h.useCase.GetStations(ctx)
	if err != nil {
		log.Printf("⚠️ [WaterLevelHandler] GetStations failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to fetch water monitoring stations",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"items":   stations,
		"total":   len(stations),
	})
}

func (h *WaterLevelHandler) GetDeviceDetail(c fiber.Ctx) error {
	ctx := c.Context()
	serial := c.Params("serial")
	if serial == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "serial is required",
		})
	}

	detail, err := h.useCase.GetDeviceDetail(ctx, serial)
	if err != nil {
		log.Printf("⚠️ [WaterLevelHandler] GetDeviceDetail(%s) failed: %v", serial, err)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   "water monitoring station not found",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    detail,
	})
}

func (h *WaterLevelHandler) GetDeviceHistory(c fiber.Ctx) error {
	ctx := c.Context()
	serial := c.Params("serial")
	if serial == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "serial is required",
		})
	}

	timeframe := 168
	if tfStr := c.Query("timeframe"); tfStr != "" {
		if tf, err := strconv.Atoi(tfStr); err == nil && tf > 0 {
			timeframe = tf
		}
	}

	history, err := h.useCase.GetDeviceHistory(ctx, serial, timeframe)
	if err != nil {
		log.Printf("⚠️ [WaterLevelHandler] GetDeviceHistory(%s) failed: %v", serial, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to fetch water level history",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"items":   history,
		"total":   len(history),
	})
}

func (h *WaterLevelHandler) GetDeviceForecast(c fiber.Ctx) error {
	ctx := c.Context()
	serial := c.Params("serial")
	if serial == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "serial is required",
		})
	}

	days := 1
	if dStr := c.Query("days"); dStr != "" {
		if d, err := strconv.Atoi(dStr); err == nil && (d == 1 || d == 7) {
			days = d
		}
	}
	date := c.Query("date")

	res, err := h.useCase.GetDeviceForecast(ctx, serial, days, date)
	if err != nil {
		log.Printf("⚠️ [WaterLevelHandler] GetDeviceForecast(%s) failed: %v", serial, err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"success": false,
			"error":   "failed to fetch water level forecast",
		})
	}

	return c.JSON(res)
}

func (h *WaterLevelHandler) GetSnapshots(c fiber.Ctx) error {
	ctx := c.Context()
	serial := c.Params("serial")
	if serial == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "serial is required",
		})
	}

	date := c.Query("date")
	logs, err := h.useCase.GetSnapshots(ctx, serial, date)
	if err != nil {
		log.Printf("⚠️ [WaterLevelHandler] GetSnapshots(%s) failed: %v", serial, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to fetch station snapshots",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"items":   logs,
		"total":   len(logs),
	})
}

func (h *WaterLevelHandler) InitStream(c fiber.Ctx) error {
	ctx := c.Context()
	serial := c.Params("serial")
	if serial == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "serial is required",
		})
	}

	res, err := h.useCase.InitStream(ctx, serial)
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidWaterStationSerial) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "water monitoring station not found",
			})
		}
		log.Printf("⚠️ [WaterLevelHandler] InitStream(%s) failed: %v", serial, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to initialize camera stream",
		})
	}

	return c.JSON(res)
}

func (h *WaterLevelHandler) TerminateStream(c fiber.Ctx) error {
	ctx := c.Context()
	serial := c.Params("serial")
	if serial == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "serial is required",
		})
	}

	err := h.useCase.TerminateStream(ctx, serial)
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidWaterStationSerial) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "water monitoring station not found",
			})
		}
		log.Printf("⚠️ [WaterLevelHandler] TerminateStream(%s) failed: %v", serial, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to terminate camera stream",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
	})
}

func (h *WaterLevelHandler) StartWebRTC(c fiber.Ctx) error {
	ctx := c.Context()
	serial := c.Params("serial")
	if serial == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "serial is required",
		})
	}

	sdpOffer := string(c.Body())
	if sdpOffer == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "sdp offer body is required",
		})
	}

	sdpAnswer, err := h.useCase.RelayWebRTCOffer(ctx, serial, sdpOffer)
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidWaterStationSerial) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "water monitoring station not found",
			})
		}
		log.Printf("⚠️ [WaterLevelHandler] StartWebRTC(%s) failed: %v", serial, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to establish camera stream",
		})
	}

	c.Set("Content-Type", "application/sdp")
	return c.SendString(sdpAnswer)
}

func (h *WaterLevelHandler) GetActiveAlerts(c fiber.Ctx) error {
	ctx := c.Context()
	alerts, err := h.useCase.GetActiveAlerts(ctx)
	if err != nil {
		log.Printf("⚠️ [WaterLevelHandler] GetActiveAlerts failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "failed to fetch active flood alerts",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"items":   alerts,
		"total":   len(alerts),
	})
}
