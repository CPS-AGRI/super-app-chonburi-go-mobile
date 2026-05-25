package http

import (
	"fmt"
	"path/filepath"

	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/storage"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// TaxNewMobileHandler handles mobile API requests for the self-declaration tax module.
type TaxNewMobileHandler struct {
	uc      domain.TaxNewMobileUseCase
	storage storage.StorageProvider
}

// NewTaxNewMobileHandler creates a new handler for mobile tax APIs.
func NewTaxNewMobileHandler(app *fiber.App, uc domain.TaxNewMobileUseCase, store storage.StorageProvider) {
	handler := &TaxNewMobileHandler{uc: uc, storage: store}

	group := app.Group("/api/v1/tax-new")
	group.Get("/business/:reg_number", handler.GetBusiness)
	group.Post("/declare", handler.DeclareTax)
	group.Get("/declare/:id", handler.GetDeclaration)
	group.Post("/upload", handler.UploadFile)
}

// GetBusiness searches for a registered business by registration number.
// GET /api/v1/tax-new/business/:reg_number
func (h *TaxNewMobileHandler) GetBusiness(c fiber.Ctx) error {
	regNumber := c.Params("reg_number")
	if regNumber == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "registration number is required"})
	}

	business, err := h.uc.GetBusiness(regNumber)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    business,
	})
}

// DeclareTax submits a new tax self-declaration.
// POST /api/v1/tax-new/declare
func (h *TaxNewMobileHandler) DeclareTax(c fiber.Ctx) error {
	var req domain.DeclareTaxRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body: " + err.Error()})
	}

	resp, err := h.uc.DeclareTax(req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    resp,
	})
}

// GetDeclaration fetches payment status and QR code of an existing declaration.
// GET /api/v1/tax-new/declare/:id
func (h *TaxNewMobileHandler) GetDeclaration(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid declaration ID"})
	}

	declaration, err := h.uc.GetDeclaration(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if declaration == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "declaration not found"})
	}

	businessName := ""
	if declaration.Business != nil {
		businessName = declaration.Business.NameTH
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"declaration_id":  declaration.ID,
			"business_name":   businessName,
			"tax_month":       declaration.TaxMonth,
			"tax_year":        declaration.TaxYear,
			"calculated_tax":  declaration.CalculatedTax,
			"payment_status":  declaration.PaymentStatus,
			"paid_at":         declaration.PaidAt,
			"ref1":            declaration.Ref1,
			"ref2":            declaration.Ref2,
			"qr_code_content": declaration.QRCodeContent,
			"monthly_revenue": declaration.MonthlyRevenue,
		},
	})
}

// UploadFile handles PDF/image form submission uploads.
// POST /api/v1/tax-new/upload
func (h *TaxNewMobileHandler) UploadFile(c fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to read file from form: " + err.Error()})
	}

	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to open file: " + err.Error()})
	}
	defer file.Close()

	ext := filepath.Ext(fileHeader.Filename)
	uniqueFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)

	fileURL, err := h.storage.Upload(file, uniqueFilename)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to upload file: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"file_url": fileURL,
	})
}
