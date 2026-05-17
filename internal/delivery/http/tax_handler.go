package http

import (
	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type taxHandler struct {
	uc domain.TaxUseCase
}

func NewTaxHandler(app *fiber.App, uc domain.TaxUseCase) {
	handler := &taxHandler{uc: uc}

	group := app.Group("/api/v1/tax")

	group.Get("/", handler.GetMyTaxes)
	group.Put("/:id/payment", handler.SubmitPayment)
}

func (h *taxHandler) GetMyTaxes(c fiber.Ctx) error {
	// For now, follow the complaint_handler pattern of simulated user or query param
	// But according to requirements, we should use IdentityNumber
	identityNumber := c.Query("identity_number")
	if identityNumber == "" {
		// Fallback to a mock or error
		return c.Status(400).JSON(fiber.Map{"error": "identity_number is required"})
	}

	taxes, err := h.uc.GetMyTaxes(identityNumber)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": taxes})
}

func (h *taxHandler) SubmitPayment(c fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		UserID  string  `json:"user_id"`
		FileUrl *string `json:"file_url"`
	}

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	taxID, _ := uuid.Parse(id)
	uID, _ := uuid.Parse(req.UserID)

	// Update status to 'reviewing' when user submits payment
	if err := h.uc.UpdateTaxStatus(taxID, domain.TaxStatusReviewing, uID, req.FileUrl); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "payment submitted successfully"})
}
