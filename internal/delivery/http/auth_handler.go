package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/jwtutil"
	"super-app-chonburi-go-mobile/pkg/storage"
)

type AuthHandler struct {
	usecase domain.AuthUseCase
	cfg     *config.Config
	storage storage.StorageProvider
}

func NewAuthHandler(app *fiber.App, usecase domain.AuthUseCase, cfg *config.Config, strg storage.StorageProvider) {
	handler := &AuthHandler{usecase: usecase, cfg: cfg, storage: strg}

	registerGroup := func(prefix string) {
		group := app.Group(prefix)
		group.Post("/google", handler.LoginWithGoogle)
		group.Post("/google/bind-override", handler.BindOverrideGoogle)
		group.Post("/facebook", handler.LoginWithFacebook)
		group.Post("/facebook/bind-override", handler.BindOverrideFacebook)
		group.Post("/line", handler.LoginWithLine)
		group.Post("/line/bind-override", handler.BindOverrideLine)
		group.Post("/apple", handler.LoginWithApple)
		group.Post("/apple/bind-override", handler.BindOverrideApple)
		group.Post("/thaiid", handler.LoginWithThaiID)
		group.Get("/thaiid/callback", handler.ThaiIDCallback)
		group.Post("/refresh", handler.RefreshToken)
		group.Post("/otp/request", handler.RequestOTP)
		group.Post("/otp/verify", handler.VerifyOTP)
		group.Post("/register", handler.Register)
		group.Post("/pin/login", handler.LoginWithPin)
		group.Post("/pin/reset", handler.ResetPin)
		group.Post("/bind-phone", handler.BindPhone)
		group.Post("/check-phone", handler.CheckPhone)

		// Protected routes (require JWT)
		protected := group.Group("", jwtutil.RequireAuth(cfg))
		protected.Post("/pin/change", handler.ChangePin)
		protected.Post("/thaiid/bind", handler.BindThaiID)
		protected.Post("/thaiid/bind/confirm", handler.ConfirmThaiIDMigration)
		protected.Get("/social-links", handler.GetSocialLinks)
		protected.Post("/social-links/unlink", handler.UnlinkSocial)
		protected.Post("/social-links/link", handler.LinkSocial)
		protected.Put("/profile", handler.UpdateProfile)
		protected.Put("/profile/image", handler.UpdateProfileImage)
		protected.Delete("/account", handler.DeleteAccount)
	}

	registerGroup("/api/v1/auth")
	registerGroup("/api/v1/mobile/auth")
}

