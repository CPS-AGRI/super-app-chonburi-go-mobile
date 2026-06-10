package domain

import (
	"time"

	"github.com/google/uuid"
)

type VerificationStatus string

const (
	VerificationStatusUnverified VerificationStatus = "unverified"
	VerificationStatusPending    VerificationStatus = "pending"
	VerificationStatusVerified   VerificationStatus = "verified"
	VerificationStatusRejected   VerificationStatus = "rejected"
)

type UserFCMToken struct {
	UserID      uuid.UUID `gorm:"type:uuid;primaryKey;column:user_id"`
	DeviceID    string    `gorm:"type:text;primaryKey;column:device_id"`
	Token       string    `gorm:"type:text;not null;column:token"`
	CreatedDate time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:created_date"`
	UpdatedDate time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:updated_date"`
}

func (UserFCMToken) TableName() string { return "user_fcm_tokens" }

type SubmitVerificationRequest struct {
	IdentityNumber string     `json:"identity_number" validate:"required"`
	LaserID        string     `json:"laser_id"        validate:"required"`
	IdCardType     int        `json:"id_card_type"    validate:"required,oneof=1 2"`
	IdCardExpiry   *time.Time `json:"id_card_expiry"`
	IdCardPhotoUrl string     `json:"id_card_photo_url" validate:"required"`
}

type RegisterFCMTokenRequest struct {
	DeviceID string `json:"device_id" validate:"required"`
	Token    string `json:"token"     validate:"required"`
}

type VerificationStatusResponse struct {
	VerificationStatus string     `json:"verification_status"`
	VerifiedDate       *time.Time `json:"verified_date,omitempty"`
	RejectionReason    *string    `json:"rejection_reason,omitempty"`
}

type MenuItemResponse struct {
	Key    string `json:"key"`
	NameTh string `json:"name_th"`
	NameEn string `json:"name_en"`
	Status string `json:"status"`
}

type MeResponse struct {
	UserID             uuid.UUID          `json:"user_id"`
	Name               string             `json:"name"`
	LastName           string             `json:"last_name"`
	Phone              string             `json:"phone"`
	Email              *string            `json:"email,omitempty"`
	ImageProfileUrl    *string            `json:"image_profile_url,omitempty"`
	VerificationStatus string             `json:"verification_status"`
	MenuItems          []MenuItemResponse `json:"menu_items"`
}

type VerificationRepository interface {
	GetUserWithInfo(userID uuid.UUID) (*AppUser, error)
	GetModulesForMenu() ([]Module, error)
	SubmitVerification(userID uuid.UUID, req *SubmitVerificationRequest) error
	GetVerificationStatus(userID uuid.UUID) (*VerificationStatusResponse, error)
	RegisterFCMToken(userID uuid.UUID, req *RegisterFCMTokenRequest) error
	GetFCMTokensByUserID(userID uuid.UUID) ([]string, error)
}

type VerificationUseCase interface {
	GetMe(userID uuid.UUID) (*MeResponse, error)
	SubmitVerification(userID uuid.UUID, req *SubmitVerificationRequest) error
	GetVerificationStatus(userID uuid.UUID) (*VerificationStatusResponse, error)
	RegisterFCMToken(userID uuid.UUID, req *RegisterFCMTokenRequest) error
}
