package middleware

import (
	"strings"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/pkg/jwtutil"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
)

func resolveRateLimitKey(c fiber.Ctx, cfg *config.Config) string {
	if userID, err := jwtutil.ExtractUserID(c); err == nil && userID != "" {
		return "user:" + userID
	}

	if cfg != nil && cfg.JWTSecret != "" {
		authHeader := c.Get("Authorization")
		if len(authHeader) > 7 && strings.HasPrefix(authHeader, "Bearer ") {
			if claims, err := jwtutil.ParseMobileToken(authHeader[7:], cfg.JWTSecret); err == nil && claims != nil && claims.UserID != "" {
				return "user:" + claims.UserID
			}
		}
	}

	return "ip:" + c.IP()
}

// NewAPIRateLimiter returns a sliding-window rate limiter with separate generous quotas
// for authenticated users and anonymous/CGNAT IPs so legitimate traffic is never throttled.
func NewAPIRateLimiter(cfg *config.Config, maxAuthenticated, maxAnonymous int, window time.Duration, skipPaths ...string) fiber.Handler {
	skipSet := make(map[string]bool, len(skipPaths))
	for _, p := range skipPaths {
		skipSet[p] = true
	}

	return limiter.New(limiter.Config{
		LimiterMiddleware: limiter.SlidingWindow{},
		Expiration:        window,
		KeyGenerator: func(c fiber.Ctx) string {
			return resolveRateLimitKey(c, cfg)
		},
		MaxFunc: func(c fiber.Ctx) int {
			if strings.HasPrefix(resolveRateLimitKey(c, cfg), "user:") {
				return maxAuthenticated
			}
			return maxAnonymous
		},
		Next: func(c fiber.Ctx) bool {
			return skipSet[c.Path()]
		},
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success": false,
				"error":   "too many requests, please try again later",
			})
		},
	})
}