func (h *AuthHandler) LoginWithGoogle(c fiber.Ctx) error {
	var req struct {
		IDToken     string `json:"id_token"`
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	tokenOrCode := strings.TrimSpace(req.IDToken)
	if tokenOrCode == "" {
		tokenOrCode = strings.TrimSpace(req.Code)
	}

	if tokenOrCode == "" {
		return c.Status(400).JSON(fiber.Map{"error": "id_token or code is required"})
	}

	res, err := h.usecase.LoginWithGoogle(tokenOrCode, req.RedirectURI)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) LoginWithFacebook(c fiber.Ctx) error {
	var req struct {
		AccessToken string `json:"access_token"` // Standard Login
		AuthToken   string `json:"auth_token"`   // Limited Login (iOS JWT)
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Prefer Limited Login JWT token (from iOS native SDK)
	if req.AuthToken != "" {
		res, err := h.usecase.LoginWithFacebookLimited(req.AuthToken)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(res)
	}

	// Fallback to Standard Graph API access token
	if req.AccessToken == "" {
		return c.Status(400).JSON(fiber.Map{"error": "access_token or auth_token is required"})
	}
	res, err := h.usecase.LoginWithFacebook(req.AccessToken)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

func (h *AuthHandler) LoginWithLine(c fiber.Ctx) error {
	var req struct {
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Code == "" || req.RedirectURI == "" {
		return c.Status(400).JSON(fiber.Map{"error": "code and redirect_uri are required"})
	}

	res, err := h.usecase.LoginWithLine(req.Code, req.RedirectURI)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) LoginWithApple(c fiber.Ctx) error {
	var req struct {
		IDToken  string `json:"id_token"`
		FullName string `json:"full_name"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if strings.TrimSpace(req.IDToken) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "id_token is required"})
	}

	res, err := h.usecase.LoginWithApple(strings.TrimSpace(req.IDToken), req.FullName)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) LoginWithThaiID(c fiber.Ctx) error {
	var req struct {
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Code == "" || req.RedirectURI == "" {
		return c.Status(400).JSON(fiber.Map{"error": "code and redirect_uri are required"})
	}

	res, err := h.usecase.LoginWithThaiID(req.Code, req.RedirectURI)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) ThaiIDCallback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		return c.Status(400).SendString("Authorization code is missing")
	}

	redirectURL := fmt.Sprintf("chonburipaoconnext://thaiid?code=%s&state=%s", url.QueryEscape(code), url.QueryEscape(state))
	return c.Redirect().Status(fiber.StatusFound).To(redirectURL)
}

func (h *AuthHandler) BindThaiID(c fiber.Ctx) error {
	userID, err := jwtutil.ExtractUserID(c)
	if err != nil || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized: valid JWT required"})
	}

	var req struct {
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Code == "" || req.RedirectURI == "" {
		return c.Status(400).JSON(fiber.Map{"error": "code and redirect_uri are required"})
	}

	res, err := h.usecase.BindThaiID(userID, req.Code, req.RedirectURI)
	if err != nil {
		if conflictErr, ok := err.(*domain.ThaiIDConflictError); ok {
			return c.Status(409).JSON(fiber.Map{
				"conflict":              true,
				"error":                 conflictErr.ErrorMsg,
				"migration_token":       conflictErr.MigrationToken,
				"existing_masked_phone": conflictErr.ExistingMaskedPhone,
			})
		}
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) ConfirmThaiIDMigration(c fiber.Ctx) error {
	userID, err := jwtutil.ExtractUserID(c)
	if err != nil || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized: valid JWT required"})
	}

	var req struct {
		MigrationToken string `json:"migration_token"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.MigrationToken == "" {
		return c.Status(400).JSON(fiber.Map{"error": "migration_token is required"})
	}

	res, err := h.usecase.ConfirmThaiIDMigration(userID, req.MigrationToken)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) RefreshToken(c fiber.Ctx) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	res, err := h.usecase.RefreshToken(req.RefreshToken)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) RequestOTP(c fiber.Ctx) error {
	var req domain.OTPRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.PhoneNumber == "" {
		return c.Status(400).JSON(fiber.Map{"error": "phone_number is required"})
	}

	res, err := h.usecase.RequestOTP(req.PhoneNumber)
	if err != nil {
		if strings.Contains(err.Error(), "cooldown") {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       err.Error(),
				"retry_after": 60,
			})
		}
		if strings.Contains(err.Error(), "invalid Thai mobile") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) VerifyOTP(c fiber.Ctx) error {
	var req domain.OTPVerifyRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.PhoneNumber == "" || req.OTP == "" || req.Ref == "" {
		return c.Status(400).JSON(fiber.Map{"error": "phone_number, otp, and ref are required"})
	}

	res, err := h.usecase.VerifyOTP(req.PhoneNumber, req.OTP, req.Ref)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) Register(c fiber.Ctx) error {
	var req domain.RegisterRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Pin == "" || req.TempToken == "" {
		return c.Status(400).JSON(fiber.Map{"error": "pin and temp_token are required"})
	}

	res, err := h.usecase.Register(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) LoginWithPin(c fiber.Ctx) error {
	var req domain.PinLoginRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.PhoneNumber == "" || req.Pin == "" {
		return c.Status(400).JSON(fiber.Map{"error": "phone_number and pin are required"})
	}

	res, err := h.usecase.LoginWithPin(req.PhoneNumber, req.Pin)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) ChangePin(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}

	var req struct {
		OldPin     string `json:"old_pin"`
		CurrentPin string `json:"current_pin"`
		NewPin     string `json:"new_pin"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	oldPin := strings.TrimSpace(req.OldPin)
	if oldPin == "" {
		oldPin = strings.TrimSpace(req.CurrentPin)
	}
	newPin := strings.TrimSpace(req.NewPin)

	if oldPin == "" || newPin == "" {
		return c.Status(400).JSON(fiber.Map{"error": "old_pin and new_pin are required"})
	}

	if err := h.usecase.ChangePin(userID, oldPin, newPin); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "เปลี่ยนรหัส PIN สำเร็จ",
	})
}

func (h *AuthHandler) ResetPin(c fiber.Ctx) error {
	var req struct {
		TempToken   string `json:"temp_token"`
		PhoneNumber string `json:"phone_number"`
		Phone       string `json:"phone"`
		NewPin      string `json:"new_pin"`
		Pin         string `json:"pin"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	phone := strings.TrimSpace(req.PhoneNumber)
	if phone == "" {
		phone = strings.TrimSpace(req.Phone)
	}
	newPin := strings.TrimSpace(req.NewPin)
	if newPin == "" {
		newPin = strings.TrimSpace(req.Pin)
	}

	if newPin == "" || (strings.TrimSpace(req.TempToken) == "" && phone == "") {
		return c.Status(400).JSON(fiber.Map{"error": "new_pin and temp_token (or phone_number) are required"})
	}

	res, err := h.usecase.ResetPin(strings.TrimSpace(req.TempToken), phone, newPin)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) BindPhone(c fiber.Ctx) error {
	var req struct {
		IDToken     string `json:"id_token"`
		PhoneNumber string `json:"phone_number"`
		OTP         string `json:"otp"`
		Ref         string `json:"ref"`
		PIN         string `json:"pin"`
		Provider    string `json:"provider"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.IDToken == "" || req.PhoneNumber == "" {
		return c.Status(400).JSON(fiber.Map{"error": "id_token and phone_number are required"})
	}

	provider := req.Provider
	if provider == "" {
		provider = "google"
	}

	res, err := h.usecase.BindPhone(provider, req.IDToken, req.PhoneNumber, req.OTP, req.Ref, req.PIN)
	if err != nil {
		var conflictErr *domain.SocialConflictError
		if errors.As(err, &conflictErr) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"code":     "SOCIAL_ALREADY_LINKED",
				"error":    fmt.Sprintf("เบอร์โทรนี้เคยผูกกับบัญชี %s อื่นแล้ว (%s)", conflictErr.Provider, conflictErr.ExistingAccountName),
				"conflict": conflictErr,
			})
		}
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *AuthHandler) BindOverrideGoogle(c fiber.Ctx) error {
	var req domain.BindOverrideRequest
	if err := c.Bind().Body(&req); err != nil || req.OAuthProfileID == "" || req.Phone == "" || req.Pin == "" {
		return c.Status(400).JSON(fiber.Map{"error": "oauth_profile_id, phone, and pin are required"})
	}
	res, err := h.usecase.BindOverrideGoogle(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

func (h *AuthHandler) BindOverrideFacebook(c fiber.Ctx) error {
	var req domain.BindOverrideRequest
	if err := c.Bind().Body(&req); err != nil || req.OAuthProfileID == "" || req.Phone == "" || req.Pin == "" {
		return c.Status(400).JSON(fiber.Map{"error": "oauth_profile_id, phone, and pin are required"})
	}
	res, err := h.usecase.BindOverrideFacebook(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

func (h *AuthHandler) BindOverrideLine(c fiber.Ctx) error {
	var req domain.BindOverrideRequest
	if err := c.Bind().Body(&req); err != nil || req.OAuthProfileID == "" || req.Phone == "" || req.Pin == "" {
		return c.Status(400).JSON(fiber.Map{"error": "oauth_profile_id, phone, and pin are required"})
	}
	res, err := h.usecase.BindOverrideLine(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

func (h *AuthHandler) BindOverrideApple(c fiber.Ctx) error {
	var req domain.BindOverrideRequest
	if err := c.Bind().Body(&req); err != nil || req.OAuthProfileID == "" || req.Phone == "" || req.Pin == "" {
		return c.Status(400).JSON(fiber.Map{"error": "oauth_profile_id, phone, and pin are required"})
	}
	res, err := h.usecase.BindOverrideApple(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

func (h *AuthHandler) CheckPhone(c fiber.Ctx) error {
	var req struct {
		PhoneNumber string `json:"phone_number"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.PhoneNumber == "" {
		return c.Status(400).JSON(fiber.Map{"error": "phone_number is required"})
	}

	isRegistered, err := h.usecase.CheckPhone(req.PhoneNumber)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"is_registered": isRegistered})
}

func (h *AuthHandler) GetSocialLinks(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}
	resp, err := h.usecase.GetSocialLinks(userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data":    resp,
	})
}

func (h *AuthHandler) UnlinkSocial(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}
	var req domain.UnlinkSocialRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := h.usecase.UnlinkSocial(userID, req.Provider); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "message": "unlinked successfully"})
}

