package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"super-app-chonburi-go-mobile/internal/domain"
)

type weatherSubscriptionRepository struct {
	db *gorm.DB
}

// NewWeatherSubscriptionRepository creates a new subscription repository backed by PostgreSQL.
func NewWeatherSubscriptionRepository(db *gorm.DB) domain.WeatherSubscriptionRepository {
	return &weatherSubscriptionRepository{db: db}
}

// GetByUserID returns all active subscriptions for a user, ordered primary first.
func (r *weatherSubscriptionRepository) GetByUserID(userID uuid.UUID) ([]domain.UserWeatherSubscription, error) {
	var subs []domain.UserWeatherSubscription
	err := r.db.
		Where("user_id = ? AND is_active = TRUE", userID).
		Order("is_primary DESC, created_at ASC").
		Find(&subs).Error
	return subs, err
}

// CountByUserID returns the count of active subscriptions for a user.
func (r *weatherSubscriptionRepository) CountByUserID(userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Model(&domain.UserWeatherSubscription{}).
		Where("user_id = ? AND is_active = TRUE", userID).
		Count(&count).Error
	return count, err
}

// UpsertPrimary creates or updates the primary subscription derived from the user's registered address.
// Called automatically when a user registers or updates their address.
func (r *weatherSubscriptionRepository) UpsertPrimary(userID uuid.UUID, district, subdistrict string) error {
	if district == "" || subdistrict == "" {
		return nil // no address info yet, skip silently
	}

	var existing domain.UserWeatherSubscription
	err := r.db.Where("user_id = ? AND is_primary = TRUE", userID).First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new primary subscription
		return r.db.Create(&domain.UserWeatherSubscription{
			UserID:      userID,
			District:    district,
			Subdistrict: subdistrict,
			IsPrimary:   true,
			IsActive:    true,
		}).Error
	}

	if err != nil {
		return err
	}

	// Update existing primary if location changed
	if existing.District != district || existing.Subdistrict != subdistrict {
		return r.db.Model(&existing).Updates(map[string]interface{}{
			"district":    district,
			"subdistrict": subdistrict,
			"updated_at":  time.Now(),
		}).Error
	}

	return nil
}

// Create inserts a new (non-primary) subscription.
func (r *weatherSubscriptionRepository) Create(sub *domain.UserWeatherSubscription) error {
	return r.db.Create(sub).Error
}

// Delete soft-deactivates a subscription (sets is_active = FALSE) for a given user.
// A user cannot delete their own primary subscription.
func (r *weatherSubscriptionRepository) Delete(id uuid.UUID, userID uuid.UUID) error {
	result := r.db.Model(&domain.UserWeatherSubscription{}).
		Where("id = ? AND user_id = ? AND is_primary = FALSE", id, userID).
		Updates(map[string]interface{}{
			"is_active":  false,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("subscription not found or cannot delete primary subscription")
	}
	return nil
}

// GetAlertHistory returns recent weather alerts matching the given subdistricts.
// If subdistricts is empty, it returns the most recent alerts across Chonburi.
func (r *weatherSubscriptionRepository) GetAlertHistory(subdistricts []string, limit int) ([]domain.WeatherAlertHistory, error) {
	if limit <= 0 {
		limit = 30
	}
	var alerts []domain.WeatherAlertHistory
	query := r.db.Model(&domain.WeatherAlertHistory{}).Order("created_at DESC").Limit(limit)
	if len(subdistricts) > 0 {
		query = query.Where("subdistrict IN ?", subdistricts)
	}
	err := query.Find(&alerts).Error
	return alerts, err
}

