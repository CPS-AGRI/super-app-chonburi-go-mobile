package usecase

import (
	"context"

	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/repository"
)

type WeatherUseCase interface {
	GetAllStations(ctx context.Context) (*domain.WeatherStationsResult, error)
	GetStationByIMEI(ctx context.Context, imei string) (map[string]interface{}, error)
	GetStationForecast(ctx context.Context, imei string) (map[string]interface{}, error)
	GetStationGraph(ctx context.Context, imei, variable string, timeframe int, date string) (map[string]interface{}, error)
	GetMapFrames(ctx context.Context, variable, animType, date string) (map[string]interface{}, error)
	FlushCache(ctx context.Context) error
}

type weatherUseCase struct {
	repo repository.WeatherRepository
}

func NewWeatherUseCase(repo repository.WeatherRepository) WeatherUseCase {
	return &weatherUseCase{
		repo: repo,
	}
}

func (u *weatherUseCase) GetAllStations(ctx context.Context) (*domain.WeatherStationsResult, error) {
	return u.repo.FetchAllStations(ctx)
}

func (u *weatherUseCase) GetStationByIMEI(ctx context.Context, imei string) (map[string]interface{}, error) {
	return u.repo.GetStationByIMEI(ctx, imei)
}

func (u *weatherUseCase) GetStationForecast(ctx context.Context, imei string) (map[string]interface{}, error) {
	return u.repo.GetStationForecast(ctx, imei)
}

func (u *weatherUseCase) GetStationGraph(ctx context.Context, imei, variable string, timeframe int, date string) (map[string]interface{}, error) {
	if timeframe <= 0 {
		timeframe = 24
	}
	if variable == "" {
		variable = "temp"
	}
	return u.repo.GetStationGraph(ctx, imei, variable, timeframe, date)
}

func (u *weatherUseCase) GetMapFrames(ctx context.Context, variable, animType, date string) (map[string]interface{}, error) {
	if variable == "" {
		variable = "nrtrr"
	}
	if animType == "" {
		animType = "nrt"
	}
	return u.repo.GetMapFrames(ctx, variable, animType, date)
}

func (u *weatherUseCase) FlushCache(ctx context.Context) error {
	return u.repo.FlushStationsCache(ctx)
}