func (h *AuthHandler) LinkSocial(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}
	var req domain.LinkSocialRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := h.usecase.LinkSocial(userID, req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "message": "linked successfully"})
}

func isAllowedImageBytes(header []byte) bool {
	if len(header) < 4 {
		return false
	}
	mime := http.DetectContentType(header)
	switch mime {
	case "image/jpeg", "image/png", "image/webp":
		return true
	}
	// Check HEIC/HEIF ISO BMFF ftyp box
	if len(header) >= 12 && string(header[4:8]) == "ftyp" {
		brand := strings.ToLower(string(header[8:12]))
		switch brand {
		case "heic", "heix", "hevc", "hevx", "mif1", "msf1":
			return true
		}
	}
	return false
}

func (h *AuthHandler) UpdateProfileImage(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}

	var imageURL string
	file, err := c.FormFile("file")
	if err == nil && file != nil && h.storage != nil {
		const maxImageSize = 10 * 1024 * 1024 // 10MB
		if file.Size > maxImageSize {
			return c.Status(400).JSON(fiber.Map{"error": "file size exceeds 10MB limit"})
		}

		ext := strings.ToLower(filepath.Ext(file.Filename))
		allowedExts := map[string]bool{
			".jpg":  true,
			".jpeg": true,
			".png":  true,
			".webp": true,
			".heic": true,
			".heif": true,
		}
		if !allowedExts[ext] {
			return c.Status(400).JSON(fiber.Map{"error": "invalid file extension: only jpg, jpeg, png, webp, heic, heif are allowed"})
		}

		src, err := file.Open()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to open uploaded file"})
		}
		defer src.Close()

		headerBuf := make([]byte, 512)
		n, readErr := src.Read(headerBuf)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return c.Status(400).JSON(fiber.Map{"error": "failed to read file header"})
		}
		if !isAllowedImageBytes(headerBuf[:n]) {
			return c.Status(400).JSON(fiber.Map{"error": "invalid image file content"})
		}

		combinedReader := io.MultiReader(bytes.NewReader(headerBuf[:n]), src)
		filename := fmt.Sprintf("profile_%s_%d%s", userID.String(), time.Now().Unix(), ext)
		uploadedURL, err := h.storage.Upload(combinedReader, filename)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to upload image: " + err.Error()})
		}
		imageURL = uploadedURL
	} else {
		var req struct {
			ImageURL string `json:"image_url"`
		}
		if err := c.Bind().JSON(&req); err == nil && req.ImageURL != "" {
			trimmed := strings.TrimSpace(req.ImageURL)
			if !strings.HasPrefix(trimmed, "https://") && !strings.HasPrefix(trimmed, "/uploads/") {
				return c.Status(400).JSON(fiber.Map{"error": "image_url must start with https:// or /uploads/"})
			}
			imageURL = trimmed
		}
	}

	if imageURL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "image file or image_url is required"})
	}

	if err := h.usecase.UpdateProfileImage(userID, imageURL); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "image_profile_url": imageURL})
}

