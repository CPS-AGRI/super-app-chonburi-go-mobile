package http

import (
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
	group.Post("/refresh", handler.RefreshToken)
	group.Post("/otp/request", handler.RequestOTP)
	group.Post("/otp/verify", handler.VerifyOTP)
	group.Post("/register", handler.Register)
	group.Post("/pin/login", handler.LoginWithPin)
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

	res, err := h.usecase.Register(req.Pin, req.TempToken)
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
