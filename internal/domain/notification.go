package domain

import (
	"time"

	"github.com/google/uuid"
)

type ModuleNotification struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;column:id;default:uuid_generate_v4()" json:"id"`
	ModuleID        uuid.UUID  `gorm:"type:uuid;not null;column:module_id" json:"module_id"`
	ModuleTypeID    *uuid.UUID `gorm:"type:uuid;column:module_type_id" json:"module_type_id"`
	UserID          *uuid.UUID `gorm:"type:uuid;column:user_id;index:idx_module_notif_user_read_created,priority:1" json:"user_id,omitempty"`
	UserAdminID     *uuid.UUID `gorm:"type:uuid;column:user_admin_id" json:"user_admin_id,omitempty"`
	DepartmentID    *uuid.UUID `gorm:"type:uuid;column:department_id" json:"department_id,omitempty"`
	Role            *string    `gorm:"type:text;column:role" json:"role,omitempty"`
	DocumentID      *string    `gorm:"type:text;column:document_id" json:"document_id,omitempty"`
	ReferenceID     string     `gorm:"type:text;not null;column:reference_id" json:"reference_id"`
	ReferenceTitle  string     `gorm:"type:text;not null;column:reference_title" json:"reference_title"`
	ReferenceBody   string     `gorm:"type:text;not null;column:reference_body" json:"reference_body"`
	ReferenceStatus string     `gorm:"type:text;not null;column:reference_status" json:"reference_status"`
	SendDate        *time.Time `gorm:"type:timestamptz;column:send_date" json:"send_date"`
	Type            string     `gorm:"type:text;not null;column:type" json:"type"`
	Status          string     `gorm:"type:text;not null;column:status" json:"status"`
	State           string     `gorm:"type:text;not null;column:state" json:"state"`
	IsRead          bool       `gorm:"type:boolean;not null;default:false;column:is_read;index:idx_module_notif_user_read_created,priority:2" json:"is_read"`
	CreatedBy       string     `gorm:"type:text;not null;default:'';column:created_by" json:"created_by"`
	CreatedDate     time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:created_date;index:idx_module_notif_user_read_created,priority:3" json:"created_date"`
	UpdatedBy       string     `gorm:"type:text;not null;default:'';column:updated_by" json:"updated_by"`
	UpdatedDate     time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:updated_date" json:"updated_date"`
}

func (ModuleNotification) TableName() string {
	return "module_notifications"
}

type ModuleUserNotification struct {
	ModuleNotificationID uuid.UUID `gorm:"type:uuid;primaryKey;column:module_notification_id" json:"module_notification_id"`
	UserID               uuid.UUID `gorm:"type:uuid;primaryKey;column:user_id" json:"user_id"`
	CreatedDate          time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:created_date" json:"created_date"`
}

func (ModuleUserNotification) TableName() string {
	return "module_user_notifications"
}



type NotificationResponseItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Date        string `json:"date"`
	UpdatedDate string `json:"updated_date"`
	Type        string `json:"type"`
	Read        bool   `json:"read"`
	ReferenceID string `json:"reference_id,omitempty"`
}

type NotificationRepository interface {
	GetNotifications(userID uuid.UUID) ([]NotificationResponseItem, error)
	MarkAsRead(userID uuid.UUID, notificationID uuid.UUID) error
	MarkAllAsRead(userID uuid.UUID) error
}

type NotificationUseCase interface {
	GetNotifications(userID string) ([]NotificationResponseItem, error)
	MarkAsRead(userID string, notificationID string) error
	MarkAllAsRead(userID string) error
}