func (h *AuthHandler) UpdateProfile(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}

	var raw struct {
		Prefix        *string         `json:"prefix"`
		Name          *string         `json:"name"`
		FirstName     *string         `json:"first_name"`
		LastName      *string         `json:"last_name"`
		Email         *string         `json:"email"`
		Birthday      *string         `json:"birthday"`
		HouseNumber   *string         `json:"house_number"`
		HouseNo       *string         `json:"house_no"`
		VillageNumber *string         `json:"village_number"`
		VillageNo     *string         `json:"village_no"`
		Alley         *string         `json:"alley"`
		Soi           *string         `json:"soi"`
		Road          *string         `json:"road"`
		Intersection  *string         `json:"intersection"`
		BuildingName  *string         `json:"building_name"`
		Building      *string         `json:"building"`
		RoomNumber    *string         `json:"room_number"`
		RoomNo        *string         `json:"room_no"`
		Subdistrict   *string         `json:"subdistrict"`
		SubDistrict   *string         `json:"sub_district"`
		District      *string         `json:"district"`
		Province      *string         `json:"province"`
		PostalCode    json.RawMessage `json:"postal_code"`
		CardAddress   *string         `json:"card_address"`
	}
	if err := c.Bind().Body(&raw); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	coalescePtr := func(primary, fallback *string) *string {
		if primary != nil {
			return primary
		}
		return fallback
	}

	var postalCodePtr *int
	if len(raw.PostalCode) > 0 && string(raw.PostalCode) != "null" {
		var intVal int
		if err := json.Unmarshal(raw.PostalCode, &intVal); err == nil {
			postalCodePtr = &intVal
		} else {
			var strVal string
			if err := json.Unmarshal(raw.PostalCode, &strVal); err == nil {
				strVal = strings.TrimSpace(strVal)
				if strVal == "" {
					zero := 0
					postalCodePtr = &zero
				} else if parsed, parseErr := strconv.Atoi(strVal); parseErr == nil {
					postalCodePtr = &parsed
				}
			}
		}
	}

	req := domain.UpdateProfileRequest{
		Prefix:        raw.Prefix,
		Name:          coalescePtr(raw.Name, raw.FirstName),
		LastName:      raw.LastName,
		Email:         raw.Email,
		Birthday:      raw.Birthday,
		HouseNumber:   coalescePtr(raw.HouseNumber, raw.HouseNo),
		VillageNumber: coalescePtr(raw.VillageNumber, raw.VillageNo),
		Alley:         coalescePtr(raw.Alley, raw.Soi),
		Road:          raw.Road,
		Intersection:  raw.Intersection,
		BuildingName:  coalescePtr(raw.BuildingName, raw.Building),
		RoomNumber:    coalescePtr(raw.RoomNumber, raw.RoomNo),
		Subdistrict:   coalescePtr(raw.Subdistrict, raw.SubDistrict),
		District:      raw.District,
		Province:      raw.Province,
		PostalCode:    postalCodePtr,
		CardAddress:   raw.CardAddress,
	}

	updatedUser, err := h.usecase.UpdateProfile(userID, req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"user":    updatedUser,
		"data":    updatedUser,
	})
}

func (h *AuthHandler) DeleteAccount(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}

	if err := h.usecase.DeleteAccount(userID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "ลบบัญชีผู้ใช้เรียบร้อยแล้ว",
	})
}

