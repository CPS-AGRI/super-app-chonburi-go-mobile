package usecase_test

import (
	"context"
	"testing"
	"time"

	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/usecase"

	"github.com/stretchr/testify/assert"
)

type mockWaterLevelRepo struct{}

func (m *mockWaterLevelRepo) FetchGatewayDevices(ctx context.Context) ([]domain.GatewayRiverDevice, error) {
	return []domain.GatewayRiverDevice{
		{Serial: "CANAL-01", Name: "คลองพัทยา", BottomToBankCM: 300, NetWaterLevelCM: 120, HasForecast: true},
		{Serial: "ROAD-01", Name: "ถนนสุขุมวิท", BottomToBankCM: 0, NetWaterLevelCM: 5, HasForecast: false},
	}, nil
}

func (m *mockWaterLevelRepo) FetchGatewayDeviceDetail(ctx context.Context, serial string) (*domain.GatewayDeviceDetailEnvelope, error) {
	return &domain.GatewayDeviceDetailEnvelope{HasForecast: serial == "CANAL-01"}, nil
}

func (m *mockWaterLevelRepo) FetchGatewayDeviceHistory(ctx context.Context, serial string, timeframe int) ([]domain.GatewayHistoryPoint, error) {
	return nil, nil
}

func (m *mockWaterLevelRepo) FetchGatewayDeviceForecast(ctx context.Context, serial string, days int, date string) (*domain.GatewayForecastResponse, error) {
	if serial != "CANAL-01" {
		return &domain.GatewayForecastResponse{
			Serial:          serial,
			HasForecast:     false,
			IntervalMinutes: 5,
			Items:           []domain.GatewayForecastPoint{},
		}, nil
	}
	bkk := time.FixedZone("ICT", 7*3600)
	t1 := time.Date(2026, 11, 11, 13, 5, 0, 0, bkk)
	t2 := time.Date(2026, 11, 11, 13, 10, 0, 0, bkk)
	return &domain.GatewayForecastResponse{
		Serial:          serial,
		HasForecast:     true,
		IntervalMinutes: 5,
		MinDate:         "2026-11-11",
		MaxDate:         "2026-11-24",
		Items: []domain.GatewayForecastPoint{
			{Time: t1.Format(time.RFC3339), NetWaterLevelCM: 145.5, WaterLevelPct: 48.5, WarningStatus: "NORMAL"},
			{Time: t2.Format(time.RFC3339), NetWaterLevelCM: 150.0, WaterLevelPct: 50.0, WarningStatus: "NORMAL"},
		},
	}, nil
}

func (m *mockWaterLevelRepo) FetchGatewaySnapshots(ctx context.Context, serial, date string) ([]domain.GatewaySnapshotLog, error) {
	return nil, nil
}

func (m *mockWaterLevelRepo) InitGatewayStream(ctx context.Context, serial string) (*domain.WaterStreamResult, error) {
	return &domain.WaterStreamResult{StreamURL: "https://example.com/stream/" + serial}, nil
}

func (m *mockWaterLevelRepo) TerminateGatewayStream(ctx context.Context, serial string) error {
	return nil
}

func (m *mockWaterLevelRepo) FetchActiveAlerts(ctx context.Context) ([]domain.GatewayAlertItem, error) {
	return nil, nil
}

func (m *mockWaterLevelRepo) RelayWebRTCOffer(ctx context.Context, serial, sdpOffer string) (string, error) {
	return "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\n", nil
}

func TestGetDeviceForecast_HasForecastAndNoForecast(t *testing.T) {
	uc := usecase.NewWaterLevelUseCase(&mockWaterLevelRepo{})

	// 1. Station with forecast returns 5-min points with minute-level HourLabel
	res, err := uc.GetDeviceForecast(context.Background(), "CANAL-01", 1, "")
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.True(t, res.HasForecast)
	assert.Equal(t, 5, res.IntervalMinutes)
	assert.Equal(t, "2026-11-11", res.MinDate)
	assert.Equal(t, "2026-11-24", res.MaxDate)
	assert.Len(t, res.Items, 2)
	assert.Equal(t, 1.46, res.Items[0].WaterLevel)
	assert.Equal(t, "13:05", res.Items[0].HourLabel)
	assert.Equal(t, "13:10", res.Items[1].HourLabel)

	// 2. Station without forecast returns hasForecast: false & empty items
	noRes, err := uc.GetDeviceForecast(context.Background(), "ROAD-01", 1, "")
	assert.NoError(t, err)
	assert.False(t, noRes.HasForecast)
	assert.Empty(t, noRes.Items)

	// 3. GetStations carries hasForecast flag accurately
	stations, err := uc.GetStations(context.Background())
	assert.NoError(t, err)
	assert.Len(t, stations, 2)
	assert.True(t, stations[0].HasForecast)
	assert.False(t, stations[1].HasForecast)
}

func TestStreamSerialWhitelistGuard(t *testing.T) {
	uc := usecase.NewWaterLevelUseCase(&mockWaterLevelRepo{})

	// Valid serial passes
	streamRes, err := uc.InitStream(context.Background(), "CANAL-01")
	assert.NoError(t, err)
	assert.NotNil(t, streamRes)

	ans, err := uc.RelayWebRTCOffer(context.Background(), "CANAL-01", "v=0")
	assert.NoError(t, err)
	assert.Contains(t, ans, "v=0")

	err = uc.TerminateStream(context.Background(), "CANAL-01")
	assert.NoError(t, err)

	// Invalid / unknown serial blocked
	_, err = uc.InitStream(context.Background(), "../etc/passwd")
	assert.ErrorIs(t, err, usecase.ErrInvalidWaterStationSerial)

	_, err = uc.RelayWebRTCOffer(context.Background(), "UNKNOWN-99", "v=0")
	assert.ErrorIs(t, err, usecase.ErrInvalidWaterStationSerial)

	err = uc.TerminateStream(context.Background(), "UNKNOWN-99")
	assert.ErrorIs(t, err, usecase.ErrInvalidWaterStationSerial)
}

