package http

import (
	"super-app-chonburi-go-mobile/internal/domain"
	"github.com/gofiber/fiber/v3"
)

type complaintHandler struct {
	useCase domain.ComplaintUseCase
}

func NewComplaintHandler(app *fiber.App, useCase domain.ComplaintUseCase) {
	handler := &complaintHandler{useCase: useCase}

	group := app.Group("/api/v1/complaints")
	
	// TODO: Add Auth Middleware here once created
	group.Get("/", handler.GetMyComplaints)
	group.Post("/", handler.AddComplaint)
	group.Get("/:id", handler.GetDetail)
	group.Post("/:id/cancel", handler.CancelComplaint)
}

func (h *complaintHandler) GetMyComplaints(c fiber.Ctx) error {
	// Mock User ID for now, will get from JWT middleware later
	userID := "1a5152bd-d98f-448c-865c-6f74fc47c1ca" 
	
	status := c.Query("status")
	search := c.Query("search")

	complaints, err := h.useCase.GetMyComplaints(userID, status, search)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": complaints})
}

func (h *complaintHandler) AddComplaint(c fiber.Ctx) error {
	userID := "1a5152bd-d98f-448c-865c-6f74fc47c1ca" 

	var req struct {
		Title        string   `json:"title"`
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
		Title:        req.Title,
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

func (h *complaintHandler) GetDetail(c fiber.Ctx) error {
	userID := "1a5152bd-d98f-448c-865c-6f74fc47c1ca"
	id := c.Params("id")

	complaint, err := h.useCase.GetDetail(id, userID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "complaint not found"})
	}

	return c.JSON(fiber.Map{"data": complaint})
}

func (h *complaintHandler) CancelComplaint(c fiber.Ctx) error {
	userID := "1a5152bd-d98f-448c-865c-6f74fc47c1ca"
	id := c.Params("id")

	if err := h.useCase.CancelComplaint(id, userID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "complaint canceled successfully"})
}
