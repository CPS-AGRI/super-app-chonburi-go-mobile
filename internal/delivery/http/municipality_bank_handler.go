package http

import (
	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/gofiber/fiber/v3"
)

type municipalityBankHandler struct {
	uc domain.MunicipalityBankUseCase
}

func NewMunicipalityBankHandler(app *fiber.App, uc domain.MunicipalityBankUseCase) {
	handler := &municipalityBankHandler{uc: uc}

	for _, prefix := range []string{"/api/v1/municipalities/bank", "/api/v1/mobile/municipalities/bank"} {
		group := app.Group(prefix)
		group.Get("/default", handler.GetActiveBank)
	}
}

func (h *municipalityBankHandler) GetActiveBank(c fiber.Ctx) error {
	bank, err := h.uc.GetActiveBank()
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"error":   "Default bank not found",
		})
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data":    bank,
	})
}
