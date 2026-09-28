package repository

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"super-app-chonburi-go-mobile/internal/domain"
)

type analyticsRepository struct {
	db *gorm.DB
}

func NewAnalyticsRepository(db *gorm.DB) domain.AnalyticsRepository {
	return &analyticsRepository{db: db}
}

func (r *analyticsRepository) RecordUsage(usageLog *domain.ModuleUsageLog) error {
	// 1. Resolve module UUID if not already provided
	if usageLog.ModuleID == nil && usageLog.ModuleCode != "" {
		var mod domain.Module
		codeLower := strings.ToLower(strings.TrimSpace(usageLog.ModuleCode))
		if err := r.db.Where("LOWER(key) = ? OR LOWER(name_en) = ?", codeLower, codeLower).First(&mod).Error; err == nil {
			if parsedUUID, parseErr := uuid.Parse(mod.ID); parseErr == nil {
				usageLog.ModuleID = &parsedUUID
			}
		}
	}

	// 2. Insert into immutable event log (ISO 27001 compliant)
	if err := r.db.Create(usageLog).Error; err != nil {
		return err
	}

	// 3. Increment daily summary counter (for fast dashboard queries) if module ID is resolved
	if usageLog.ModuleID != nil {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		rawQuery := `
			INSERT INTO user_activity_trackings (id, date, module_id, view_count, created_date, updated_date)
			VALUES (uuid_generate_v4(), ?, ?, 1, NOW(), NOW())
			ON CONFLICT (date, module_id)
			DO UPDATE SET view_count = user_activity_trackings.view_count + 1, updated_date = NOW();
		`
		_ = r.db.Exec(rawQuery, today, *usageLog.ModuleID).Error
	}

	return nil
}
