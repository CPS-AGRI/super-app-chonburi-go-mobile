package http

import (
	"bytes"
	"fmt"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"golang.org/x/image/webp"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/pkg/jwtutil"
	"super-app-chonburi-go-mobile/pkg/storage"
)

const maxUploadFileSize = 10 * 1024 * 1024 // 10MB limit

type UploadHandler struct {
	storage storage.StorageProvider
	cfg     *config.Config
}

func NewUploadHandler(app *fiber.App, store storage.StorageProvider, cfg *config.Config) {
	handler := &UploadHandler{storage: store, cfg: cfg}

	authMiddleware := jwtutil.RequireAuth(cfg)

	// Standard upload routes (Protected)
	app.Post("/api/v1/upload", authMiddleware, handler.UploadFile)
	app.Post("/api/v1/mobile/upload", authMiddleware, handler.UploadFile)

	// Backward compatibility aliases
	app.Post("/upload", authMiddleware, handler.UploadFile)
	app.Post("/api/v1/tax-new/upload", authMiddleware, handler.UploadFile)
}

func (h *UploadHandler) UploadFile(c fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "failed to read file from form: " + err.Error(),
		})
	}

	// 1. Enforce Max File Size (10MB)
	if fileHeader.Size > maxUploadFileSize {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "file size exceeds maximum allowed limit of 10MB",
		})
	}

	rawExt := strings.ToLower(filepath.Ext(fileHeader.Filename))

	// 2. Extension Whitelist
	allowedExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".pdf":  true,
		".heic": true,
		".heif": true,
		".webp": true,
	}
	if !allowedExts[rawExt] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid file format. Allowed formats: .jpg, .jpeg, .png, .pdf, .heic, .heif, .webp",
		})
	}

	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to open file: " + err.Error(),
		})
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to read file content: " + err.Error(),
		})
	}

	// 3. MIME Sniffing & Magic Bytes Check
	sniffHeader := fileBytes
	if len(sniffHeader) > 512 {
		sniffHeader = sniffHeader[:512]
	}
	detectedMime := http.DetectContentType(sniffHeader)

	// iOS HEIC/HEIF check (ISO Base Media File Format box ftypheic/ftypmif1/ftypmsf1)
	isHeic := rawExt == ".heic" || rawExt == ".heif" || isHeicBytes(sniffHeader)
	isWebp := rawExt == ".webp" || strings.HasPrefix(detectedMime, "image/webp")

	var finalReader io.Reader
	var finalFilename string

	// 4. iOS HEIC/HEIF Auto-transcode to Standard High-Performance JPEG
	if isWebp {
		// Transcode WebP to standard JPEG for universal cross-platform rendering
		img, decodeErr := webp.Decode(bytes.NewReader(fileBytes))
		if decodeErr != nil {
			finalReader = bytes.NewReader(fileBytes)
			finalFilename = fmt.Sprintf("%s%s", uuid.New().String(), rawExt)
		} else {
			var jpegBuf bytes.Buffer
			if encodeErr := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 88}); encodeErr != nil {
				finalReader = bytes.NewReader(fileBytes)
				finalFilename = fmt.Sprintf("%s%s", uuid.New().String(), rawExt)
			} else {
				finalReader = &jpegBuf
				finalFilename = fmt.Sprintf("%s.jpg", uuid.New().String())
			}
		}
	} else {
		// Standard .jpg, .jpeg, .png, .pdf, .heic
		if rawExt == ".pdf" {
			if !strings.HasPrefix(detectedMime, "application/pdf") && !bytes.HasPrefix(sniffHeader, []byte("%PDF")) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "corrupted or forged PDF file",
				})
			}
		} else if isHeic {
			// Validated HEIF container structure
			if !isHeicBytes(sniffHeader) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "invalid or corrupted HEIC/HEIF file",
				})
			}
		} else {
			// Image formats (.png, .jpg, .jpeg)
			if !strings.HasPrefix(detectedMime, "image/") {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "content does not match a valid image file",
				})
			}
		}

		finalReader = bytes.NewReader(fileBytes)
		finalFilename = fmt.Sprintf("%s%s", uuid.New().String(), rawExt)
	}

	fileURL, err := h.storage.Upload(finalReader, finalFilename)
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

// isHeicBytes checks if the byte slice starts with an ISO/IEC 23008-12 HEIF signature
func isHeicBytes(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	if string(b[4:8]) != "ftyp" {
		return false
	}
	majorBrand := string(b[8:12])
	heicBrands := map[string]bool{
		"heic": true, "heix": true, "hevc": true, "hevx": true,
		"mif1": true, "msf1": true,
	}
	return heicBrands[majorBrand]
}
