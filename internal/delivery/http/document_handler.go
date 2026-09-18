package http

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"
)

type DocumentHandler struct {
	docDir string
}

func NewDocumentHandler(app *fiber.App) {
	// Look for assets/documents relative to working dir or binary
	docDir := "./assets/documents"
	if _, err := os.Stat(docDir); os.IsNotExist(err) {
		// Fallback check parent dirs
		if _, err := os.Stat("../assets/documents"); err == nil {
			docDir = "../assets/documents"
		}
	}

	h := &DocumentHandler{docDir: docDir}

	group := app.Group("/api/v1/documents")
	group.Get("/terms", h.GetTerms)
	group.Get("/privacy", h.GetPrivacy)
	group.Get("/file/:filename", h.ServeFile)
}

func (h *DocumentHandler) GetTerms(c fiber.Ctx) error {
	txtPath := filepath.Join(h.docDir, "term-and-condition.txt")
	contentBytes, err := os.ReadFile(txtPath)
	content := ""
	if err == nil {
		content = string(contentBytes)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"type":        "terms",
			"title":       "ข้อกำหนดและเงื่อนไขการใช้งาน (Terms & Conditions)",
			"pdf_url":     "/api/v1/documents/file/term-and-condition.pdf",
			"content":     strings.TrimSpace(content),
		},
	})
}

func (h *DocumentHandler) GetPrivacy(c fiber.Ctx) error {
	txtPath := filepath.Join(h.docDir, "privacy.txt")
	contentBytes, err := os.ReadFile(txtPath)
	content := ""
	if err == nil {
		content = string(contentBytes)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"type":        "privacy",
			"title":       "นโยบายความเป็นส่วนตัว (Privacy Policy)",
			"pdf_url":     "/api/v1/documents/file/privacy.pdf",
			"content":     strings.TrimSpace(content),
		},
	})
}

func (h *DocumentHandler) ServeFile(c fiber.Ctx) error {
	filename := c.Params("filename")
	// Prevent path traversal
	cleanName := filepath.Base(filename)
	if cleanName != filename || cleanName == "." || cleanName == ".." {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid filename",
		})
	}

	filePath := filepath.Join(h.docDir, cleanName)
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) || fileInfo.IsDir() {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   "Document not found",
		})
	}

	if strings.HasSuffix(cleanName, ".pdf") {
		c.Set("Content-Type", "application/pdf")
	}
	c.Set("Cache-Control", "public, max-age=86400")

	return c.SendFile(filePath)
}
