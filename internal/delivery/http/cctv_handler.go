package http

import (
	"strconv"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/jwtutil"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type cctvHandler struct {
	useCase domain.MobileCCTVUseCase
}

// NewCCTVHandler registers the Mobile CCTV endpoints.
func NewCCTVHandler(app *fiber.App, useCase domain.MobileCCTVUseCase, cfg *config.Config) {
	handler := &cctvHandler{
		useCase: useCase,
	}

	group := app.Group("/api/v1/mobile/cctv", jwtutil.RequireAuth(cfg))
	group.Get("/", handler.GetCameras)
	group.Post("/requests", handler.CreateRequest)
	group.Get("/requests/my-requests", handler.GetMyRequests)
}

func (h *cctvHandler) GetCameras(c fiber.Ctx) error {
	latMin, _ := strconv.ParseFloat(c.Query("lat_min"), 64)
	latMax, _ := strconv.ParseFloat(c.Query("lat_max"), 64)
	lngMin, _ := strconv.ParseFloat(c.Query("lng_min"), 64)
	lngMax, _ := strconv.ParseFloat(c.Query("lng_max"), 64)

	query := domain.MobileCCTVQuery{
		LatMin: latMin,
		LatMax: latMax,
		LngMin: lngMin,
		LngMax: lngMax,
	}

	cameras, err := h.useCase.GetCameras(query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "data": cameras})
}

type createRequestInput struct {
	CCTVID          string `json:"cctv_id"`
	IncidentDate    string `json:"incident_date"` // format: YYYY-MM-DD
	StartTime       string `json:"start_time"`
	EndTime         string `json:"end_time"`
	Reason          string `json:"reason"`
	EvidenceFileURL string `json:"evidence_file_url"`
}

func (h *cctvHandler) CreateRequest(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}

	var input createRequestInput
	if err := c.Bind().JSON(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	cctvID, err := uuid.Parse(input.CCTVID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid camera ID"})
	}

	incidentDate, err := time.Parse("2006-01-02", input.IncidentDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid incident_date format, must be YYYY-MM-DD"})
	}

	req := &domain.CCTVRequest{
		UserID:          userID,
		CCTVID:          cctvID,
		IncidentDate:    incidentDate,
		StartTime:       input.StartTime,
		EndTime:         input.EndTime,
		Reason:          input.Reason,
		EvidenceFileURL: input.EvidenceFileURL,
	}

	if err := h.useCase.CreateRequest(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "CCTV request submitted successfully", "data": req})
}

func (h *cctvHandler) GetMyRequests(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}

	requests, err := h.useCase.GetMyRequests(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "data": requests})
}
