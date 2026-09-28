package http

import (
	"fmt"
	"path/filepath"

	"super-app-chonburi-go-mobile/pkg/storage"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type UploadHandler struct {
	storage storage.StorageProvider
}

func NewUploadHandler(app *fiber.App, store storage.StorageProvider) {
	handler := &UploadHandler{storage: store}

	// Standard upload route
	app.Post("/api/v1/upload", handler.UploadFile)

	// Backward compatibility alias
	app.Post("/api/v1/tax-new/upload", handler.UploadFile)
}

func (h *UploadHandler) UploadFile(c fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "failed to read file from form: " + err.Error(),
		})
	}

	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to open file: " + err.Error(),
		})
	}
	defer file.Close()

	ext := filepath.Ext(fileHeader.Filename)
	uniqueFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)

	fileURL, err := h.storage.Upload(file, uniqueFilename)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to upload file: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"file_url": fileURL,
	})
}
