package usecase

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"super-app-chonburi-go-mobile/internal/domain"
)

const maxSubscriptionsPerUser = 5

type weatherSubscriptionUseCase struct {
	repo domain.WeatherSubscriptionRepository
}

// NewWeatherSubscriptionUseCase creates the subscription business logic handler.
func NewWeatherSubscriptionUseCase(repo domain.WeatherSubscriptionRepository) domain.WeatherSubscriptionUseCase {
	return &weatherSubscriptionUseCase{repo: repo}
}

// GetSubscriptions returns all active weather area subscriptions for a user.
func (u *weatherSubscriptionUseCase) GetSubscriptions(userID uuid.UUID) ([]domain.WeatherSubscriptionResponse, error) {
	subs, err := u.repo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	resp := make([]domain.WeatherSubscriptionResponse, 0, len(subs))
	for _, s := range subs {
		resp = append(resp, domain.WeatherSubscriptionResponse{
			ID:          s.ID,
			District:    s.District,
			Subdistrict: s.Subdistrict,
			IsPrimary:   s.IsPrimary,
			IsActive:    s.IsActive,
		})
	}
	return resp, nil
}

// AddSubscription adds a new area for the user to receive alerts from.
// Business rules:
//   - Maximum 5 active subscriptions per user
//   - Duplicate subdistrict is rejected (DB UNIQUE constraint will also catch this)
//   - New subscriptions are non-primary (IsPrimary = false)
func (u *weatherSubscriptionUseCase) AddSubscription(userID uuid.UUID, req *domain.WeatherSubscriptionRequest) error {
	if req.District == "" || req.Subdistrict == "" {
		return errors.New("district and subdistrict are required")
	}

	count, err := u.repo.CountByUserID(userID)
	if err != nil {
		return err
	}
	if count >= maxSubscriptionsPerUser {
		return errors.New("สามารถติดตามพื้นที่ได้สูงสุด 5 ตำบลเท่านั้น")
	}

	sub := &domain.UserWeatherSubscription{
		UserID:      userID,
		District:    req.District,
		Subdistrict: req.Subdistrict,
		IsPrimary:   false,
		IsActive:    true,
	}
	return u.repo.Create(sub)
}

// RemoveSubscription deactivates a secondary subscription.
// Primary subscriptions (from home address) cannot be removed via this method.
func (u *weatherSubscriptionUseCase) RemoveSubscription(id uuid.UUID, userID uuid.UUID) error {
	return u.repo.Delete(id, userID)
}

// SyncPrimaryFromAddress is called after UpdateAddress to automatically create/update
// the user's primary weather subscription to match their registered home address.
// This ensures zero-touch default opt-in for all registered citizens.
func (u *weatherSubscriptionUseCase) SyncPrimaryFromAddress(userID uuid.UUID, district, subdistrict string) error {
	if district == "" || subdistrict == "" {
		return nil // Address not filled, skip
	}
	return u.repo.UpsertPrimary(userID, district, subdistrict)
}

// GetAlertHistory returns recent weather alerts matching the user's subscribed areas.
// Format timestamp into relative Thai time (e.g. "10 นาทีที่แล้ว", "วันนี้ 11:34 น.").
func (u *weatherSubscriptionUseCase) GetAlertHistory(userID uuid.UUID, limit int) ([]domain.WeatherAlertFeedResponse, error) {
	subs, err := u.repo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	var subdistricts []string
	for _, s := range subs {
		if s.Subdistrict != "" {
			subdistricts = append(subdistricts, s.Subdistrict)
		}
	}

	alerts, err := u.repo.GetAlertHistory(subdistricts, limit)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	resp := make([]domain.WeatherAlertFeedResponse, 0, len(alerts))
	for _, a := range alerts {
		// Calculate user-friendly Thai relative time
		diff := now.Sub(a.CreatedAt)
		var timeStr string
		if diff < time.Minute {
			timeStr = "เมื่อสักครู่"
		} else if diff < time.Hour {
			timeStr = fmt.Sprintf("%d นาทีที่แล้ว", int(diff.Minutes()))
		} else if diff < 24*time.Hour {
			timeStr = fmt.Sprintf("วันนี้ %s น.", a.CreatedAt.Format("15:04"))
		} else {
			timeStr = a.CreatedAt.Format("02/01/2006 15:04 น.")
		}

		resp = append(resp, domain.WeatherAlertFeedResponse{
			ID:        a.ID.String(),
			Title:     a.Title,
			Message:   a.Message,
			Timestamp: timeStr,
			Type:      a.AlertType,
			Level:     a.Severity,
			ImageURL:  a.ImageURL,
		})
	}
	return resp, nil
}

