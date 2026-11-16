package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/repository"
)

var ErrInvalidWaterStationSerial = errors.New("water monitoring station not found")

type WaterLevelUseCase interface {
	GetStations(ctx context.Context) ([]domain.WaterStation, error)
	GetDeviceDetail(ctx context.Context, serial string) (*domain.WaterDeviceDetail, error)
	GetDeviceHistory(ctx context.Context, serial string, timeframe int) ([]domain.WaterHistoryPoint, error)
	GetDeviceForecast(ctx context.Context, serial string, days int, date string) (*domain.WaterForecastResult, error)
	GetSnapshots(ctx context.Context, serial, date string) ([]domain.WaterSnapshotLog, error)
	InitStream(ctx context.Context, serial string) (*domain.WaterStreamResult, error)
	TerminateStream(ctx context.Context, serial string) error
	GetActiveAlerts(ctx context.Context) ([]domain.WaterAlert, error)
	RelayWebRTCOffer(ctx context.Context, serial, sdpOffer string) (string, error)
}

type waterLevelUseCase struct {
	repo repository.WaterLevelRepository
}

func NewWaterLevelUseCase(repo repository.WaterLevelRepository) WaterLevelUseCase {
	return &waterLevelUseCase{
		repo: repo,
	}
}

func parseCoordinate(val any) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0
}

func formatThaiTime(isoStr string) string {
	if isoStr == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, isoStr)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", isoStr)
		if err != nil {
			return isoStr
		}
	}
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err == nil {
		t = t.In(loc)
	}
	return fmt.Sprintf("%02d:%02d น.", t.Hour(), t.Minute())
}

func mapWarningStatus(status string) string {
	switch status {
	case "WARNING":
		return "warning"
	case "CRITICAL":
		return "critical"
	case "EMERGENCY":
		return "emergency"
	default:
		return "normal"
	}
}

func (u *waterLevelUseCase) GetStations(ctx context.Context) ([]domain.WaterStation, error) {
	devices, err := u.repo.FetchGatewayDevices(ctx)
	if err != nil {
		log.Printf("⚠️ [WaterLevelUseCase] Error fetching devices: %v", err)
		return []domain.WaterStation{}, err
	}

	result := make([]domain.WaterStation, 0, len(devices))
	for _, dev := range devices {
		lat := parseCoordinate(dev.InstallLatitude)
		if lat == 0 {
			lat = dev.Latitude
		}
		lng := parseCoordinate(dev.InstallLongitude)
		if lng == 0 {
			lng = dev.Longitude
		}

		waterLevelM := dev.NetWaterLevelCM / 100.0
		waterLevelMaxM := dev.BottomToBankCM / 100.0

		pctToBank := dev.WaterLevelPct
		if pctToBank == 0 && dev.BottomToBankCM > 0 {
			pctToBank = math.Round((dev.NetWaterLevelCM/dev.BottomToBankCM)*1000) / 10
		}

		sensorMode := "RIVER"
		if dev.BottomToBankCM == 0 {
			sensorMode = "SURFACE"
		}

		sub, dist := repository.ResolveSubdistrict(lat, lng)
		locName := dev.Location
		if locName == "" || locName == "samphran" {
			locName = fmt.Sprintf("ต.%s อ.%s", sub, dist)
		}

		st := domain.WaterStation{
			ID:              dev.Serial,
			Serial:          dev.Serial,
			Name:            dev.Name,
			LocationName:    locName,
			Lat:             lat,
			Lng:             lng,
			Subdistrict:     sub,
			District:        dist,
			WaterLevel:      math.Round(waterLevelM*100) / 100,
			WaterLevelMax:   math.Round(waterLevelMaxM*100) / 100,
			RainAmount:      dev.RainAmount,
			WindSpeed:       dev.WindSpeed,
			WindDirection:   dev.WindDirection,
			Status:          mapWarningStatus(dev.WarningStatus),
			LastUpdated:     formatThaiTime(dev.LastSeen),
			SnapshotURL:     dev.SnapshotURL0,
			CCTV:            dev.CCTVStreamURL,
			CommonSizeName:  dev.Name,
			PctToBank:       pctToBank,
			SensorMode:      sensorMode,
			BottomToBankCM:  dev.BottomToBankCM,
			NetWaterLevelCM: dev.NetWaterLevelCM,
			HasForecast:     dev.HasForecast,
		}

		if st.SnapshotURL == "" {
			st.SnapshotURL = dev.CCTVPath0
		}

		result = append(result, st)
	}

	return result, nil
}

