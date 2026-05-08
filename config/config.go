package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort            string
	DBDsn              string
	OrgName            string
	OrgLogoURL         string
	JWTSecret          string
	GoogleClientID     string
	GoogleClientSecret string
	FacebookAppID      string
	FacebookAppSecret  string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️  Warning: No .env file found. Falling back to system environment variables.")
	}

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("❌ FATAL: DB_DSN environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	orgName := os.Getenv("ORG_NAME")
	if orgName == "" {
		orgName = "องค์การบริหารส่วนจังหวัดชลบุรี"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "chonburi-plus-mobile-secret-key-2024"
	}

	return &Config{
		AppPort:            port,
		DBDsn:              dsn,
		OrgName:            orgName,
		OrgLogoURL:         os.Getenv("ORG_LOGO_URL"),
		JWTSecret:          jwtSecret,
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		FacebookAppID:      os.Getenv("FACEBOOK_APP_ID"),
		FacebookAppSecret:  os.Getenv("FACEBOOK_APP_SECRET"),
	}
}
