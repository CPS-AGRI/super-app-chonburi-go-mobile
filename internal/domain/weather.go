package domain

import (
	"time"

	"github.com/google/uuid"
)

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

// Raw models returned from ff-weather-service (10.1.2.25:50059)

type FahfonStationItem struct {
	IMEI         string             `json:"imei"`
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
	Height       string  `json:"height"`
	Latitude     string  `json:"latitude"`
	Longitude    string  `json:"longitude"`
	TimeStampUTC int64   `json:"timeStampUTC"`
}

type FahfonRegistedIMEI struct {
	AccountID      string `json:"accountId"`
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

// Clean Mobile Response Model

type WeatherStationDTO struct {
	ID              string  `json:"id"`
	IMEI            string  `json:"imei"`
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
