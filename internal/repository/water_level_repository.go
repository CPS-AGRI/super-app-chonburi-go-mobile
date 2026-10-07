package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/infrastructure"

	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

const (
	redisKeyWaterStationsAll = "chonburi:water:stations:all"
	redisTTLWaterStationsAll = 120 * time.Second
	l1TTLWaterStationsAll    = 20 * time.Second
)

type WaterLevelRepository interface {
	FetchGatewayDevices(ctx context.Context) ([]domain.GatewayRiverDevice, error)
	FetchGatewayDeviceDetail(ctx context.Context, serial string) (*domain.GatewayDeviceDetailEnvelope, error)
	FetchGatewayDeviceHistory(ctx context.Context, serial string, timeframe int) ([]domain.GatewayHistoryPoint, error)
	FetchGatewaySnapshots(ctx context.Context, serial, date string) ([]domain.GatewaySnapshotLog, error)
	InitGatewayStream(ctx context.Context, serial string) (*domain.WaterStreamResult, error)
	TerminateGatewayStream(ctx context.Context, serial string) error
	FetchActiveAlerts(ctx context.Context) ([]domain.GatewayAlertItem, error)
	RelayWebRTCOffer(ctx context.Context, serial, sdpOffer string) (string, error)
}

type waterLevelRepository struct {
	db          *gorm.DB
	cfg         *config.Config
	redisClient *infrastructure.RedisClient
	httpClient  *http.Client
	sf          singleflight.Group

	l1Mu       sync.RWMutex
	l1Cache    []domain.GatewayRiverDevice
	l1CachedAt time.Time
}

func NewWaterLevelRepository(db *gorm.DB, cfg *config.Config, redisClient *infrastructure.RedisClient) WaterLevelRepository {
	return &waterLevelRepository{
		db:          db,
		cfg:         cfg,
		redisClient: redisClient,
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 50,
				MaxConnsPerHost:     100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (r *waterLevelRepository) buildGatewayRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	baseURL := r.cfg.RiverServiceURL
	if baseURL == "" {
		baseURL = "http://10.1.2.19:8081"
	}
	fullURL := fmt.Sprintf("%s%s", baseURL, path)

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-gateway-meta-email", "all")
	if r.cfg.RiverMunicipalityID != "" {
		req.Header.Set("x-gateway-meta-municipality-id", r.cfg.RiverMunicipalityID)
		req.Header.Set("x-client-meta-municipality-id", r.cfg.RiverMunicipalityID)
	}
	req.Header.Set("x-client-meta-page-code", "water-level-cctv")
	req.Header.Set("x-gateway-meta-page-code", "water-level-cctv")

	return req, nil
}

func (r *waterLevelRepository) FetchGatewayDevices(ctx context.Context) ([]domain.GatewayRiverDevice, error) {
	// 1. Check L1 Memory Cache
	r.l1Mu.RLock()
	if r.l1Cache != nil && time.Since(r.l1CachedAt) < l1TTLWaterStationsAll {
		cached := r.l1Cache
		r.l1Mu.RUnlock()
		return cached, nil
	}
	r.l1Mu.RUnlock()

	redisKey := fmt.Sprintf("chonburi:water:stations:%s", r.cfg.RiverMunicipalityID)
	if r.cfg.RiverMunicipalityID == "" {
		redisKey = redisKeyWaterStationsAll
	}

	// 2. Singleflight coalesce concurrent requests
	res, err, _ := r.sf.Do("fetch_gateway_devices_"+r.cfg.RiverMunicipalityID, func() (interface{}, error) {
		// 3. Check Redis L2 Cache
		if r.redisClient != nil {
			var cached []domain.GatewayRiverDevice
			if err := r.redisClient.GetJSON(ctx, redisKey, &cached); err == nil && len(cached) > 0 {
				r.l1Mu.Lock()
				r.l1Cache = cached
				r.l1CachedAt = time.Now()
				r.l1Mu.Unlock()
				return cached, nil
			}
		}

		// 4. Fetch Live from ff-river-services
		req, err := r.buildGatewayRequest(ctx, http.MethodGet, "/api/v1/river/devices", nil)
		if err != nil {
			return nil, err
		}

		resp, err := r.httpClient.Do(req)
		if err != nil {
			log.Printf("⚠️ [WaterLevelRepo] Failed to fetch live devices: %v", err)
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("gateway error %d: %s", resp.StatusCode, string(body))
		}

		var devices []domain.GatewayRiverDevice
		if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
			return nil, err
		}

		// Update L1 Cache
		r.l1Mu.Lock()
		r.l1Cache = devices
		r.l1CachedAt = time.Now()
		r.l1Mu.Unlock()

		// Update L2 Redis Cache asynchronously
		if r.redisClient != nil {
			go func(d []domain.GatewayRiverDevice, key string) {
				_ = r.redisClient.SetJSON(context.Background(), key, d, redisTTLWaterStationsAll)
			}(devices, redisKey)
		}

		return devices, nil
	})

	if err != nil {
		// Resilience: Return stale L1 cache if available
		r.l1Mu.RLock()
		if r.l1Cache != nil {
			cached := r.l1Cache
			r.l1Mu.RUnlock()
			return cached, nil
		}
		r.l1Mu.RUnlock()
		return nil, err
	}

	return res.([]domain.GatewayRiverDevice), nil
}

