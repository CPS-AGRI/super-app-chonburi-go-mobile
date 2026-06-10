package main

import (
	"log"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/delivery/http"
	"super-app-chonburi-go-mobile/internal/repository"
	"super-app-chonburi-go-mobile/internal/usecase"
	"super-app-chonburi-go-mobile/pkg/database"
	"super-app-chonburi-go-mobile/pkg/mail"
	"super-app-chonburi-go-mobile/pkg/storage"
	minioStorage "super-app-chonburi-go-mobile/pkg/storage/minio"

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

	minioClient, err := minioStorage.NewClient(cfg.MinIO)
	if err != nil {
		log.Fatalf("Fatal: Failed to initialize MinIO client: %v", err)
	}

	app := fiber.New(fiber.Config{
		AppName:      "Super App Chonburi Mobile API",
		BodyLimit:    100 * 1024 * 1024,
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

	authRepo := repository.NewAuthRepository(database.DB)
	authUseCase := usecase.NewAuthUseCase(authRepo, cfg)
	http.NewAuthHandler(app, authUseCase)

	complaintRepo := repository.NewComplaintRepository(database.DB)
	complaintUseCase := usecase.NewComplaintUseCase(complaintRepo)
	http.NewComplaintHandler(app, complaintUseCase)

	moduleRepo := repository.NewModuleRepository(database.DB)
	moduleUseCase := usecase.NewModuleUseCase(moduleRepo)
	http.NewModuleHandler(app, moduleUseCase)

	mailSender := mail.NewSMTPEmailSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPEmail, cfg.SMTPPassword)
	uploadStorage := storage.NewMinIOStorage(minioClient)
	taxNewMobileRepo := repository.NewTaxNewMobileRepository(database.DB)
	taxNewMobileUseCase := usecase.NewTaxNewMobileUseCase(taxNewMobileRepo, mailSender, cfg.TaxBillerID)
	http.NewTaxNewMobileHandler(app, taxNewMobileUseCase, uploadStorage)

	muniBankRepo := repository.NewMunicipalityBankRepository(database.DB)
	muniBankUseCase := usecase.NewMunicipalityBankUseCase(muniBankRepo)
	http.NewMunicipalityBankHandler(app, muniBankUseCase)

	publicRelationRepo := repository.NewPublicRelationMobileRepository(database.DB)
	publicRelationUseCase := usecase.NewPublicRelationMobileUseCase(publicRelationRepo)
	http.NewPublicRelationMobileHandler(app, publicRelationUseCase)

	notificationRepo := repository.NewNotificationRepository(database.DB)
	notificationUseCase := usecase.NewNotificationUseCase(notificationRepo)
	http.NewNotificationHandler(app, notificationUseCase, cfg)

	verificationRepo := repository.NewVerificationRepository(database.DB)
	verificationUseCase := usecase.NewVerificationUseCase(verificationRepo)
	http.NewVerificationHandler(app, verificationUseCase, cfg)

	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("Super App Chonburi Mobile API is running... 🚀")
	})

	port := ":" + cfg.AppPort
	log.Printf("🚀 Mobile API Server is starting on http://localhost%s", port)

	if err := app.Listen(port); err != nil {
		log.Fatalf("Fatal: Could not start server: %v", err)
	}
}
