package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/jwtutil"
	"super-app-chonburi-go-mobile/pkg/safe"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ocrRateEntry struct {
	count     int
	expiresAt time.Time
}

type verificationHandler struct {
	useCase domain.VerificationUseCase
	cfg     *config.Config
	ocrRate sync.Map
}

func NewVerificationHandler(app *fiber.App, useCase domain.VerificationUseCase, cfg *config.Config) {
	handler := &verificationHandler{
		useCase: useCase,
		cfg:     cfg,
	}

	safe.Go(func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			handler.ocrRate.Range(func(k, v interface{}) bool {
				if entry, ok := v.(ocrRateEntry); !ok || now.After(entry.expiresAt) {
					handler.ocrRate.Delete(k)
				}
				return true
			})
		}
	})

	ocrMiddleware := func(c fiber.Ctx) error {
		ip := c.IP()
		if ip == "" {
			ip = "unknown"
		}
		now := time.Now()
		val, loaded := handler.ocrRate.Load(ip)
		var entry ocrRateEntry
		if loaded {
			entry, _ = val.(ocrRateEntry)
		}
		if !loaded || now.After(entry.expiresAt) {
			entry = ocrRateEntry{
				count:     1,
				expiresAt: now.Add(time.Minute),
			}
			handler.ocrRate.Store(ip, entry)
			return c.Next()
		}
		if entry.count >= 20 {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "คำขอสแกนบัตรประชาชนเกินขีดจำกัด กรุณารอสักครู่แล้วลองใหม่อีกครั้ง",
			})
		}
		entry.count++
		handler.ocrRate.Store(ip, entry)
		return c.Next()
	}

	authMiddleware := jwtutil.RequireAuth(cfg)

	for _, prefix := range []string{"/api/v1/verification", "/api/v1/mobile/verification"} {
		// Unauthenticated OCR endpoint for new user registration (rate-limited by ocrMiddleware)
		app.Post(prefix+"/ocr", ocrMiddleware, handler.ScanIDCardOCR)

		vGroup := app.Group(prefix, authMiddleware)
		vGroup.Post("/submit", handler.Submit)
		vGroup.Put("/address", handler.UpdateAddress)
		vGroup.Get("/status", handler.GetStatus)
		vGroup.Post("/fcm-token", handler.RegisterFCMToken)
		vGroup.Delete("/fcm-token", handler.UnregisterFCMToken)
	}

	app.Get("/api/v1/auth/me", authMiddleware, handler.Me)
	app.Get("/api/v1/mobile/auth/me", authMiddleware, handler.Me)
}

func (h *verificationHandler) Me(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id format"})
	}

	res, err := h.useCase.GetMe(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(strings.ToLower(err.Error()), "record not found") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "user not found or session expired"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "data": res})
}

func (h *verificationHandler) Submit(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id format"})
	}

	var req domain.SubmitVerificationRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.IdentityNumber == "" || req.IdCardPhotoUrl == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "identity_number and id_card_photo_url are required"})
	}

	if err := h.useCase.SubmitVerification(userID, &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "verification request submitted successfully"})
}

func (h *verificationHandler) UpdateAddress(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id format"})
	}

	var req domain.UpdateAddressRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := h.useCase.UpdateAddress(userID, &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "address updated successfully"})
}

func (h *verificationHandler) GetStatus(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id format"})
	}

	res, err := h.useCase.GetVerificationStatus(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "data": res})
}

func (h *verificationHandler) RegisterFCMToken(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id format"})
	}

	var req domain.RegisterFCMTokenRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.DeviceID == "" || req.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "device_id and token are required"})
	}

	if err := h.useCase.RegisterFCMToken(userID, &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "FCM token registered successfully"})
}

func (h *verificationHandler) UnregisterFCMToken(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id format"})
	}

	var req domain.UnregisterFCMTokenRequest
	_ = c.Bind().JSON(&req)
	if req.DeviceID == "" {
		req.DeviceID = c.Query("device_id")
	}

	if req.DeviceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "device_id is required"})
	}

	if err := h.useCase.UnregisterFCMToken(userID, req.DeviceID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "FCM token unregistered successfully"})
}

type thaiIDCardOCRData struct {
	IdentityNumber string `json:"identityNumber"`
	Prefix         string `json:"prefix"`
	FirstName      string `json:"firstName"`
	LastName       string `json:"lastName"`
	BirthDay       string `json:"birthDay"`
	BirthMonth     string `json:"birthMonth"`
	BirthYear      string `json:"birthYear"`
	HouseNo        string `json:"houseNo"`
	VillageNo      string `json:"villageNo"`
	Soi            string `json:"soi"`
	Road           string `json:"road"`
	SubDistrict    string `json:"subDistrict"`
	District       string `json:"district"`
	Province       string `json:"province"`
}

