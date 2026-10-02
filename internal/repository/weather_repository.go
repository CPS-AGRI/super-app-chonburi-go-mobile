package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/infrastructure"

	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

const (
	redisKeyWeatherStationsAll = "chonburi:weather:stations:all"
	redisTTLWeatherStationsAll = 180 * time.Second
	l1TTLWeatherStationsAll    = 30 * time.Second
)

type WeatherRepository interface {
	GetActiveAccounts(ctx context.Context) ([]domain.WeatherAccount, error)
	FetchAllStations(ctx context.Context) (*domain.WeatherStationsResult, error)
	GetStationByIMEI(ctx context.Context, imei string) (map[string]interface{}, error)
	GetStationForecast(ctx context.Context, imei string) (map[string]interface{}, error)
	GetStationGraph(ctx context.Context, imei, variable string, timeframe int, date string) (map[string]interface{}, error)
	GetMapFrames(ctx context.Context, variable, animType, date string) (map[string]interface{}, error)
	FlushStationsCache(ctx context.Context) error
}

type weatherRepository struct {
	db          *gorm.DB
	cfg         *config.Config
	redisClient *infrastructure.RedisClient
	httpClient  *http.Client
	sf          singleflight.Group

	// L1 In-Memory Cache
	l1Mu       sync.RWMutex
	l1Cache    *domain.WeatherStationsResult
	l1CachedAt time.Time
}

func NewWeatherRepository(db *gorm.DB, cfg *config.Config, redisClient *infrastructure.RedisClient) WeatherRepository {
	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression: false,
	}

	return &weatherRepository{
		db:          db,
		cfg:         cfg,
		redisClient: redisClient,
		httpClient: &http.Client{
			Timeout:   8 * time.Second,
			Transport: tr,
		},
	}
}

func (r *weatherRepository) GetActiveAccounts(ctx context.Context) ([]domain.WeatherAccount, error) {
	var accounts []domain.WeatherAccount
	err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("created_at ASC").
		Find(&accounts).Error
	return accounts, err
}

func (r *weatherRepository) FlushStationsCache(ctx context.Context) error {
	r.l1Mu.Lock()
	r.l1Cache = nil
	r.l1CachedAt = time.Time{}
	r.l1Mu.Unlock()

	if r.redisClient != nil && r.redisClient.Client != nil {
		_ = r.redisClient.Client.Del(ctx, redisKeyWeatherStationsAll).Err()
	}
	return nil
}

func (r *weatherRepository) FetchAllStations(ctx context.Context) (*domain.WeatherStationsResult, error) {
	// 1. Check L1 Memory Cache (< 1ms)
	r.l1Mu.RLock()
	if r.l1Cache != nil && time.Since(r.l1CachedAt) < l1TTLWeatherStationsAll {
		cached := r.l1Cache
		r.l1Mu.RUnlock()
		return cached, nil
	}
	r.l1Mu.RUnlock()

	// 2. Check L2 Redis Cache (< 3ms)
	if r.redisClient != nil && r.redisClient.Client != nil {
		cachedJSON, err := r.redisClient.Client.Get(ctx, redisKeyWeatherStationsAll).Result()
		if err == nil && cachedJSON != "" {
			var result domain.WeatherStationsResult
			if err := json.Unmarshal([]byte(cachedJSON), &result); err == nil && len(result.Stations) > 0 {
				r.l1Mu.Lock()
				r.l1Cache = &result
				r.l1CachedAt = time.Now()
				r.l1Mu.Unlock()
				return &result, nil
			}
		}
	}

	// 3. Singleflight group: prevent Thundering Herd / Cache Stampede
	val, err, _ := r.sf.Do("fetch_chonburi_stations", func() (interface{}, error) {
		accounts, err := r.GetActiveAccounts(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load active weather accounts: %w", err)
		}
		if len(accounts) == 0 {
			return &domain.WeatherStationsResult{
				Stations: []domain.WeatherStationDTO{},
				Total:    0,
				CachedAt: time.Now(),
			}, nil
		}

		type accountResult struct {
			uid   string
			name  string
			items []domain.FahfonStationItem
			err   error
		}

		resultsChan := make(chan accountResult, len(accounts))
		var wg sync.WaitGroup

		// Concurrent Goroutines with bounded parallelism
		semaphore := make(chan struct{}, 8)

		for _, acc := range accounts {
			wg.Add(1)
			go func(account domain.WeatherAccount) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				items, fetchErr := r.fetchStationsForAccount(ctx, account.UniqueID)
				resultsChan <- accountResult{
					uid:   account.UniqueID,
					name:  account.AccountName,
					items: items,
					err:   fetchErr,
				}
			}(acc)
		}

		wg.Wait()
		close(resultsChan)

		// Aggregate and deduplicate by IMEI
		stationMap := make(map[string]domain.WeatherStationDTO)
		for res := range resultsChan {
			if res.err != nil {
				log.Printf("⚠️ [Weather Repo] Failed to fetch stations for UID %s (%s): %v", res.uid, res.name, res.err)
				continue
			}

			for _, it := range res.items {
				if it.IMEI == "" {
					continue
				}

				lat, _ := strconv.ParseFloat(it.Position.Latitude, 64)
				lng, _ := strconv.ParseFloat(it.Position.Longitude, 64)

				siteName := it.RegistedIMEI.CommonSiteName
				if siteName == "" {
					siteName = fmt.Sprintf("สถานี : %s", it.IMEI)
				}

				locName := siteName
				if res.name != "" {
					locName = fmt.Sprintf("%s (%s)", siteName, res.name)
				}

				// Status calculation based on PM2.5 AQI standard (Thailand)
				status := "normal"
				if it.Parameters.PM2dot5 > 75.0 {
					status = "danger"
				} else if it.Parameters.PM2dot5 > 37.5 {
					status = "warning"
				}

				updatedTime := time.Unix(it.Position.TimeStampUTC, 0).Format("15:04 น.")

				dto := domain.WeatherStationDTO{
					ID:              it.IMEI,
					IMEI:            it.IMEI,
					Name:            siteName,
					LocationName:    locName,
					AccountID:       it.RegistedIMEI.AccountID,
					Latitude:        lat,
					Longitude:       lng,
					Temp:            it.Parameters.Temp,
					Humidity:        it.Parameters.RelativeHumidity,
					Pressure:        it.Parameters.BarometricPressure,
					Rainfall:        it.Parameters.Rainfall,
					HourlyAccumRain: it.Parameters.HourlyAccumRain,
					DailyAccumRain:  it.Parameters.DailyAccumRain,
					WindSpeed:       it.Parameters.WindSpeed,
					WindDirection:   it.Parameters.WindDirection,
					PM25:            it.Parameters.PM2dot5,
					PM10:            it.Parameters.PM10,
					PM1:             it.Parameters.PM1dot0,
					PM4:             it.Parameters.PM4,
					CO2:             it.Parameters.CO2,
					UVIndex:         it.Parameters.UVIndex,
					Status:          status,
					TimestampUTC:    it.Position.TimeStampUTC,
					LastUpdated:     updatedTime,
				}

				stationMap[it.IMEI] = dto
			}
		}

		finalStations := make([]domain.WeatherStationDTO, 0, len(stationMap))
		for _, s := range stationMap {
			finalStations = append(finalStations, s)
		}

		res := &domain.WeatherStationsResult{
			Stations: finalStations,
			Total:    len(finalStations),
			CachedAt: time.Now(),
		}

		// Save to L2 Redis Cache
		if r.redisClient != nil && r.redisClient.Client != nil {
			if data, err := json.Marshal(res); err == nil {
				_ = r.redisClient.Client.Set(ctx, redisKeyWeatherStationsAll, string(data), redisTTLWeatherStationsAll).Err()
			}
		}

		// Save to L1 Memory Cache
		r.l1Mu.Lock()
		r.l1Cache = res
		r.l1CachedAt = time.Now()
		r.l1Mu.Unlock()

		return res, nil
	})

	if err != nil {
		return nil, err
	}
	return val.(*domain.WeatherStationsResult), nil
}