func (u *waterLevelUseCase) GetDeviceDetail(ctx context.Context, serial string) (*domain.WaterDeviceDetail, error) {
	envelope, err := u.repo.FetchGatewayDeviceDetail(ctx, serial)
	if err != nil {
		return nil, err
	}

	dev := envelope.Device
	cur := envelope.Current
	cfg := envelope.Config

	lat := parseCoordinate(dev.InstallLatitude)
	lng := parseCoordinate(dev.InstallLongitude)

	waterLevelM := cur.NetWaterLevelCM / 100.0
	waterLevelMaxM := cfg.BottomToBankCM / 100.0

	pctToBank := cur.WaterLevelPct
	if pctToBank == 0 && cfg.BottomToBankCM > 0 {
		pctToBank = math.Round((cur.NetWaterLevelCM/cfg.BottomToBankCM)*1000) / 10
	}

	snapURL := dev.SnapshotURL0
	if snapURL == "" {
		snapURL = dev.CCTVPath0
	}

	cctvURL := dev.CCTVStreamURL
	if cctvURL == "" {
		cctvURL = dev.SnapshotURL1
	}

	sub, dist := repository.ResolveSubdistrict(lat, lng)
	locName := dev.OrganizationName
	if locName == "" || locName == "samphran" {
		locName = fmt.Sprintf("ต.%s อ.%s", sub, dist)
	}

	return &domain.WaterDeviceDetail{
		ID:            dev.Serial,
		Serial:        dev.Serial,
		Name:          dev.Name,
		Location:      locName,
		Subdistrict:   sub,
		District:      dist,
		Lat:           lat,
		Lng:           lng,
		Status:        mapWarningStatus(dev.WarningStatus),
		WaterLevel:    math.Round(waterLevelM*100) / 100,
		WaterLevelMax: math.Round(waterLevelMaxM*100) / 100,
		LastUpdated:   formatThaiTime(cur.Time),
		Battery:       cur.Battery,
		Solar:         cur.Solar,
		CCTVStreamURL: cctvURL,
		SnapshotURL:   snapURL,
		CCTV:          cctvURL,
		PctToBank:     pctToBank,
		HasForecast:   envelope.HasForecast,
	}, nil
}

func (u *waterLevelUseCase) GetDeviceHistory(ctx context.Context, serial string, timeframe int) ([]domain.WaterHistoryPoint, error) {
	rawPoints, err := u.repo.FetchGatewayDeviceHistory(ctx, serial, timeframe)
	if err != nil {
		return []domain.WaterHistoryPoint{}, err
	}

	loc, _ := time.LoadLocation("Asia/Bangkok")
	points := make([]domain.WaterHistoryPoint, 0, len(rawPoints))

	for _, pt := range rawPoints {
		t, err := time.Parse(time.RFC3339, pt.Time)
		if err != nil {
			t, _ = time.Parse("2006-01-02 15:04:05", pt.Time)
		}
		if loc != nil {
			t = t.In(loc)
		}

		waterLevelM := pt.NetWaterLevelCM / 100.0

		points = append(points, domain.WaterHistoryPoint{
			Time:       pt.Time,
			WaterLevel: math.Round(waterLevelM*100) / 100,
			RainAmount: pt.RainAmount,
			WindSpeed:  pt.WindSpeed,
			HourLabel:  fmt.Sprintf("%02d:00", t.Hour()),
			DateLabel:  t.Format("02/01"),
			ISODate:    t.Format(time.RFC3339),
			Pct:        pt.WaterLevelPct,
		})
	}

	return points, nil
}

func (u *waterLevelUseCase) GetDeviceForecast(ctx context.Context, serial string, days int, date string) (*domain.WaterForecastResult, error) {
	rawResp, err := u.repo.FetchGatewayDeviceForecast(ctx, serial, days, date)
	if err != nil {
		return nil, err
	}

	loc, _ := time.LoadLocation("Asia/Bangkok")
	points := make([]domain.WaterHistoryPoint, 0, len(rawResp.Items))

	for _, pt := range rawResp.Items {
		t, err := time.Parse(time.RFC3339, pt.Time)
		if err != nil {
			t, _ = time.Parse("2006-01-02 15:04:05", pt.Time)
		}
		if loc != nil {
			t = t.In(loc)
		}

		waterLevelM := pt.NetWaterLevelCM / 100.0
		points = append(points, domain.WaterHistoryPoint{
			Time:       t.Format(time.RFC3339),
			WaterLevel: math.Round(waterLevelM*100) / 100,
			HourLabel:  fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute()),
			DateLabel:  t.Format("02/01"),
			ISODate:    t.Format(time.RFC3339),
			Pct:        pt.WaterLevelPct,
			Status:     mapWarningStatus(pt.WarningStatus),
		})
	}

	return &domain.WaterForecastResult{
		Success:         true,
		Serial:          serial,
		HasForecast:     rawResp.HasForecast,
		IntervalMinutes: rawResp.IntervalMinutes,
		MinDate:         rawResp.MinDate,
		MaxDate:         rawResp.MaxDate,
		Items:           points,
	}, nil
}

