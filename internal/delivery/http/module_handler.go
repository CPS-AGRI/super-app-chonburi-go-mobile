package http

import (
	"super-app-chonburi-go-mobile/internal/domain"
	"github.com/gofiber/fiber/v3"
)

type moduleHandler struct {
	useCase domain.ModuleUseCase
}

func NewModuleHandler(app *fiber.App, useCase domain.ModuleUseCase) {
	handler := &moduleHandler{useCase: useCase}

	group := app.Group("/api/v1/module-types")
	group.Get("/", handler.GetModuleTypes)
}

func (h *moduleHandler) GetModuleTypes(c fiber.Ctx) error {
	moduleID := c.Query("module_id")

	if moduleID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "module_id is required"})
	}

	moduleTypes, err := h.useCase.GetModuleTypesByModuleID(moduleID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": moduleTypes})
}