func (r *waterLevelRepository) FetchGatewayDeviceDetail(ctx context.Context, serial string) (*domain.GatewayDeviceDetailEnvelope, error) {
	path := fmt.Sprintf("/api/v1/river/devices/%s/detail", url.PathEscape(serial))
	req, err := r.buildGatewayRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway error %d: %s", resp.StatusCode, string(body))
	}

	var dev domain.GatewayDeviceDetailEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&dev); err != nil {
		return nil, err
	}
	return &dev, nil
}

func (r *waterLevelRepository) FetchGatewayDeviceHistory(ctx context.Context, serial string, timeframe int) ([]domain.GatewayHistoryPoint, error) {
	if timeframe <= 0 {
		timeframe = 168 // 7 days default
	}
	startTime := time.Now().Add(-time.Duration(timeframe) * time.Hour).UTC().Format(time.RFC3339)
	endTime := time.Now().UTC().Format(time.RFC3339)

	path := fmt.Sprintf("/api/v1/river/devices/%s/stats?start=%s&end=%s",
		url.PathEscape(serial), url.QueryEscape(startTime), url.QueryEscape(endTime))
	req, err := r.buildGatewayRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway error %d: %s", resp.StatusCode, string(body))
	}

	var points []domain.GatewayHistoryPoint
	if err := json.NewDecoder(resp.Body).Decode(&points); err != nil {
		return nil, err
	}
	return points, nil
}

func (r *waterLevelRepository) FetchGatewaySnapshots(ctx context.Context, serial, date string) ([]domain.GatewaySnapshotLog, error) {
	path := fmt.Sprintf("/api/v1/river/devices/%s/snapshots", url.PathEscape(serial))
	if date != "" {
		path = fmt.Sprintf("%s?date=%s", path, url.QueryEscape(date))
	}

	req, err := r.buildGatewayRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway error %d: %s", resp.StatusCode, string(body))
	}

	var logs []domain.GatewaySnapshotLog
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *waterLevelRepository) InitGatewayStream(ctx context.Context, serial string) (*domain.WaterStreamResult, error) {
	path := fmt.Sprintf("/api/v1/river/devices/%s/stream", url.PathEscape(serial))
	req, err := r.buildGatewayRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result domain.WaterStreamResult
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		_ = json.NewDecoder(resp.Body).Decode(&result)
		result.Success = true
		return &result, nil
	}

	body, _ := io.ReadAll(resp.Body)
	return &domain.WaterStreamResult{
		Success: false,
		Message: string(body),
	}, nil
}

func (r *waterLevelRepository) TerminateGatewayStream(ctx context.Context, serial string) error {
	path := fmt.Sprintf("/api/v1/river/devices/%s/stream", url.PathEscape(serial))
	req, err := r.buildGatewayRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (r *waterLevelRepository) FetchActiveAlerts(ctx context.Context) ([]domain.GatewayAlertItem, error) {
	// First try internal alerts endpoint (no meta required)
	baseURL := r.cfg.RiverServiceURL
	if baseURL == "" {
		baseURL = "http://10.1.2.19:8081"
	}
	path := fmt.Sprintf("%s/api/v1/internal/alerts/active", baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("alerts error %d: %s", resp.StatusCode, string(body))
	}

	var alerts []domain.GatewayAlertItem
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		return nil, err
	}
	return alerts, nil
}

func (r *waterLevelRepository) RelayWebRTCOffer(ctx context.Context, serial, sdpOffer string) (string, error) {
	go2rtcURL := r.cfg.RiverGo2RTCURL
	if go2rtcURL == "" {
		go2rtcURL = "https://cctv.mueangsmart.com"
	}
	targetURL := fmt.Sprintf("%s/api/webrtc?src=%s", go2rtcURL, url.QueryEscape(serial))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(sdpOffer))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/sdp")
	req.Header.Set("Origin", "https://cctv.mueangsmart.com")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		// Fallback to public domain
		fallbackURL := fmt.Sprintf("https://cctv.mueangsmart.com/api/webrtc?src=%s", url.QueryEscape(serial))
		req2, err2 := http.NewRequestWithContext(ctx, http.MethodPost, fallbackURL, strings.NewReader(sdpOffer))
		if err2 != nil {
			return "", err2
		}
		req2.Header.Set("Content-Type", "application/sdp")
		req2.Header.Set("Origin", "https://cctv.mueangsmart.com")
		resp2, err2 := r.httpClient.Do(req2)
		if err2 != nil {
			return "", fmt.Errorf("webrtc offer failed: %v", err2)
		}
		defer resp2.Body.Close()
		sdpAnswer, _ := io.ReadAll(resp2.Body)
		return string(sdpAnswer), nil
	}
	defer resp.Body.Close()

	sdpAnswer, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(sdpAnswer), nil
}
