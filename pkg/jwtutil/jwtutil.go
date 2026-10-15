package jwtutil

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"super-app-chonburi-go-mobile/config"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

type MobileClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

func RequireAuth(cfg *config.Config) fiber.Handler {
	return func(c fiber.Ctx) error {
		var tokenString string

		authHeader := c.Get("Authorization")
		if authHeader != "" && len(authHeader) > 7 && strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = authHeader[7:]
		}

		if tokenString == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: Missing or invalid token",
			})
		}

		token, err := jwt.ParseWithClaims(tokenString, &MobileClaims{}, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(cfg.JWTSecret), nil
		})

		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: " + err.Error(),
			})
		}

		claims, ok := token.Claims.(*MobileClaims)
		if !ok || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: Invalid token claims",
			})
		}

		if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: Token expired",
			})
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		return c.Next()
	}
}

func ExtractUserID(c fiber.Ctx) (string, error) {
	val := c.Locals("user_id")
	if val == nil {
		return "", errors.New("unauthorized: user id not found in context")
	}
	userID, ok := val.(string)
	if !ok || userID == "" {
		return "", errors.New("unauthorized: invalid user id type in context")
	}
	return userID, nil
}

func ParseMobileToken(tokenString string, secret string) (*MobileClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &MobileClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*MobileClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}
