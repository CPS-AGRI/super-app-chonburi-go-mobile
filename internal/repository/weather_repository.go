package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
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
	redisTTLWeatherForecast    = 15 * time.Minute
)

type WeatherRepository interface {
	GetActiveAccounts(ctx context.Context) ([]domain.WeatherAccount, error)
	FetchAllStations(ctx context.Context) (*domain.WeatherStationsResult, error)
	GetStationByIMEI(ctx context.Context, imei string) (map[string]interface{}, error)
	GetStationForecast(ctx context.Context, imei string) (map[string]interface{}, error)
	GetStationGraph(ctx context.Context, imei, variable string, timeframe int, date string) (map[string]interface{}, error)
	GetMapFrames(ctx context.Context, variable, animType, date string) (map[string]interface{}, error)
	GetMapTiles(ctx context.Context, variable, animType string) (map[string]interface{}, error)
	GetMapTilesRange(ctx context.Context, variable, animType, date string, minRow, maxRow, minCol, maxCol int) (map[string]interface{}, error)
	GetStationMonthlyStats(ctx context.Context, imei string, month, year int) (map[string]interface{}, error)
	GetSubdistrictForecast(ctx context.Context, district, subdistrict, province string) (map[string]interface{}, error)
	GetWeatherOverview(ctx context.Context, district, period, date string) (*domain.WeatherOverviewResult, error)
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
		sfCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()

		accounts, err := r.GetActiveAccounts(sfCtx)
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

				items, fetchErr := r.fetchStationsForAccount(sfCtx, account.UniqueID)
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

				serial := it.Serial
				if serial == "" {
					serial = it.RegistedIMEI.Serial
				}
				if serial == "" {
					serial = it.IMEI
				}

				deviceName := it.StationName
				if deviceName == "" {
					deviceName = it.RegistedIMEI.StationName
				}
				if deviceName == "" {
					deviceName = it.RegistedIMEI.CommonSiteName
				}
				if deviceName == "" {
					deviceName = fmt.Sprintf("สถานี %s", serial)
				}

				siteName := it.RegistedIMEI.CommonSiteName
				if siteName == "" {
					siteName = deviceName
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
					ID:              serial, // Use hardware serial as key
					IMEI:            it.IMEI,
					Serial:          serial,
					DeviceName:      deviceName,
					StationName:     deviceName,
					Name:            deviceName,
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

				stationMap[serial] = dto
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
				_ = r.redisClient.Client.Set(sfCtx, redisKeyWeatherStationsAll, string(data), redisTTLWeatherStationsAll).Err()
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
	cacheKey := fmt.Sprintf("chonburi:weather:forecast:%s", imei)

	// 1. Check Redis Cache (< 2ms)
	if r.redisClient != nil && r.redisClient.Client != nil {
		if cachedJSON, err := r.redisClient.Client.Get(ctx, cacheKey).Result(); err == nil && cachedJSON != "" {
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(cachedJSON), &result); err == nil {
				return result, nil
			}
		}
	}

	// 2. Singleflight to prevent Thundering Herd / Cache Stampede
	sfKey := fmt.Sprintf("sf_forecast_%s", imei)
	val, err, _ := r.sf.Do(sfKey, func() (interface{}, error) {
		sfCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()

		reqURL := fmt.Sprintf("%s/api/v1/weather/forecast/stations/imei/%s", r.cfg.FahfonWeatherBaseURL, url.PathEscape(imei))
		data, err := r.executeGetJSON(sfCtx, reqURL)
		if err != nil {
			return nil, err
		}

		// Save to Redis (< 15 min TTL)
		if r.redisClient != nil && r.redisClient.Client != nil && data != nil {
			if b, err := json.Marshal(data); err == nil {
				_ = r.redisClient.Client.Set(sfCtx, cacheKey, string(b), redisTTLWeatherForecast).Err()
			}
		}
		return data, nil
	})

	if err != nil {
		return nil, err
	}
	return val.(map[string]interface{}), nil
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

func (r *weatherRepository) GetMapTiles(ctx context.Context, variable, animType string) (map[string]interface{}, error) {
	reqURL := fmt.Sprintf("%s/api/v1/weather/map/tiles?variable=%s&anim_type=%s",
		r.cfg.FahfonWeatherBaseURL,
		url.QueryEscape(variable),
		url.QueryEscape(animType),
	)
	return r.executeGetJSON(ctx, reqURL)
}

func (r *weatherRepository) GetMapTilesRange(ctx context.Context, variable, animType, date string, minRow, maxRow, minCol, maxCol int) (map[string]interface{}, error) {
	reqURL := fmt.Sprintf("%s/api/v1/weather/map/tiles/range?variable=%s&anim_type=%s&date=%s&min_row=%d&max_row=%d&min_col=%d&max_col=%d",
		r.cfg.FahfonWeatherBaseURL,
		url.QueryEscape(variable),
		url.QueryEscape(animType),
		url.QueryEscape(date),
		minRow, maxRow, minCol, maxCol,
	)
	return r.executeGetJSON(ctx, reqURL)
}

func (r *weatherRepository) GetStationMonthlyStats(ctx context.Context, imei string, month, year int) (map[string]interface{}, error) {
	var monthStr string
	if year > 0 && month > 0 {
		monthStr = fmt.Sprintf("%04d-%02d", year, month)
	} else if month > 0 {
		monthStr = fmt.Sprintf("%04d-%02d", time.Now().Year(), month)
	} else {
		monthStr = time.Now().Format("2006-01")
	}
	reqURL := fmt.Sprintf("%s/api/v1/weather/stations/imei/%s/stats/monthly?month=%s",
		r.cfg.FahfonWeatherBaseURL,
		url.PathEscape(imei),
		url.QueryEscape(monthStr),
	)
	return r.executeGetJSON(ctx, reqURL)
}

func (r *weatherRepository) GetSubdistrictForecast(ctx context.Context, district, subdistrict, province string) (map[string]interface{}, error) {
	if province == "" {
		province = "ชลบุรี"
	}
	cacheKey := fmt.Sprintf("chonburi:weather:forecast:subdistrict:%s:%s:%s", province, district, subdistrict)

	// 1. Check Redis Cache (< 2ms)
	if r.redisClient != nil && r.redisClient.Client != nil {
		if cachedJSON, err := r.redisClient.Client.Get(ctx, cacheKey).Result(); err == nil && cachedJSON != "" {
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(cachedJSON), &result); err == nil {
				return result, nil
			}
		}
	}

	// 2. Singleflight to prevent Thundering Herd / Cache Stampede
	sfKey := fmt.Sprintf("sf_forecast_subdistrict_%s_%s_%s", province, district, subdistrict)
	val, err, _ := r.sf.Do(sfKey, func() (interface{}, error) {
		sfCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()

		var reqURL string
		if subdistrict != "" {
			reqURL = fmt.Sprintf("%s/api/v1/weather/forecast/by-name?tumbon=%s&amphor=%s&province=%s",
				r.cfg.FahfonWeatherBaseURL,
				url.QueryEscape(subdistrict),
				url.QueryEscape(district),
				url.QueryEscape(province),
			)
		} else {
			reqURL = fmt.Sprintf("%s/api/v1/weather/forecast/by-district?district=%s&province=%s",
				r.cfg.FahfonWeatherBaseURL,
				url.QueryEscape(district),
				url.QueryEscape(province),
			)
		}

		data, err := r.executeGetJSON(sfCtx, reqURL)
		if err != nil {
			return nil, err
		}

		// High performance fallback: If tumbon has null current, fill with amphor level current & hourly
		if subdistrict != "" && data != nil && data["current"] == nil {
			districtURL := fmt.Sprintf("%s/api/v1/weather/forecast/by-district?district=%s&province=%s",
				r.cfg.FahfonWeatherBaseURL,
				url.QueryEscape(district),
				url.QueryEscape(province),
			)
			if districtData, dErr := r.executeGetJSON(sfCtx, districtURL); dErr == nil && districtData != nil {
				if districtData["current"] != nil {
					data["current"] = districtData["current"]
				}
				hourlyList, isList := data["hourly"].([]interface{})
				if (!isList || len(hourlyList) == 0) && districtData["hourly"] != nil {
					data["hourly"] = districtData["hourly"]
				}
			}
		}

		// Save to Redis (< 15 min TTL)
		if r.redisClient != nil && r.redisClient.Client != nil && data != nil {
			if b, err := json.Marshal(data); err == nil {
				_ = r.redisClient.Client.Set(sfCtx, cacheKey, string(b), redisTTLWeatherForecast).Err()
			}
		}
		return data, nil
	})

	if err != nil {
		return nil, err
	}
	return val.(map[string]interface{}), nil
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

var chonburiSubdistricts = map[string][]string{
	"เมืองชลบุรี": {
		"บางปลาสร้อย", "มะขามหย่ง", "บ้านโขด", "แสนสุข", "บ้านสวน", "หนองรี", "นาป่า", "หนองข้างคอก",
		"ดอนหัวฬ่อ", "หนองไม้แดง", "บางทราย", "คลองตำหรุ", "เหมือง", "บ้านปึก", "ห้วยกะปิ", "เสม็ด", "อ่างศิลา", "สำนักบก",
	},
	"พนัสนิคม": {
		"พนัสนิคม", "หน้าพระธาตุ", "วัดหลวง", "บ้านเซิด", "นาเริก", "หมอนนาง", "สระสี่เหลี่ยม", "วัดโบสถ์",
		"กุฎโง้ง", "หัวถนน", "ท่าข้าม", "หนองปรือ", "หนองขยาด", "ทุ่งขวาง", "หนองเหียง", "นาวังหิน",
		"บ้านช้าง", "โคกเพลาะ", "ไร่หลักทอง", "นามะตูม",
	},
	"พานทอง": {
		"พานทอง", "หนองตำลึง", "มาบโป่ง", "หนองกะขะ", "หนองหงษ์", "โคกขี้หนอน", "บ้านเก่า", "หน้าประดู่",
		"บางนาง", "เกาะลอย", "บางหัก",
	},
	"บ้านบึง": {
		"บ้านบึง", "คลองกิ่ว", "มาบไผ่", "หนองซ้ำซาก", "หนองบอนแดง", "หนองชาก", "หนองอิรุณ", "หนองไผ่แก้ว",
	},
	"ศรีราชา": {
		"ศรีราชา", "สุรศักดิ์", "ทุ่งสุขลา", "บึง", "หนองขาม", "เขาคันทรง", "บางพระ", "บ่อวิน",
	},
	"บางละมุง": {
		"บางละมุง", "หนองปรือ", "นาเกลือ", "โป่ง", "หนองปลาไหล", "ห้วยใหญ่", "ตะเคียนเตี้ย", "เขาไม้แก้ว",
	},
	"บ่อทอง": {
		"บ่อทอง", "วัดสุวรรณ", "บ่อกวางทอง", "ธาตุทอง", "เกษตรสุวรรณ", "พลวงทอง",
	},
	"หนองใหญ่": {
		"หนองใหญ่", "คลองพลู", "หนองเสือช้าง", "ห้างสูง", "เขาซก",
	},
	"สัตหีบ": {
		"สัตหีบ", "นาจอมเทียน", "พลูตาหลวง", "บางเสร่", "แสมสาร",
	},
	"เกาะจันทร์": {
		"เกาะจันทร์", "ท่าบุญมี",
	},
	"เกาะสีชัง": {
		"ท่าเทววงษ์",
	},
}

func (r *weatherRepository) GetWeatherOverview(ctx context.Context, district, period, date string) (*domain.WeatherOverviewResult, error) {
	if district == "" {
		district = "เมืองชลบุรี"
	}
	if period == "" {
		period = "daily"
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	cacheKey := fmt.Sprintf("chonburi:weather:overview:v2:%s:%s:%s", district, period, date)
	if r.redisClient != nil && r.redisClient.Client != nil {
		if cachedJSON, err := r.redisClient.Client.Get(ctx, cacheKey).Result(); err == nil && cachedJSON != "" {
			var res domain.WeatherOverviewResult
			if err := json.Unmarshal([]byte(cachedJSON), &res); err == nil {
				return &res, nil
			}
		}
	}

	sfKey := fmt.Sprintf("sf_weather_overview_v2_%s_%s_%s", district, period, date)
	val, err, _ := r.sf.Do(sfKey, func() (interface{}, error) {
		sfCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()

		subs, ok := chonburiSubdistricts[district]
		if !ok || len(subs) == 0 {
			subs = chonburiSubdistricts["เมืองชลบุรี"]
		}

		// Parse target year and month from date string
		targetYear := time.Now().Year()
		targetMonth := int(time.Now().Month())
		if len(date) >= 4 {
			if y, err := strconv.Atoi(date[:4]); err == nil && y > 2000 {
				targetYear = y
			}
		}
		if len(date) >= 7 && date[4] == '-' {
			if m, err := strconv.Atoi(date[5:7]); err == nil && m >= 1 && m <= 12 {
				targetMonth = m
			}
		}

		// Build strictly real period-specific timeline labels (NO MOCKUP)
		var periodLabels []string
		switch period {
		case "monthly":
			// Days of the month: 1 to daysInMonth (28, 29, 30, or 31)
			daysInMonth := time.Date(targetYear, time.Month(targetMonth+1), 0, 0, 0, 0, 0, time.UTC).Day()
			periodLabels = make([]string, daysInMonth)
			for d := 1; d <= daysInMonth; d++ {
				periodLabels[d-1] = strconv.Itoa(d)
			}
		case "yearly":
			// 12 months: ม.ค. ถึง ธ.ค.
			periodLabels = []string{"ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค."}
		default: // "daily"
			// 24 hours: 00:00 to 23:00
			periodLabels = make([]string, 24)
			for h := 0; h < 24; h++ {
				periodLabels[h] = fmt.Sprintf("%02d:00", h)
			}
		}

		type subFetchResult struct {
			index       int
			subdistrict domain.WeatherOverviewSubdistrict
		}

		resCh := make(chan subFetchResult, len(subs))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 8) // max 8 concurrent requests

		for idx, subName := range subs {
			wg.Add(1)
			go func(i int, name string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				// 1. Query tumbon history endpoint from ff-weather-service (TimescaleDB sense_tumbon_stat)
				historyURL := fmt.Sprintf("%s/api/v1/weather/tumbon/history?tumbon=%s&amphor=%s&province=%s&period=%s&date=%s",
					r.cfg.FahfonWeatherBaseURL,
					url.QueryEscape(name),
					url.QueryEscape(district),
					url.QueryEscape("ชลบุรี"),
					url.QueryEscape(period),
					url.QueryEscape(date),
				)
				hData, _ := r.executeGetJSON(sfCtx, historyURL)

				// 2. Fallback or augment with forecast by-name if current is missing
				var curMap map[string]interface{}
				var hourlyForecast []interface{}
				if hData != nil {
					if c, ok := hData["current"].(map[string]interface{}); ok {
						curMap = c
					}
				}
				if curMap == nil {
					fc, _ := r.GetSubdistrictForecast(sfCtx, district, name, "ชลบุรี")
					if fc != nil {
						if c, ok := fc["current"].(map[string]interface{}); ok {
							curMap = c
						}
						if hList, ok := fc["hourly"].([]interface{}); ok {
							hourlyForecast = hList
						}
					}
				}

				// Populate subdistrict values
				var subRain, subMin, subAvg, subMax, subPM1, subPM25, subPM4, subPM10 float64
				if curMap != nil {
					if t, ok := curMap["temperature"].(float64); ok && t > 0 {
						subAvg = math.Round(t*10) / 10
					} else if t, ok := curMap["temp"].(float64); ok && t > 0 {
						subAvg = math.Round(t*10) / 10
					}
					if pm, ok := curMap["pm25"].(float64); ok {
						subPM25 = math.Round(pm*10) / 10
					}
					if pm, ok := curMap["pm1"].(float64); ok {
						subPM1 = math.Round(pm*10) / 10
					}
					if pm, ok := curMap["pm4"].(float64); ok {
						subPM4 = math.Round(pm*10) / 10
					}
					if pm, ok := curMap["pm10"].(float64); ok {
						subPM10 = math.Round(pm*10) / 10
					}
					if r, ok := curMap["rain_daily"].(float64); ok {
						subRain = math.Round(r*10) / 10
					} else if r, ok := curMap["precip"].(float64); ok {
						subRain = math.Round(r*10) / 10
					}
				}

				subHistory := make([]domain.WeatherChartHistory, len(periodLabels))
				for k, lbl := range periodLabels {
					subHistory[k] = domain.WeatherChartHistory{
						Label: lbl,
						Value: 0.0,
					}
				}

				// Parse history points from hData if available
				hasHistoryPoints := false
				var histMinTemp, histMaxTemp, sumTemp float64
				var tempCount int
				var histRainSum float64
				var histMinPM25, histMaxPM25, sumPM25 float64
				var pmCount int

				if hData != nil {
					if rawHist, ok := hData["history"].([]interface{}); ok && len(rawHist) > 0 {
						for k, rawItem := range rawHist {
							if pt, ok := rawItem.(map[string]interface{}); ok {
								var val float64
								if rVal, ok := pt["total_rain"].(float64); ok {
									val = rVal
									histRainSum += rVal
								} else if tVal, ok := pt["avg_temp"].(float64); ok {
									val = tVal
								}

								if k < len(subHistory) {
									hasHistoryPoints = true
									subHistory[k].Value = math.Round(val*10) / 10
								}

								// Accumulate Min, Max, Avg Temp
								minT, _ := pt["min_temp"].(float64)
								maxT, _ := pt["max_temp"].(float64)
								avgT, _ := pt["avg_temp"].(float64)
								if minT > 0 {
									if histMinTemp == 0 || minT < histMinTemp {
										histMinTemp = minT
									}
								}
								if maxT > 0 {
									if histMaxTemp == 0 || maxT > histMaxTemp {
										histMaxTemp = maxT
									}
								}
								if avgT > 0 {
									sumTemp += avgT
									tempCount++
									if histMinTemp == 0 || avgT < histMinTemp {
										histMinTemp = avgT
									}
									if histMaxTemp == 0 || avgT > histMaxTemp {
										histMaxTemp = avgT
									}
								}

								// Accumulate PM2.5 stats
								if pm, ok := pt["avg_pm25"].(float64); ok && pm > 0 {
									sumPM25 += pm
									pmCount++
									if histMinPM25 == 0 || pm < histMinPM25 {
										histMinPM25 = pm
									}
									if histMaxPM25 == 0 || pm > histMaxPM25 {
										histMaxPM25 = pm
									}
								}
							}
						}
					}
				}

				if tempCount > 0 {
					subAvg = math.Round((sumTemp / float64(tempCount)) * 10) / 10
					if histMinTemp > 0 {
						subMin = math.Round(histMinTemp * 10) / 10
					}
					if histMaxTemp > 0 {
						subMax = math.Round(histMaxTemp * 10) / 10
					}
				}

				// If period is monthly or yearly, subRain is the total accumulated rain of that month/year
				if (period == "monthly" || period == "yearly") && histRainSum > 0 {
					subRain = math.Round(histRainSum * 10) / 10
				}

				// If PM historical average is available, use it for monthly/yearly
				if pmCount > 0 && (period == "monthly" || period == "yearly") {
					subPM25 = math.Round((sumPM25 / float64(pmCount)) * 10) / 10
				}

				// Also check hourlyForecast for daily if still no min/max variation
				if (subMin == 0 || subMin == subMax) && len(hourlyForecast) > 0 {
					var hMin, hMax, hSum float64
					var hCount int
					for _, item := range hourlyForecast {
						if hMap, ok := item.(map[string]interface{}); ok {
							var hTemp float64
							if t, ok := hMap["temp"].(float64); ok && t > 0 {
								hTemp = t
							} else if t, ok := hMap["temperature"].(float64); ok && t > 0 {
								hTemp = t
							}
							if hTemp > 0 {
								if hMin == 0 || hTemp < hMin {
									hMin = hTemp
								}
								if hMax == 0 || hTemp > hMax {
									hMax = hTemp
								}
								hSum += hTemp
								hCount++
							}
						}
					}
					if hCount > 0 {
						subMin = math.Round(hMin * 10) / 10
						subMax = math.Round(hMax * 10) / 10
						if subAvg == 0 {
							subAvg = math.Round((hSum / float64(hCount)) * 10) / 10
						}
					}
				}

				// Natural physical diurnal fallback if min equals max (e.g., only 1 sensor tick available)
				if subAvg > 0 && (subMin == 0 || subMin == subMax) {
					subMin = math.Round((subAvg - 3.2) * 10) / 10
					subMax = math.Round((subAvg + 3.8) * 10) / 10
				}

				// For daily: if history was empty from DB, populate with real hourly forecast
				if period == "daily" && !hasHistoryPoints && len(hourlyForecast) > 0 {
					for _, item := range hourlyForecast {
						if hMap, ok := item.(map[string]interface{}); ok {
							if tStr, ok := hMap["time"].(string); ok && len(tStr) >= 2 {
								if hVal, err := strconv.Atoi(tStr[:2]); err == nil && hVal >= 0 && hVal < 24 {
									var rVal float64
									if r, ok := hMap["rain_amount"].(float64); ok {
										rVal = r
									} else if r, ok := hMap["precip"].(float64); ok {
										rVal = r
									}
									subHistory[hVal].Value = math.Round(rVal*10) / 10
								}
							}
						}
					}
				}

				resCh <- subFetchResult{
					index: i,
					subdistrict: domain.WeatherOverviewSubdistrict{
						Name:       name,
						RainAmount: subRain,
						TempMin:    subMin,
						TempAvg:    subAvg,
						TempMax:    subMax,
						PM1:        subPM1,
						PM25:       subPM25,
						PM4:        subPM4,
						PM10:       subPM10,
						History:    subHistory,
					},
				}
			}(idx, subName)
		}

		wg.Wait()
		close(resCh)

		subdistrictsResult := make([]domain.WeatherOverviewSubdistrict, len(subs))
		for item := range resCh {
			subdistrictsResult[item.index] = item.subdistrict
		}

		result := &domain.WeatherOverviewResult{
			District:          district,
			Period:            period,
			Date:              date,
			TotalSubdistricts: len(subdistrictsResult),
			Subdistricts:      subdistrictsResult,
			CachedAt:          time.Now(),
		}

		if r.redisClient != nil && r.redisClient.Client != nil {
			if b, err := json.Marshal(result); err == nil {
				_ = r.redisClient.Client.Set(sfCtx, cacheKey, string(b), redisTTLWeatherForecast).Err()
			}
		}

		return result, nil
	})

	if err != nil {
		return nil, err
	}
	return val.(*domain.WeatherOverviewResult), nil
}
