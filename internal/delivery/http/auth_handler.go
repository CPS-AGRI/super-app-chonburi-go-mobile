package http

import (
	"super-app-chonburi-go-mobile/internal/domain"
	"github.com/gofiber/fiber/v3"
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
