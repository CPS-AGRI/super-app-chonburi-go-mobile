package http

import (
	"strconv"
	"strings"
	"time"

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

	registerRoutes := func(g fiber.Router) {
		g.Get("/stations", h.GetAllStations)
		g.Get("/stations/:imei", h.GetStationByIMEI)
		g.Get("/stations/imei/:imei", h.GetStationByIMEI)
		g.Get("/stations/:imei/forecast", h.GetStationForecast)
		g.Get("/forecast/stations/imei/:imei", h.GetStationForecast)
		g.Get("/stations/:imei/graph", h.GetStationGraph)
		g.Get("/stations/imei/:imei/graph", h.GetStationGraph)
		g.Get("/stations/:imei/stats/monthly", h.GetStationMonthlyStats)
		g.Get("/stations/imei/:imei/stats/monthly", h.GetStationMonthlyStats)
		g.Get("/forecast/subdistrict", h.GetSubdistrictForecast)
		g.Get("/forecast/by-name", h.GetSubdistrictForecast)
		g.Get("/forecast/by-district", h.GetSubdistrictForecast)
		g.Get("/map/frames", h.GetMapFrames)
		g.Get("/map/tiles", h.GetMapTiles)
		g.Get("/map/tiles/range", h.GetMapTilesRange)
		g.Post("/cache/flush", h.FlushCache)
	}

	// Register under /api/v1/weather
	group := app.Group("/api/v1/weather")
	registerRoutes(group)

	// Also alias /api/v1/mobile/weather for mobile client consistency
	mobileGroup := app.Group("/api/v1/mobile/weather")
	registerRoutes(mobileGroup)
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

func (h *WeatherHandler) GetMapTiles(c fiber.Ctx) error {
	variable := c.Query("variable", "nrtrr")
	animType := c.Query("anim_type", "nrt")

	ctx := c.Context()
	data, err := h.useCase.GetMapTiles(ctx, variable, animType)
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

func (h *WeatherHandler) GetMapTilesRange(c fiber.Ctx) error {
	variable := c.Query("variable", "nrtrr")
	animType := c.Query("anim_type", "nrt")
	date := c.Query("date", "")
	minRow, _ := strconv.Atoi(c.Query("min_row", "1"))
	maxRow, _ := strconv.Atoi(c.Query("max_row", "16"))
	minCol, _ := strconv.Atoi(c.Query("min_col", "1"))
	maxCol, _ := strconv.Atoi(c.Query("max_col", "9"))

	ctx := c.Context()
	data, err := h.useCase.GetMapTilesRange(ctx, variable, animType, date, minRow, maxRow, minCol, maxCol)
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

func (h *WeatherHandler) GetStationMonthlyStats(c fiber.Ctx) error {
	imei := c.Params("imei")
	if imei == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "imei parameter is required",
		})
	}

	monthParam := c.Query("month", "")
	yearParam := c.Query("year", "")

	var month, year int
	if strings.Contains(monthParam, "-") {
		parts := strings.Split(monthParam, "-")
		if len(parts) >= 2 {
			year, _ = strconv.Atoi(parts[0])
			month, _ = strconv.Atoi(parts[1])
		}
	} else {
		month, _ = strconv.Atoi(monthParam)
		year, _ = strconv.Atoi(yearParam)
	}

	if year == 0 {
		year = time.Now().Year()
	}
	if month == 0 {
		month = int(time.Now().Month())
	}

	ctx := c.Context()
	data, err := h.useCase.GetStationMonthlyStats(ctx, imei, month, year)
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

func (h *WeatherHandler) GetSubdistrictForecast(c fiber.Ctx) error {
	district := c.Query("district", "")
	if district == "" {
		district = c.Query("amphor", "")
	}
	subdistrict := c.Query("subdistrict", "")
	if subdistrict == "" {
		subdistrict = c.Query("tumbon", "")
	}
	province := c.Query("province", "ชลบุรี")

	if district == "" && subdistrict == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "district or subdistrict query parameter is required",
		})
	}

	ctx := c.Context()
	data, err := h.useCase.GetSubdistrictForecast(ctx, district, subdistrict, province)
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
