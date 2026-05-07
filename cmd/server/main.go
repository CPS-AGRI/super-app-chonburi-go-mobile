package main

import (
	"log"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/pkg/database"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

func main() {
	cfg := config.LoadConfig()

	database.ConnectDB(cfg)

	app := fiber.New(fiber.Config{
		AppName:      "Super App Chonburi Mobile API",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	})

	app.Use(recover.New())
	app.Use(helmet.New())
	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowCredentials: false,
	}))

	api := app.Group("/api/v1")
	_ = api // TODO: register routes here

	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("Super App Chonburi Mobile API is running... 🚀")
	})

	port := ":" + cfg.AppPort
	log.Printf("🚀 Mobile API Server is starting on http://localhost%s", port)

	if err := app.Listen(port); err != nil {
		log.Fatalf("Fatal: Could not start server: %v", err)
	}
}