func (h *verificationHandler) ScanIDCardOCR(c fiber.Ctx) error {
	const maxOCRSize = 10 * 1024 * 1024 // 10MB

	var imgBytes []byte
	file, err := c.FormFile("file")
	if err == nil && file != nil {
		if file.Size > maxOCRSize {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file size exceeds 10MB limit"})
		}
		src, openErr := file.Open()
		if openErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to open uploaded file"})
		}
		defer src.Close()
		data, readErr := io.ReadAll(io.LimitReader(src, maxOCRSize+1))
		if readErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to read uploaded file"})
		}
		if len(data) > maxOCRSize {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file size exceeds 10MB limit"})
		}
		imgBytes = data
	} else {
		var req struct {
			Base64Image string `json:"base64_image"`
			ImageURL    string `json:"image_url"`
		}
		if err := c.Bind().Body(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file, base64_image, or image_url is required"})
		}

		if strings.TrimSpace(req.Base64Image) != "" {
			b64 := strings.TrimSpace(req.Base64Image)
			if idx := strings.Index(b64, ","); idx != -1 && strings.Contains(b64[:idx], "base64") {
				b64 = b64[idx+1:]
			}
			decoded, decErr := base64.StdEncoding.DecodeString(b64)
			if decErr != nil {
				decoded, decErr = base64.RawStdEncoding.DecodeString(b64)
			}
			if decErr != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid base64_image"})
			}
			if len(decoded) > maxOCRSize {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "image size exceeds 10MB limit"})
			}
			imgBytes = decoded
		} else if strings.TrimSpace(req.ImageURL) != "" {
			imgURL := strings.TrimSpace(req.ImageURL)
			if strings.HasPrefix(imgURL, "/uploads/") {
				cleanRel := filepath.Clean(strings.TrimPrefix(imgURL, "/"))
				if !strings.HasPrefix(cleanRel, "uploads/") {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid local image path"})
				}
				data, readErr := os.ReadFile(cleanRel)
				if readErr != nil {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to read uploaded image"})
				}
				if len(data) > maxOCRSize {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "image size exceeds 10MB limit"})
				}
				imgBytes = data
			} else if strings.HasPrefix(imgURL, "https://") {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
				if reqErr != nil {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid image_url"})
				}
				resp, doErr := http.DefaultClient.Do(httpReq)
				if doErr != nil || resp.StatusCode != http.StatusOK {
					if resp != nil {
						resp.Body.Close()
					}
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to fetch image_url"})
				}
				defer resp.Body.Close()
				data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxOCRSize+1))
				if readErr != nil || len(data) > maxOCRSize {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid or oversized image from image_url"})
				}
				imgBytes = data
			} else {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "image_url must start with /uploads/ or https://"})
			}
		}
	}

	if len(imgBytes) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "image file or base64_image is required"})
	}

	headerLen := len(imgBytes)
	if headerLen > 512 {
		headerLen = 512
	}
	if !isAllowedImageBytes(imgBytes[:headerLen]) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid image format: only JPEG, PNG, WebP, HEIC are supported"})
	}

	var rawText string

	// 1. Check if INTERNAL_OCR_URL is configured
	internalOCRURL := ""
	if h.cfg != nil && h.cfg.InternalOCRURL != "" {
		internalOCRURL = h.cfg.InternalOCRURL
	} else {
		internalOCRURL = os.Getenv("INTERNAL_OCR_URL")
	}

	if internalOCRURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		payload, _ := json.Marshal(map[string]string{
			"base64_image": base64.StdEncoding.EncodeToString(imgBytes),
		})
		ocrReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, internalOCRURL, bytes.NewReader(payload))
		if reqErr == nil {
			ocrReq.Header.Set("Content-Type", "application/json")
			if resp, doErr := http.DefaultClient.Do(ocrReq); doErr == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var ocrResp struct {
						RawText string             `json:"raw_text"`
						Text    string             `json:"text"`
						Data    *thaiIDCardOCRData `json:"data"`
					}
					if json.NewDecoder(resp.Body).Decode(&ocrResp) == nil {
						if ocrResp.RawText != "" {
							rawText = ocrResp.RawText
						} else {
							rawText = ocrResp.Text
						}
						if ocrResp.Data != nil && ocrResp.Data.IdentityNumber != "" {
							return c.JSON(fiber.Map{
								"success":  true,
								"raw_text": rawText,
								"data":     ocrResp.Data,
							})
						}
					}
				}
			}
		}
	}

	// 2. Otherwise run local on-premise Tesseract OCR if available
	if rawText == "" {
		if tesseractPath, lookErr := exec.LookPath("tesseract"); lookErr == nil && tesseractPath != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, tesseractPath, "stdin", "stdout", "-l", "tha+eng", "--psm", "6")
			cmd.Stdin = bytes.NewReader(imgBytes)
			var outBuf bytes.Buffer
			cmd.Stdout = &outBuf
			if runErr := cmd.Run(); runErr == nil {
				rawText = outBuf.String()
			}
		}
	}

	parsedData := parseThaiIDCardOCRText(rawText)

	return c.JSON(fiber.Map{
		"success":  true,
		"raw_text": rawText,
		"data":     parsedData,
	})
}

