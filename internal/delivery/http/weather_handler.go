package http

import (
	"strconv"

	"super-app-chonburi-go-mobile/internal/usecase"

	"github.com/gofiber/fiber/v3"
)

type WeatherHandler struct {
	useCase usecase.WeatherUseCase
}

func NewWeatherHandler(app *fiber.App, useCase usecase.WeatherUseCase) {
	h := &WeatherHandler{
		useCase: useCase,
	}

	// Register under /api/v1/weather
	group := app.Group("/api/v1/weather")
	group.Get("/stations", h.GetAllStations)
	group.Get("/stations/:imei", h.GetStationByIMEI)
	group.Get("/stations/:imei/forecast", h.GetStationForecast)
	group.Get("/stations/:imei/graph", h.GetStationGraph)
	group.Get("/map/frames", h.GetMapFrames)
	group.Post("/cache/flush", h.FlushCache)

	// Also alias /api/v1/mobile/weather for mobile client consistency
	mobileGroup := app.Group("/api/v1/mobile/weather")
	mobileGroup.Get("/stations", h.GetAllStations)
	mobileGroup.Get("/stations/:imei", h.GetStationByIMEI)
	mobileGroup.Get("/stations/:imei/forecast", h.GetStationForecast)
	mobileGroup.Get("/stations/:imei/graph", h.GetStationGraph)
	mobileGroup.Get("/map/frames", h.GetMapFrames)
}

func (h *WeatherHandler) GetAllStations(c fiber.Ctx) error {
	ctx := c.Context()
	result, err := h.useCase.GetAllStations(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"items":      result.Stations,
		"total":      result.Total,
		"cached_at":  result.CachedAt,
	})
}

func (h *WeatherHandler) GetStationByIMEI(c fiber.Ctx) error {
	imei := c.Params("imei")
	if imei == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "imei parameter is required",
		})
	}

	ctx := c.Context()
	data, err := h.useCase.GetStationByIMEI(ctx, imei)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

func (h *WeatherHandler) GetStationForecast(c fiber.Ctx) error {
	imei := c.Params("imei")
	if imei == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "imei parameter is required",
		})
	}

	ctx := c.Context()
	data, err := h.useCase.GetStationForecast(ctx, imei)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

func (h *WeatherHandler) GetStationGraph(c fiber.Ctx) error {
	imei := c.Params("imei")
	if imei == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "imei parameter is required",
		})
	}

	variable := c.Query("variable", "temp")
	timeframe, _ := strconv.Atoi(c.Query("timeframe", "24"))
	date := c.Query("date", "")

	ctx := c.Context()
	data, err := h.useCase.GetStationGraph(ctx, imei, variable, timeframe, date)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

func (h *WeatherHandler) GetMapFrames(c fiber.Ctx) error {
	variable := c.Query("variable", "nrtrr")
	animType := c.Query("anim_type", "nrt")
	date := c.Query("date", "")

	ctx := c.Context()
	data, err := h.useCase.GetMapFrames(ctx, variable, animType, date)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

func (h *WeatherHandler) FlushCache(c fiber.Ctx) error {
	ctx := c.Context()
	if err := h.useCase.FlushCache(ctx); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"success": true,
		"message": "Weather stations cache cleared",
	})
}