func (u *waterLevelUseCase) GetSnapshots(ctx context.Context, serial, date string) ([]domain.WaterSnapshotLog, error) {
	rawLogs, err := u.repo.FetchGatewaySnapshots(ctx, serial, date)
	if err != nil {
		return []domain.WaterSnapshotLog{}, err
	}

	logs := make([]domain.WaterSnapshotLog, 0, len(rawLogs))
	for _, l := range rawLogs {
		waterLevelM := l.NetWaterLevelCM / 100.0
		logs = append(logs, domain.WaterSnapshotLog{
			Time:       formatThaiTime(l.Time),
			ImageURL:   l.ImageURL,
			WaterLevel: math.Round(waterLevelM*100) / 100,
		})
	}
	return logs, nil
}

func (u *waterLevelUseCase) validateDeviceSerial(ctx context.Context, serial string) error {
	clean := strings.TrimSpace(serial)
	if clean == "" || len(clean) > 64 {
		return ErrInvalidWaterStationSerial
	}
	devices, err := u.repo.FetchGatewayDevices(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify station serial: %w", err)
	}
	for _, dev := range devices {
		if dev.Serial == clean {
			return nil
		}
	}
	return ErrInvalidWaterStationSerial
}

func (u *waterLevelUseCase) InitStream(ctx context.Context, serial string) (*domain.WaterStreamResult, error) {
	if err := u.validateDeviceSerial(ctx, serial); err != nil {
		return nil, err
	}
	return u.repo.InitGatewayStream(ctx, serial)
}

func (u *waterLevelUseCase) TerminateStream(ctx context.Context, serial string) error {
	if err := u.validateDeviceSerial(ctx, serial); err != nil {
		return err
	}
	return u.repo.TerminateGatewayStream(ctx, serial)
}

func (u *waterLevelUseCase) GetActiveAlerts(ctx context.Context) ([]domain.WaterAlert, error) {
	devices, err := u.repo.FetchGatewayDevices(ctx)
	if err != nil {
		devices = []domain.GatewayRiverDevice{}
	}

	chonburiSerials := make(map[string]domain.GatewayRiverDevice, len(devices))
	for _, dev := range devices {
		chonburiSerials[dev.Serial] = dev
	}

	items, err := u.repo.FetchActiveAlerts(ctx)
	if err != nil {
		return []domain.WaterAlert{}, nil
	}

	alerts := make([]domain.WaterAlert, 0, len(items))
	for _, item := range items {
		dev, exists := chonburiSerials[item.Serial]
		if !exists || item.Threshold <= 0 {
			continue
		}

		lat := parseCoordinate(dev.InstallLatitude)
		if lat == 0 {
			lat = dev.Latitude
		}
		lng := parseCoordinate(dev.InstallLongitude)
		if lng == 0 {
			lng = dev.Longitude
		}
		sub, dist := repository.ResolveSubdistrict(lat, lng)

		locName := dev.Name
		if locName == "" {
			locName = dev.Location
		}
		if locName == "" || locName == "samphran" {
			locName = fmt.Sprintf("ต.%s อ.%s", sub, dist)
		}

		alerts = append(alerts, domain.WaterAlert{
			ID:          fmt.Sprintf("%v", item.ID),
			Title:       fmt.Sprintf("แจ้งเตือนสถานี %s", locName),
			Detail:      item.Message,
			Location:    locName,
			Subdistrict: sub,
			District:    dist,
			Time:        formatThaiTime(item.Time),
			Severity:    mapWarningStatus(item.AlertLevel),
			Unread:      true,
		})
	}
	return alerts, nil
}

func (u *waterLevelUseCase) RelayWebRTCOffer(ctx context.Context, serial, sdpOffer string) (string, error) {
	if err := u.validateDeviceSerial(ctx, serial); err != nil {
		return "", err
	}
	return u.repo.RelayWebRTCOffer(ctx, serial, sdpOffer)
}
