package domain

import (
	"time"

	"github.com/google/uuid"
)

type ModuleUsageLog struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4();column:id" json:"id"`
	UserID     *uuid.UUID `gorm:"type:uuid;index:idx_module_usage_logs_user_id;column:user_id" json:"userId,omitempty"`
	ModuleCode string     `gorm:"type:varchar(50);not null;index:idx_module_usage_logs_module;column:module_code" json:"moduleCode"`
	ModuleID   *uuid.UUID `gorm:"type:uuid;column:module_id" json:"moduleId,omitempty"`
	Action     string     `gorm:"type:varchar(50);not null;default:'VIEW';column:action" json:"action"`
	Platform   string     `gorm:"type:varchar(20);not null;column:platform" json:"platform"`
	DeviceID   string     `gorm:"type:varchar(100);index:idx_module_usage_logs_device;column:device_id" json:"deviceId,omitempty"`
	IPAddress  string     `gorm:"type:varchar(100);not null;default:'';column:ip_address" json:"ipAddress"`
	UserAgent  string     `gorm:"type:text;not null;default:'';column:user_agent" json:"userAgent"`
	CreatedAt  time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;index:idx_module_usage_logs_created_at;column:created_at" json:"createdAt"`
}

func (ModuleUsageLog) TableName() string {
	return "module_usage_logs"
}

type RecordModuleUsageDTO struct {
	ModuleCode string `json:"moduleId"` // Accepts "complaint", "cctv", "news", etc., or UUID
	Action     string `json:"action"`
	Platform   string `json:"platform"`
	DeviceID   string `json:"deviceId"`
	Timestamp  string `json:"timestamp"`
}

type AnalyticsRepository interface {
	RecordUsage(log *ModuleUsageLog) error
}

type AnalyticsUseCase interface {
	TrackModuleUsage(req *RecordModuleUsageDTO, userID *uuid.UUID, ip, userAgent string) error
}