var (
	thaiDigitReplacer = strings.NewReplacer(
		"๐", "0", "๑", "1", "๒", "2", "๓", "3", "๔", "4",
		"๕", "5", "๖", "6", "7", "7", "๗", "7", "๘", "8", "๙", "9",
	)
	reIDSpaced    = regexp.MustCompile(`(\d)\s*[-\s]?\s*(\d{4})\s*[-\s]?\s*(\d{5})\s*[-\s]?\s*(\d{2})\s*[-\s]?\s*(\d)`)
	reID13Contig  = regexp.MustCompile(`\b(\d{13})\b`)
	reThaiName    = regexp.MustCompile(`(?:ชื่อตัวและชื่อสกุล|ชื่อตัว|ชื่อ)\s*(นาย|นางสาว|น\.ส\.|นาง|ด\.ช\.|ด\.ญ\.|เด็กชาย|เด็กหญิง)?\s*([ก-๙]+)\s+([ก-๙]+)`)
	reThaiPrefix  = regexp.MustCompile(`(นาย|นางสาว|น\.ส\.|นาง|ด\.ช\.|ด\.ญ\.|เด็กชาย|เด็กหญิง)\s*([ก-๙]{2,})\s+([ก-๙]{2,})`)
	reThaiDate    = regexp.MustCompile(`(\d{1,2})\s*(ม\.ค\.|ก\.พ\.|มี\.ค\.|เม\.ย\.|พ\.ค\.|มิ\.ย\.|ก\.ค\.|ส\.ค\.|ก\.ย\.|ต\.ค\.|พ\.ย\.|ธ\.ค\.|มกราคม|กุมภาพันธ์|มีนาคม|เมษายน|พฤษภาคม|มิถุนายน|กรกฎาคม|สิงหาคม|กันยายน|ตุลาคม|พฤศจิกายน|ธันวาคม)\s*(\d{4})`)
	reEngDate     = regexp.MustCompile(`(?i)(\d{1,2})\s+(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z.]*\s+(\d{4})`)
	reHouseNo     = regexp.MustCompile(`(?:ที่อยู่|บ้านเลขที่)?\s*(\d+(?:/\d+)?)\s*(?:หมู่ที่|หมู่|ม\.)`)
	reVillageNo   = regexp.MustCompile(`(?:หมู่ที่|หมู่|ม\.)\s*(\d+)`)
	reSoi         = regexp.MustCompile(`(?:ซอย|ซ\.)\s*([^\sตอจ]+)`)
	reRoad        = regexp.MustCompile(`(?:ถนน|ถ\.)\s*([^\sตอจ]+)`)
	reSubDistrict = regexp.MustCompile(`(?:ตำบล|ต\.|แขวง)\s*([ก-๙]+)`)
	reDistrict    = regexp.MustCompile(`(?:อำเภอ|อ\.|เขต)\s*([ก-๙]+)`)
	reProvince    = regexp.MustCompile(`(?:จังหวัด|จ\.)\s*([ก-๙]+)`)
)

