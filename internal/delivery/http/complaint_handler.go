package http

import (
	"github.com/gofiber/fiber/v3"
	"strconv"
	"super-app-chonburi-go-mobile/internal/domain"
)

type complaintHandler struct {
	useCase domain.ComplaintUseCase
}

func NewComplaintHandler(app *fiber.App, useCase domain.ComplaintUseCase) {
	handler := &complaintHandler{useCase: useCase}

	group := app.Group("/api/v1/complaints")

	group.Get("/", handler.GetMyComplaints)
	group.Post("/", handler.AddComplaint)
	group.Patch("/:id", handler.UpdateComplaint)
	group.Get("/:id", handler.GetDetail)
	group.Delete("/:id", handler.CancelComplaint)
	group.Post("/:id/rate", handler.Rate)
	group.Post("/:id/dispute", handler.Dispute)
}

func (h *complaintHandler) GetMyComplaints(c fiber.Ctx) error {
	userID, _ := h.useCase.GetFirstUserID()
	status := c.Query("status")
	search := c.Query("search")
	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(c.Query("limit", "10"))
	if err != nil || limit < 1 {
		limit = 10
	}

	complaints, err := h.useCase.GetMyComplaints(userID, status, search, page, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": complaints})
}

func (h *complaintHandler) AddComplaint(c fiber.Ctx) error {
	userID, err := h.useCase.GetFirstUserID()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get default user: " + err.Error()})
	}

	var req struct {
		Description  string   `json:"description"`
		ModuleTypeId string   `json:"module_type_id"`
		Latitude     float64  `json:"latitude"`
		Longitude    float64  `json:"longitude"`
		Status       string   `json:"status"`
		Images       []string `json:"images"`
	}

	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	complaint := &domain.Complaint{
		UserId:       userID,
		Description:  req.Description,
		ModuleTypeId: req.ModuleTypeId,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		Status:       req.Status,
		CreatedBy:    userID,
		UpdatedBy:    userID,
	}

	if err := h.useCase.AddComplaint(complaint, req.Images); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"data": complaint})
}

func (h *complaintHandler) UpdateComplaint(c fiber.Ctx) error {
	userID, err := h.useCase.GetFirstUserID()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get default user: " + err.Error()})
	}

	id := c.Params("id")

	var req struct {
		Description  string   `json:"description"`
		ModuleTypeId string   `json:"module_type_id"`
		Latitude     float64  `json:"latitude"`
		Longitude    float64  `json:"longitude"`
		Status       string   `json:"status"`
		Images       []string `json:"images"`
	}

	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	complaint := &domain.Complaint{
		ID:           id,
		UserId:       userID,
		Description:  req.Description,
		ModuleTypeId: req.ModuleTypeId,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		Status:       req.Status,
	}

	if err := h.useCase.UpdateComplaint(complaint, req.Images); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": complaint})
}

func (h *complaintHandler) GetDetail(c fiber.Ctx) error {
	userID, _ := h.useCase.GetFirstUserID()
	id := c.Params("id")

	complaint, err := h.useCase.GetDetail(id, userID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "complaint not found"})
	}

	return c.JSON(fiber.Map{"data": complaint})
}

func (h *complaintHandler) CancelComplaint(c fiber.Ctx) error {
	userID, _ := h.useCase.GetFirstUserID()
	id := c.Params("id")

	if err := h.useCase.DeleteComplaint(id, userID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "complaint deleted successfully"})
}

func (h *complaintHandler) Rate(c fiber.Ctx) error {
	userID, _ := h.useCase.GetFirstUserID()
	id := c.Params("id")

	var req struct {
		Rating  int    `json:"rating"`
		Comment string `json:"comment"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Rating < 1 || req.Rating > 5 {
		return c.Status(400).JSON(fiber.Map{"error": "rating must be between 1 and 5"})
	}

	if err := h.useCase.RateComplaint(id, userID, req.Rating, req.Comment); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "rated successfully"})
}

func (h *complaintHandler) Dispute(c fiber.Ctx) error {
	userID, _ := h.useCase.GetFirstUserID()
	id := c.Params("id")

	var req struct {
		Reason string   `json:"reason"`
		Images []string `json:"images"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := h.useCase.DisputeComplaint(id, userID, req.Reason, req.Images); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "dispute submitted successfully"})
}
