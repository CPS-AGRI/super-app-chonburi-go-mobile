package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/delivery/http"
	"super-app-chonburi-go-mobile/internal/delivery/http/middleware"
	"super-app-chonburi-go-mobile/internal/infrastructure"
	"super-app-chonburi-go-mobile/internal/repository"
	"super-app-chonburi-go-mobile/internal/usecase"
	"super-app-chonburi-go-mobile/pkg/database"
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
		BodyLimit:    25 * 1024 * 1024,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
		ProxyHeader:  fiber.HeaderXForwardedFor,
		TrustProxy:   true,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Loopback: true,
			Private:  true,
		},
	})

	app.Use(recover.New())
	app.Use(helmet.New())
	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))
	app.Use(logger.New())
	app.Use(middleware.NewAPIRateLimiter(cfg, 600, 1200, time.Minute, "/"))
	app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowCredentials: false,
	}))

	smsClient := infrastructure.NewSMSClient(
		cfg.SMSGatewayURL,
		cfg.SMSAPIKey,
		cfg.SMSAPISecret,
		cfg.SMSSenderName,
		infrastructure.WithSMSRelay(cfg.SMSRelayEnabled, cfg.SMSRelayURL, cfg.SMSRelaySecret),
	)
	redisClient, _ := infrastructure.NewRedisClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	uploadStorage := storage.NewMinIOStorage(minioClient)

	authRepo := repository.NewAuthRepository(database.DB)
	authUseCase := usecase.NewAuthUseCase(authRepo, cfg, smsClient, redisClient)
	http.NewAuthHandler(app, authUseCase, cfg, uploadStorage)

	complaintRepo := repository.NewComplaintRepository(database.DB)
	complaintUseCase := usecase.NewComplaintUseCase(complaintRepo)
	http.NewComplaintHandler(app, complaintUseCase, cfg)

	moduleRepo := repository.NewModuleRepository(database.DB)
	moduleUseCase := usecase.NewModuleUseCase(moduleRepo)
	http.NewModuleHandler(app, moduleUseCase)

	http.NewUploadHandler(app, uploadStorage, cfg)

	muniBankRepo := repository.NewMunicipalityBankRepository(database.DB)
	muniBankUseCase := usecase.NewMunicipalityBankUseCase(muniBankRepo)
	http.NewMunicipalityBankHandler(app, muniBankUseCase)

	publicRelationRepo := repository.NewPublicRelationMobileRepository(database.DB)
	publicRelationUseCase := usecase.NewPublicRelationMobileUseCase(publicRelationRepo)
	http.NewPublicRelationMobileHandler(app, publicRelationUseCase, cfg)

	notificationRepo := repository.NewNotificationRepository(database.DB)
	notificationUseCase := usecase.NewNotificationUseCase(notificationRepo)
	http.NewNotificationHandler(app, notificationUseCase, cfg)

	// Weather Subscriptions (Area alert preferences)
	weatherSubRepo := repository.NewWeatherSubscriptionRepository(database.DB)
	weatherSubUseCase := usecase.NewWeatherSubscriptionUseCase(weatherSubRepo)
	http.RegisterWeatherSubscriptionRoutes(app, cfg, weatherSubUseCase)

	verificationRepo := repository.NewVerificationRepository(database.DB, cfg)
	verificationUseCase := usecase.NewVerificationUseCase(verificationRepo, weatherSubUseCase)
	http.NewVerificationHandler(app, verificationUseCase, cfg)

	analyticsRepo := repository.NewAnalyticsRepository(database.DB)
	analyticsUseCase := usecase.NewAnalyticsUseCase(analyticsRepo)
	http.NewAnalyticsHandler(app, analyticsUseCase, cfg)

	cctvRepo := repository.NewCCTVRepository(database.DB)
	cctvUseCase := usecase.NewCCTVUseCase(cctvRepo, authRepo, cfg, redisClient, analyticsUseCase)
	http.NewCCTVHandler(app, cctvUseCase, cfg, redisClient, authUseCase)

	weatherRepo := repository.NewWeatherRepository(database.DB, cfg, redisClient)
	weatherUseCase := usecase.NewWeatherUseCase(weatherRepo)
	http.NewWeatherHandler(app, weatherUseCase)

	waterLevelRepo := repository.NewWaterLevelRepository(database.DB, cfg, redisClient)
	waterLevelUseCase := usecase.NewWaterLevelUseCase(waterLevelRepo)
	http.NewWaterLevelHandler(app, waterLevelUseCase, cfg)

	http.NewDocumentHandler(app)

	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("Super App Chonburi Mobile API is running... 🚀")
	})

	port := ":" + cfg.AppPort
	log.Printf("🚀 Mobile API Server is starting on http://localhost%s", port)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("🛑 Received signal %v, initiating graceful shutdown...", sig)
		if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
			log.Printf("⚠️ Graceful shutdown error: %v", err)
		}
		if redisClient != nil && redisClient.Client != nil {
			_ = redisClient.Client.Close()
		}
		if database.DB != nil {
			if sqlDB, err := database.DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	}()

	if err := app.Listen(port); err != nil {
		log.Fatalf("Fatal: Could not start server: %v", err)
	}
}
