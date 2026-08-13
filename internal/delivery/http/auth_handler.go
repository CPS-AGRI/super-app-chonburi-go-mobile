package http

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"
	"super-app-chonburi-go-mobile/internal/domain"
)

type AuthHandler struct {
	usecase domain.AuthUseCase
}

func NewAuthHandler(app *fiber.App, usecase domain.AuthUseCase) {
	handler := &AuthHandler{usecase: usecase}

	group := app.Group("/api/v1/auth")
	group.Post("/google", handler.LoginWithGoogle)
	group.Post("/facebook", handler.LoginWithFacebook)
	group.Post("/line", handler.LoginWithLine)
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
		AccessToken string `json:"access_token"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
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
