package http

import (
	"strconv"

	"super-app-chonburi-go-mobile/internal/usecase"

	"github.com/gofiber/fiber/v3"
)

type WaterLevelHandler struct {
	useCase usecase.WaterLevelUseCase
}

func NewWaterLevelHandler(app *fiber.App, useCase usecase.WaterLevelUseCase) {
	h := &WaterLevelHandler{
		useCase: useCase,
	}

	registerRoutes := func(g fiber.Router) {
		g.Get("/devices", h.GetStations)
		g.Get("/stations", h.GetStations)
		g.Get("/devices/:serial", h.GetDeviceDetail)
		g.Get("/stations/:serial", h.GetDeviceDetail)
		g.Get("/devices/:serial/history", h.GetDeviceHistory)
		g.Get("/stations/:serial/history", h.GetDeviceHistory)
		g.Get("/devices/:serial/stats", h.GetDeviceHistory)
		g.Get("/devices/:serial/snapshots", h.GetSnapshots)
		g.Get("/stations/:serial/snapshots", h.GetSnapshots)
		g.Post("/devices/:serial/stream", h.InitStream)
		g.Post("/stations/:serial/stream", h.InitStream)
		g.Post("/devices/:serial/webrtc-offer", h.StartWebRTC)
		g.Post("/stations/:serial/webrtc-offer", h.StartWebRTC)
		g.Delete("/devices/:serial/stream", h.TerminateStream)
		g.Delete("/stations/:serial/stream", h.TerminateStream)
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"items":   history,
		"total":   len(history),
	})
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	c.Set("Content-Type", "application/sdp")
	return c.SendString(sdpAnswer)
}

func (h *WaterLevelHandler) GetActiveAlerts(c fiber.Ctx) error {
	ctx := c.Context()
	alerts, err := h.useCase.GetActiveAlerts(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"items":   alerts,
		"total":   len(alerts),
	})
}
