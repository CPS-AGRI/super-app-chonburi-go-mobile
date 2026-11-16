package config

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv             string
	AppPort            string
	DBDsn              string
	OrgName            string
	OrgLogoURL         string
	JWTSecret          string
	GoogleClientID     string
	GoogleClientSecret string
	FacebookAppID      string
	FacebookAppSecret  string
	LineChannelID      string
	LineChannelSecret  string
	AppleBundleID      string

	ThaiIDClientID     string
	ThaiIDClientSecret string
	ThaiIDApiKey       string
	ThaiIDRedirectURI  string

	// Security: Thai citizen ID (PID) protection — PDPA compliance
	PIDHmacSecret    string // HMAC-SHA256 key for PID hashing (lookup)
	PIDEncryptionKey string // AES-256-GCM key (must be exactly 32 bytes in hex)
	AllowDebugOTP    bool
	InternalOCRURL   string

	SMTPHost     string
	SMTPPort     string
	SMTPEmail    string
	SMTPPassword string

	TaxBillerID  string
	TaxUploadDir string
	MinIO        MinIOConfig

	SMSGatewayURL   string
	SMSAPIKey       string
	SMSAPISecret    string
	SMSSenderName   string
	SMSRelayEnabled bool
	SMSRelayURL     string
	SMSRelaySecret  string

	RedisHost     string
	RedisPort     string
	RedisAddr     string // host:port — ใช้ field นี้กับ RedisClient
	RedisPassword string
	RedisDB       int

	// Fahfon Microservice (10.1.2.25)
	FahfonWeatherBaseURL string
	FahfonWeatherMapURL  string
	FahfonServiceKey     string

	// Fahfon River Microservice (10.1.2.19)
	RiverServiceURL     string
	RiverGo2RTCURL      string
	RiverMunicipalityID string

	// CCTV Partner Integration (Unified Handshake System)
	CCTVPartnerAPIKey      string
	CCTVPartnerPortalURL   string
	CCTVAllowedCallbacks   []string
	CCTVTicketTTLSeconds   int
	CCTVVerifyRateLimit    int
	CCTVHandshakeRateLimit int
	ThaiIDQRGatewayURL     string

	// Ph6 Integration Specifics
	PH6ClientID     string
	PH6ClientSecret string
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
	if jwtSecret == "" || len(jwtSecret) < 32 {
		log.Fatal("❌ FATAL: JWT_SECRET environment variable is required and must be at least 32 bytes")
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
		AppEnv: func() string {
			env := strings.TrimSpace(os.Getenv("APP_ENV"))
			if env == "" {
				return "development"
			}
			return env
		}(),
		AppPort:            port,
		DBDsn:              dsn,
		OrgName:            orgName,
		OrgLogoURL:         os.Getenv("ORG_LOGO_URL"),
		JWTSecret:          jwtSecret,
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		FacebookAppID:      os.Getenv("FACEBOOK_APP_ID"),
		FacebookAppSecret:  os.Getenv("FACEBOOK_APP_SECRET"),
		LineChannelID:      os.Getenv("LINE_CHANNEL_ID"),
		LineChannelSecret:  os.Getenv("LINE_CHANNEL_SECRET"),
		AppleBundleID: func() string {
			b := strings.TrimSpace(os.Getenv("APPLE_BUNDLE_ID"))
			if b == "" {
				return "com.chonburipaoconnext.mobile"
			}
			return b
		}(),
		ThaiIDClientID: func() string {
			cid := os.Getenv("THAIID_CLIENT_ID")
			if cid == "" {
				cid = "RnRBS0JOemx2dUZBSlhmVnNLcXU0VDM5T0J0Y1FDSTY"
			}
			return cid
		}(),
		ThaiIDClientSecret: os.Getenv("THAIID_CLIENT_SECRET"),
		ThaiIDApiKey:       os.Getenv("THAIID_API_KEY"),
		ThaiIDRedirectURI: func() string {
			uri := os.Getenv("THAIID_REDIRECT_URI")
			if uri == "" {
				uri = "https://uat-super-app-chonburi.mueangsmart.com/thaiid-callback"
			}
			return uri
		}(),
		PIDHmacSecret: func() string {
			sec := strings.TrimSpace(os.Getenv("PID_HMAC_SECRET"))
			if sec == "" {
				log.Println("⚠️  Warning: PID_HMAC_SECRET is empty; deriving deterministic HMAC secret from JWT_SECRET")
				sum := sha256.Sum256([]byte("pid-hmac:" + jwtSecret))
				return hex.EncodeToString(sum[:])
			}
			return sec
		}(),
		PIDEncryptionKey: func() string {
			key := strings.TrimSpace(os.Getenv("PID_ENCRYPTION_KEY"))
			if decoded, err := hex.DecodeString(key); err != nil || len(decoded) != 32 {
				log.Println("⚠️  Warning: PID_ENCRYPTION_KEY is empty or not 64 hex chars; deriving deterministic 32-byte key from JWT_SECRET")
				sum := sha256.Sum256([]byte("pid-aes256gcm:" + jwtSecret))
				return hex.EncodeToString(sum[:])
			}
			return key
		}(),
		AllowDebugOTP:  getEnvBool("ALLOW_DEBUG_OTP", false),
		InternalOCRURL: strings.TrimSpace(os.Getenv("INTERNAL_OCR_URL")),
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
		SMSGatewayURL:   os.Getenv("SMS_GATEWAY_URL"),
		SMSAPIKey:       os.Getenv("SMS_API_KEY"),
		SMSAPISecret:    os.Getenv("SMS_API_SECRET"),
		SMSSenderName:   os.Getenv("SMS_SENDER_NAME"),
		SMSRelayEnabled: getEnvBool("SMS_RELAY_ENABLED", false),
		SMSRelayURL:     strings.TrimSpace(os.Getenv("SMS_RELAY_URL")),
		SMSRelaySecret:  strings.TrimSpace(os.Getenv("SMS_RELAY_SECRET")),
		RedisHost: os.Getenv("REDIS_HOST"),
		RedisPort: func() string {
			p := os.Getenv("REDIS_PORT")
			if p == "" {
				p = "6379"
			}
			return p
		}(),
		RedisAddr: func() string {
			host := os.Getenv("REDIS_HOST")
			port := os.Getenv("REDIS_PORT")
			if port == "" {
				port = "6379"
			}
			return host + ":" + port
		}(),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB: func() int {
			db, _ := strconv.Atoi(os.Getenv("REDIS_DB"))
			return db
		}(),
		FahfonWeatherBaseURL: func() string {
			u := os.Getenv("FAHFON_WEATHER_API_URL")
			if u == "" {
				u = "http://10.1.2.25:50059"
			}
			return u
		}(),
		FahfonWeatherMapURL: func() string {
			u := os.Getenv("FAHFON_WEATHER_MAP_URL")
			if u == "" {
				u = "http://10.1.2.25:3000"
			}
			return u
		}(),
		FahfonServiceKey: func() string {
			k := os.Getenv("FAHFON_SERVICE_KEY")
			if k == "" {
				k = "ff_sec_service_key_Prod_7821_wQ9s8Zx2tA"
			}
			return k
		}(),
		RiverServiceURL: func() string {
			u := os.Getenv("RIVER_SERVICE_URL")
			if u == "" {
				u = "http://10.1.2.19:8081"
			}
			return u
		}(),
		RiverGo2RTCURL: func() string {
			u := os.Getenv("RIVER_GO2RTC_URL")
			if u == "" {
				u = "http://10.1.2.7:1984"
			}
			return u
		}(),
		RiverMunicipalityID: func() string {
			m := os.Getenv("RIVER_MUNICIPALITY_ID")
			if m == "" {
				m = "c04da467-53f7-4076-93e9-54896eedb6c6"
			}
			return m
		}(),
		CCTVPartnerAPIKey:    os.Getenv("CCTV_PARTNER_API_KEY"),
		CCTVPartnerPortalURL: os.Getenv("CCTV_PARTNER_PORTAL_URL"),
		CCTVAllowedCallbacks: func() []string {
			cb := os.Getenv("CCTV_ALLOWED_CALLBACKS")
			if cb == "" {
				return []string{}
			}
			parts := strings.Split(cb, ",")
			var cleaned []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					cleaned = append(cleaned, p)
				}
			}
			return cleaned
		}(),
		CCTVTicketTTLSeconds:   getEnvInt("CCTV_TICKET_TTL_SECONDS", 60),
		CCTVVerifyRateLimit:    getEnvInt("CCTV_VERIFY_RATE_LIMIT", 60),
		CCTVHandshakeRateLimit: getEnvInt("CCTV_HANDSHAKE_RATE_LIMIT", 10),
		ThaiIDQRGatewayURL: func() string {
			u := os.Getenv("THAIID_QR_GATEWAY_URL")
			if u == "" {
				u = "https://imauth.bora.dopa.go.th/api/v2/oauth2/auth/"
			}
			return u
		}(),
		PH6ClientID:     os.Getenv("PH6_CLIENT_ID"),
		PH6ClientSecret: os.Getenv("PH6_CLIENT_SECRET"),
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
