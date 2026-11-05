package domain

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================
// User Weather Subscription
// Written by: super-app-chonburi-go-mobile (Mobile API)
// Read by:    super-app-chonburi-go (Alert Evaluation Cron)
// ============================================================

// UserWeatherSubscription represents a user's weather alert area of interest.
type UserWeatherSubscription struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null"                             json:"user_id"`
	District    string    `gorm:"type:text;not null"                             json:"district"`
	Subdistrict string    `gorm:"type:text;not null"                             json:"subdistrict"`
	IsPrimary   bool      `gorm:"type:boolean;not null;default:false"            json:"is_primary"`
	IsActive    bool      `gorm:"type:boolean;not null;default:true"             json:"is_active"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null;default:now()"        json:"created_at"`
	UpdatedAt   time.Time `gorm:"type:timestamptz;not null;default:now()"        json:"updated_at"`
}

func (UserWeatherSubscription) TableName() string {
	return "user_weather_subscriptions"
}

// WeatherAlertHistory represents an issued weather alert entry
type WeatherAlertHistory struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Subdistrict string    `gorm:"type:varchar(100);not null" json:"subdistrict"`
	District    string    `gorm:"type:varchar(100);not null" json:"district"`
	Title       string    `gorm:"type:varchar(255);not null" json:"title"`
	Message     string    `gorm:"type:text;not null" json:"message"`
	Severity    string    `gorm:"type:varchar(20);not null;default:'info'" json:"severity"`
	AlertType   string    `gorm:"type:varchar(50);not null;default:'general'" json:"alert_type"`
	ImageURL    string    `gorm:"type:text" json:"image_url"`
	SentCount   int       `gorm:"type:int;not null;default:0" json:"sent_count"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null;default:now()" json:"created_at"`
}

func (WeatherAlertHistory) TableName() string {
	return "weather_alert_history"
}