func (r *weatherRepository) fetchStationsForAccount(ctx context.Context, accountID string) ([]domain.FahfonStationItem, error) {
	reqURL := fmt.Sprintf("%s/api/v1/weather/stations?account_id=%s", r.cfg.FahfonWeatherBaseURL, url.QueryEscape(accountID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("x-service-key", r.cfg.FahfonServiceKey)
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upstream error %d: %s", resp.StatusCode, string(body))
	}

	var raw domain.FahfonStationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	return raw.Items, nil
}

func (r *weatherRepository) GetStationByIMEI(ctx context.Context, imei string) (map[string]interface{}, error) {
	reqURL := fmt.Sprintf("%s/api/v1/weather/stations/imei/%s", r.cfg.FahfonWeatherBaseURL, url.PathEscape(imei))
	return r.executeGetJSON(ctx, reqURL)
}

func (r *weatherRepository) GetStationForecast(ctx context.Context, imei string) (map[string]interface{}, error) {
	reqURL := fmt.Sprintf("%s/api/v1/weather/forecast/stations/imei/%s", r.cfg.FahfonWeatherBaseURL, url.PathEscape(imei))
	return r.executeGetJSON(ctx, reqURL)
}

func (r *weatherRepository) GetStationGraph(ctx context.Context, imei, variable string, timeframe int, date string) (map[string]interface{}, error) {
	reqURL := fmt.Sprintf("%s/api/v1/weather/stations/imei/%s/graph?variable=%s&timeframe=%d&date=%s",
		r.cfg.FahfonWeatherBaseURL,
		url.PathEscape(imei),
		url.QueryEscape(variable),
		timeframe,
		url.QueryEscape(date),
	)
	return r.executeGetJSON(ctx, reqURL)
}

func (r *weatherRepository) GetMapFrames(ctx context.Context, variable, animType, date string) (map[string]interface{}, error) {
	baseURL := r.cfg.FahfonWeatherMapURL
	if baseURL == "" {
		baseURL = r.cfg.FahfonWeatherBaseURL
	}
	reqURL := fmt.Sprintf("%s/api/v1/weather/map/frames?variable=%s&anim_type=%s&date=%s",
		baseURL,
		url.QueryEscape(variable),
		url.QueryEscape(animType),
		url.QueryEscape(date),
	)
	return r.executeGetJSON(ctx, reqURL)
}

func (r *weatherRepository) executeGetJSON(ctx context.Context, reqURL string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("x-service-key", r.cfg.FahfonServiceKey)
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upstream error %d: %s", resp.StatusCode, string(body))
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return data, nil
}