func validateThaiIDChecksum(id string) bool {
	if len(id) != 13 {
		return false
	}
	sum := 0
	for i := 0; i < 12; i++ {
		d := int(id[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		sum += d * (13 - i)
	}
	check := (11 - (sum % 11)) % 10
	return check == int(id[12]-'0')
}

func parseThaiIDCardOCRText(raw string) thaiIDCardOCRData {
	res := thaiIDCardOCRData{}
	if strings.TrimSpace(raw) == "" {
		return res
	}

	normalized := thaiDigitReplacer.Replace(raw)

	// 1. Extract 13-digit Thai ID
	for _, m := range reIDSpaced.FindAllStringSubmatch(normalized, -1) {
		if len(m) == 6 {
			candidate := m[1] + m[2] + m[3] + m[4] + m[5]
			if validateThaiIDChecksum(candidate) {
				res.IdentityNumber = candidate
				break
			}
			if res.IdentityNumber == "" && len(candidate) == 13 {
				res.IdentityNumber = candidate
			}
		}
	}
	if res.IdentityNumber == "" {
		for _, m := range reID13Contig.FindAllStringSubmatch(normalized, -1) {
			if len(m) >= 2 {
				if validateThaiIDChecksum(m[1]) {
					res.IdentityNumber = m[1]
					break
				}
				if res.IdentityNumber == "" {
					res.IdentityNumber = m[1]
				}
			}
		}
	}

	// 2. Extract Prefix, FirstName, LastName
	if m := reThaiName.FindStringSubmatch(normalized); len(m) == 4 {
		res.Prefix = strings.TrimSpace(m[1])
		res.FirstName = strings.TrimSpace(m[2])
		res.LastName = strings.TrimSpace(m[3])
	} else if m := reThaiPrefix.FindStringSubmatch(normalized); len(m) == 4 {
		res.Prefix = strings.TrimSpace(m[1])
		res.FirstName = strings.TrimSpace(m[2])
		res.LastName = strings.TrimSpace(m[3])
	}

	// 3. Extract Birth Date
	thaiMonths := map[string]string{
		"ม.ค.": "01", "มกราคม": "01",
		"ก.พ.": "02", "กุมภาพันธ์": "02",
		"มี.ค.": "03", "มีนาคม": "03",
		"เม.ย.": "04", "เมษายน": "04",
		"พ.ค.": "05", "พฤษภาคม": "05",
		"มิ.ย.": "06", "มิถุนายน": "06",
		"ก.ค.": "07", "กรกฎาคม": "07",
		"ส.ค.": "08", "สิงหาคม": "08",
		"ก.ย.": "09", "กันยายน": "09",
		"ต.ค.": "10", "ตุลาคม": "10",
		"พ.ย.": "11", "พฤศจิกายน": "11",
		"ธ.ค.": "12", "ธันวาคม": "12",
	}
	if m := reThaiDate.FindStringSubmatch(normalized); len(m) == 4 {
		if day, err := strconv.Atoi(m[1]); err == nil {
			res.BirthDay = fmt.Sprintf("%02d", day)
		}
		if mm, ok := thaiMonths[m[2]]; ok {
			res.BirthMonth = mm
		}
		if yr, err := strconv.Atoi(m[3]); err == nil {
			if yr > 2400 {
				yr -= 543
			}
			res.BirthYear = strconv.Itoa(yr)
		}
	} else if m := reEngDate.FindStringSubmatch(normalized); len(m) == 4 {
		engMonths := map[string]string{
			"jan": "01", "feb": "02", "mar": "03", "apr": "04",
			"may": "05", "jun": "06", "jul": "07", "aug": "08",
			"sep": "09", "oct": "10", "nov": "11", "dec": "12",
		}
		if day, err := strconv.Atoi(m[1]); err == nil {
			res.BirthDay = fmt.Sprintf("%02d", day)
		}
		if mm, ok := engMonths[strings.ToLower(m[2][:3])]; ok {
			res.BirthMonth = mm
		}
		if yr, err := strconv.Atoi(m[3]); err == nil {
			if yr > 2400 {
				yr -= 543
			}
			res.BirthYear = strconv.Itoa(yr)
		}
	}

	// 4. Address components
	if m := reHouseNo.FindStringSubmatch(normalized); len(m) >= 2 {
		res.HouseNo = strings.TrimSpace(m[1])
	}
	if m := reVillageNo.FindStringSubmatch(normalized); len(m) >= 2 {
		res.VillageNo = strings.TrimSpace(m[1])
	}
	if m := reSoi.FindStringSubmatch(normalized); len(m) >= 2 {
		res.Soi = strings.TrimSpace(m[1])
	}
	if m := reRoad.FindStringSubmatch(normalized); len(m) >= 2 {
		res.Road = strings.TrimSpace(m[1])
	}
	if m := reSubDistrict.FindStringSubmatch(normalized); len(m) >= 2 {
		res.SubDistrict = strings.TrimSpace(m[1])
	}
	if m := reDistrict.FindStringSubmatch(normalized); len(m) >= 2 {
		res.District = strings.TrimSpace(m[1])
	}
	if m := reProvince.FindStringSubmatch(normalized); len(m) >= 2 {
		res.Province = strings.TrimSpace(m[1])
	}

	return res
}

