package jwtutil_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/pkg/jwtutil"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestRequireAuth_MissingToken_Unauthorized(t *testing.T) {
	cfg := &config.Config{
		JWTSecret: "super-secret-key-that-is-at-least-32-bytes-long",
	}

	app := fiber.New()
	app.Get("/protected", jwtutil.RequireAuth(cfg), func(c fiber.Ctx) error {
		return c.SendString("OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAuth_ValidHMAC_Success(t *testing.T) {
	secret := "super-secret-key-that-is-at-least-32-bytes-long"
	cfg := &config.Config{
		JWTSecret: secret,
	}

	claims := &jwtutil.MobileClaims{
		UserID: "usr_12345",
		Email:  "test@chonburi.go.th",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	assert.NoError(t, err)

	app := fiber.New()
	app.Get("/protected", jwtutil.RequireAuth(cfg), func(c fiber.Ctx) error {
		userID, err := jwtutil.ExtractUserID(c)
		assert.NoError(t, err)
		assert.Equal(t, "usr_12345", userID)
		return c.SendString("OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestRequireAuth_AlgorithmConfusion_Rejected(t *testing.T) {
	cfg := &config.Config{
		JWTSecret: "super-secret-key-that-is-at-least-32-bytes-long",
	}

	// Token with 'none' signing method (Unsigned / Algorithm Confusion)
	claims := &jwtutil.MobileClaims{
		UserID: "usr_attacker",
		Email:  "evil@attacker.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenStr, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	assert.NoError(t, err)

	app := fiber.New()
	app.Get("/protected", jwtutil.RequireAuth(cfg), func(c fiber.Ctx) error {
		return c.SendString("SHOULD_NOT_REACH")
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}