// WeatherAlertFeedResponse is the DTO formatted specifically for Mobile App notification cards
type WeatherAlertFeedResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`  // "rain" | "pm25" | "storm" | "general"
	Level     string `json:"level"` // "info" | "warning" | "danger"
	ImageURL  string `json:"image_url,omitempty"`
}

// WeatherSubscriptionRequest is the Mobile API request payload.
type WeatherSubscriptionRequest struct {
	District    string `json:"district"    validate:"required"`
	Subdistrict string `json:"subdistrict" validate:"required"`
}

// WeatherSubscriptionResponse is the Mobile API response for a single subscription.
type WeatherSubscriptionResponse struct {
	ID          uuid.UUID `json:"id"`
	District    string    `json:"district"`
	Subdistrict string    `json:"subdistrict"`
	IsPrimary   bool      `json:"is_primary"`
	IsActive    bool      `json:"is_active"`
}

// ChonburiLocationItem represents a selectable area for the mobile UI picker.
type ChonburiLocationItem struct {
	District     string   `json:"district"`
	Subdistricts []string `json:"subdistricts"`
}

// WeatherSubscriptionRepository defines persistence operations.
type WeatherSubscriptionRepository interface {
	GetByUserID(userID uuid.UUID) ([]UserWeatherSubscription, error)
	CountByUserID(userID uuid.UUID) (int64, error)
	UpsertPrimary(userID uuid.UUID, district, subdistrict string) error
	Create(sub *UserWeatherSubscription) error
	Delete(id uuid.UUID, userID uuid.UUID) error
	GetAlertHistory(subdistricts []string, limit int) ([]WeatherAlertHistory, error)
}

// WeatherSubscriptionUseCase defines business logic for subscription management.
type WeatherSubscriptionUseCase interface {
	GetSubscriptions(userID uuid.UUID) ([]WeatherSubscriptionResponse, error)
	AddSubscription(userID uuid.UUID, req *WeatherSubscriptionRequest) error
	RemoveSubscription(id uuid.UUID, userID uuid.UUID) error
	SyncPrimaryFromAddress(userID uuid.UUID, district, subdistrict string) error
	GetAlertHistory(userID uuid.UUID, limit int) ([]WeatherAlertFeedResponse, error)
}

// ============================================================
// Weather Account (existing)
// ============================================================

// WeatherAccount represents an account registered in Chonburi database
type WeatherAccount struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UniqueID       string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"unique_id"`
	AccountName    string    `gorm:"type:varchar(128);not null;default:''" json:"account_name"`
	Description    string    `gorm:"type:text" json:"description,omitempty"`
	IsActive       bool      `gorm:"type:boolean;not null;default:true" json:"is_active"`
	IsForecastNoti bool      `gorm:"type:boolean;not null;default:true" json:"is_forecast_noti"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (WeatherAccount) TableName() string {
	return "chonburi_weather_accounts"
}

// ============================================================
// Raw models returned from ff-weather-service (10.1.2.25:50059)
// ============================================================

type FahfonStationItem struct {
	IMEI         string             `json:"imei"`
	StationName  string             `json:"stationName"`
	Serial       string             `json:"serial"`
	IsOwner      bool               `json:"isOwner"`
	Parameters   FahfonParameters   `json:"parameters"`
	Position     FahfonPosition     `json:"position"`
	RegistedIMEI FahfonRegistedIMEI `json:"registedIMEI"`
}

type FahfonParameters struct {
	Temp                            float64 `json:"temp"`
	RelativeHumidity                float64 `json:"relativeHumidity"`
	BarometricPressure              float64 `json:"barometricPressure"`
	Rainfall                        float64 `json:"rainfall"`
	HourlyAccumRain                 float64 `json:"hourlyAccumRain"`
	DailyAccumRain                  float64 `json:"dailyAccumRain"`
	WindSpeed                       float64 `json:"windSpeed"`
	WindDirection                   float64 `json:"windDirection"`
	PM1dot0                         float64 `json:"pM1dot0"`
	PM2dot5                         float64 `json:"pM2dot5"`
	PM4                             float64 `json:"pM4"`
	PM10                            float64 `json:"pM10"`
	CO2                             float64 `json:"cO2"`
	UVIndex                         float64 `json:"uvIndex"`
	LightIntensity                  float64 `json:"lightIntensity"`
	RedLightIntensityGain1          float64 `json:"redLightIntensityGain1"`
	GreenLightIntensityGain1        float64 `json:"greenLightIntensityGain1"`
	BlueLightIntensityGain1         float64 `json:"blueLightIntensityGain1"`
	NearInfraredLightIntensityGain1 float64 `json:"nearInfraredLightIntensityGain1"`
}

type FahfonPosition struct {
	Height       string `json:"height"`
	Latitude     string `json:"latitude"`
	Longitude    string `json:"longitude"`
	TimeStampUTC int64  `json:"timeStampUTC"`
}

type FahfonRegistedIMEI struct {
	AccountID      string `json:"accountId"`
	StationName    string `json:"stationName"`
	Serial         string `json:"serial"`
	CommonSiteName string `json:"commonSiteName"`
	Email          string `json:"email"`
	Latitude       string `json:"latitude"`
	Longitude      string `json:"longitude"`
	Owner          string `json:"owner"`
	RegisType      string `json:"regisType"`
}

type FahfonStationsResponse struct {
	Items      []FahfonStationItem `json:"items"`
	TotalItems int                 `json:"total_items,omitempty"`
}

// ============================================================
// Clean Mobile Response Model
// ============================================================

type WeatherStationDTO struct {
	ID              string  `json:"id"`
	IMEI            string  `json:"imei"`
	Serial          string  `json:"serial"`
	DeviceName      string  `json:"deviceName"`
	StationName     string  `json:"stationName"`
	Name            string  `json:"name"`
	LocationName    string  `json:"locationName"`
	AccountID       string  `json:"accountId"`
	Latitude        float64 `json:"latitude"`
	Longitude       float64 `json:"longitude"`
	Temp            float64 `json:"temp"`
	Humidity        float64 `json:"humidity"`
	Pressure        float64 `json:"pressure"`
	Rainfall        float64 `json:"rainfall"`
	HourlyAccumRain float64 `json:"hourlyAccumRain"`
	DailyAccumRain  float64 `json:"dailyAccumRain"`
	WindSpeed       float64 `json:"windSpeed"`
	WindDirection   float64 `json:"windDirection"`
	PM25            float64 `json:"pm25"`
	PM10            float64 `json:"pm10"`
	PM1             float64 `json:"pm1"`
	PM4             float64 `json:"pm4"`
	CO2             float64 `json:"co2"`
	UVIndex         float64 `json:"uvIndex"`
	Status          string  `json:"status"` // "normal", "warning", "danger"
	TimestampUTC    int64   `json:"timestampUtc"`
	LastUpdated     string  `json:"lastUpdated"`
}

type WeatherStationsResult struct {
	Stations   []WeatherStationDTO `json:"stations"`
	Total      int                 `json:"total"`
	CachedAt   time.Time           `json:"cachedAt"`
	IsFallback bool                `json:"isFallback,omitempty"`
}
