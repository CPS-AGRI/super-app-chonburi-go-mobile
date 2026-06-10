package config

import (
	"log"
	"os"
	"strconv"

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

	SMTPHost     string
	SMTPPort     string
	SMTPEmail    string
	SMTPPassword string

	TaxBillerID  string
	TaxUploadDir string
	MinIO        MinIOConfig
}

type MinIOConfig struct {
	Endpoint        string
	AccessKey       string
	SecretKey       string
	Bucket          string
	Region          string
	Secure          bool
	PublicRead      bool
	PublicBaseURL   string
	PresignURLTTL   int
	MaxUploadSizeMB int64
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

	taxUploadDir := os.Getenv("TAX_UPLOAD_DIR")
	if taxUploadDir == "" {
		taxUploadDir = "./uploads"
	}

	minioEndpoint := os.Getenv("MINIO_ENDPOINT")
	if minioEndpoint == "" {
		minioEndpoint = "122.155.169.235:9000"
	}

	minioAccessKey := os.Getenv("MINIO_ACCESS_KEY")
	minioSecretKey := os.Getenv("MINIO_SECRET_KEY")
	minioBucket := os.Getenv("MINIO_BUCKET")
	if minioBucket == "" {
		minioBucket = "super-app-assets"
	}

	minioRegion := os.Getenv("MINIO_REGION")
	if minioRegion == "" {
		minioRegion = "ap-southeast-1"
	}

	minioSecure := os.Getenv("MINIO_SECURE")
	useSecure := minioSecure == "true" || minioSecure == "1"

	minioPublicRead := getEnvBool("MINIO_PUBLIC_READ", false)
	minioPublicBaseURL := os.Getenv("MINIO_PUBLIC_BASE_URL")
	if minioPublicBaseURL == "" {
		scheme := "http"
		if useSecure {
			scheme = "https"
		}
		minioPublicBaseURL = scheme + "://" + minioEndpoint
	}

	minioPresignURLTTL := getEnvInt("MINIO_PRESIGN_URL_TTL_SECONDS", 3600)
	minioMaxUploadSizeMB := int64(getEnvInt("MINIO_MAX_UPLOAD_SIZE_MB", 10))

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
		SMTPHost:           os.Getenv("SMTP_HOST"),
		SMTPPort:           os.Getenv("SMTP_PORT"),
		SMTPEmail:          os.Getenv("SMTP_EMAIL"),
		SMTPPassword:       os.Getenv("SMTP_PASSWORD"),
		TaxBillerID:        os.Getenv("TAX_BILLER_ID"),
		TaxUploadDir:       taxUploadDir,
		MinIO: MinIOConfig{
			Endpoint:        minioEndpoint,
			AccessKey:       minioAccessKey,
			SecretKey:       minioSecretKey,
			Bucket:          minioBucket,
			Region:          minioRegion,
			Secure:          useSecure,
			PublicRead:      minioPublicRead,
			PublicBaseURL:   minioPublicBaseURL,
			PresignURLTTL:   minioPresignURLTTL,
			MaxUploadSizeMB: minioMaxUploadSizeMB,
		},
	}
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value == "true" || value == "1"
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
