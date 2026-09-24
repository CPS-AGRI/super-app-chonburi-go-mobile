package http

import (
	"errors"
	"fmt"
	"path/filepath"
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

	group := app.Group("/api/v1/auth")
	group.Post("/google", handler.LoginWithGoogle)
	group.Post("/google/bind-override", handler.BindOverrideGoogle)
	group.Post("/facebook", handler.LoginWithFacebook)
	group.Post("/facebook/bind-override", handler.BindOverrideFacebook)
	group.Post("/line", handler.LoginWithLine)
	group.Post("/line/bind-override", handler.BindOverrideLine)
	group.Post("/apple/bind-override", handler.BindOverrideApple)
	group.Post("/thaiid", handler.LoginWithThaiID)
	group.Get("/thaiid/callback", handler.ThaiIDCallback)
	group.Post("/thaiid/bind", handler.BindThaiID)
	group.Post("/refresh", handler.RefreshToken)
	group.Post("/otp/request", handler.RequestOTP)
	group.Post("/otp/verify", handler.VerifyOTP)
	group.Post("/register", handler.Register)
	group.Post("/pin/login", handler.LoginWithPin)
	group.Post("/bind-phone", handler.BindPhone)
	group.Post("/check-phone", handler.CheckPhone)

	// Protected routes (require JWT)
	protected := group.Group("", jwtutil.RequireAuth(cfg))
	protected.Get("/social-links", handler.GetSocialLinks)
	protected.Post("/social-links/unlink", handler.UnlinkSocial)
	protected.Post("/social-links/link", handler.LinkSocial)
	protected.Put("/profile/image", handler.UpdateProfileImage)
}

func (h *AuthHandler) LoginWithGoogle(c fiber.Ctx) error {
	var req struct {
		IDToken string `json:"id_token"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.IDToken == "" {
		return c.Status(400).JSON(fiber.Map{"error": "id_token is required"})
	}

	res, err := h.usecase.LoginWithGoogle(req.IDToken)
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

	redirectURL := fmt.Sprintf("chonburiplus://thaiid?code=%s&state=%s", code, state)
	return c.Redirect().Status(fiber.StatusFound).To(redirectURL)
}

func (h *AuthHandler) BindThaiID(c fiber.Ctx) error {
	var req struct {
		UserID      string `json:"user_id"`
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	// ถ้าไม่ได้ส่ง user_id มา ให้ลองดึงจาก Authorization header (Bearer JWT)
	if req.UserID == "" {
		auth := c.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			req.UserID = c.Get("X-User-ID") // middleware ควร inject ไว้ ถ้ายังไม่มีให้ client ส่งมา
		}
	}

	if req.UserID == "" || req.Code == "" || req.RedirectURI == "" {
		return c.Status(400).JSON(fiber.Map{"error": "user_id, code, and redirect_uri are required"})
	}

	res, err := h.usecase.BindThaiID(req.UserID, req.Code, req.RedirectURI)
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
	return c.JSON(resp)
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
		src, err := file.Open()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to open uploaded file"})
		}
		defer src.Close()

		ext := filepath.Ext(file.Filename)
		filename := fmt.Sprintf("profile_%s_%d%s", userID.String(), time.Now().Unix(), ext)
		uploadedURL, err := h.storage.Upload(src, filename)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to upload image: " + err.Error()})
		}
		imageURL = uploadedURL
	} else {
		var req struct {
			ImageURL string `json:"image_url"`
		}
		if err := c.Bind().JSON(&req); err == nil && req.ImageURL != "" {
			imageURL = req.ImageURL
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

