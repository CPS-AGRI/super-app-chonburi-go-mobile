package http

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"super-app-chonburi-go-mobile/internal/domain"
)

type moduleHandler struct {
	useCase domain.ModuleUseCase
}

func NewModuleHandler(app *fiber.App, useCase domain.ModuleUseCase) {
	handler := &moduleHandler{useCase: useCase}

	for _, prefix := range []string{"/api/v1/module-types", "/api/v1/mobile/module-types"} {
		group := app.Group(prefix)
		group.Get("/", handler.GetModuleTypes)
	}
}

func (h *moduleHandler) GetModuleTypes(c fiber.Ctx) error {
	moduleID := c.Query("module_id")

	if moduleID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "module_id is required"})
	}

	moduleTypes, err := h.useCase.GetModuleTypesByModuleID(moduleID)
	if err != nil {
		log.Printf("⚠️ [ModuleHandler] GetModuleTypes failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to load module types"})
	}

	return c.JSON(fiber.Map{"data": moduleTypes})
}
