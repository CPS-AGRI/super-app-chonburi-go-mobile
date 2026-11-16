package database

import (
	"log"
	"os"
	"strings"
	"time"

	"super-app-chonburi-go-mobile/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func ConnectDB(cfg *config.Config) {
	var err error

	// Determine log mode based on environment to prevent leaking PII in production
	appEnv := strings.ToLower(os.Getenv("APP_ENV"))
	gormLogLevel := logger.Warn
	if appEnv == "development" || appEnv == "dev" || appEnv == "local" {
		gormLogLevel = logger.Info
	}

	DB, err = gorm.Open(postgres.Open(cfg.DBDsn), &gorm.Config{
		Logger: logger.Default.LogMode(gormLogLevel),
	})
	if err != nil {
		log.Fatalf("Fatal: Failed to connect to database: %v", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatalf("Fatal: Failed to extract underlying sql.DB: %v", err)
	}

	// High-Performance connection pool configuration
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(1 * time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // Evict dead idle connections before firewall/PG drops them

	DB.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"")

	log.Println("✅ Database connected successfully.")
}
