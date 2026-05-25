package domain

import (
	"time"

	"github.com/google/uuid"
)

type AppUserStatus string

const (
	AppUserStatusPending  AppUserStatus = "pending"
	AppUserStatusActive   AppUserStatus = "active"
	AppUserStatusInactive AppUserStatus = "inactive"
)

// AppUser (Table: users) - Core Authentication Data
type AppUser struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4();column:id" json:"id"`
	PhoneNumber     string     `gorm:"type:text;not null;column:phone_number" json:"phone_number"`
	PhoneNumberHash string     `gorm:"type:text;index;column:phone_number_hash" json:"-"` // For fast searching
	PinHash         string     `gorm:"type:text;not null;column:pin_hash" json:"-"`
	IsConsent       bool       `gorm:"not null;default:false;column:is_consent" json:"is_consent"`
	ImageProfileUrl *string    `gorm:"type:text;column:image_profile_url" json:"image_profile_url"`
	Provider        *string    `gorm:"type:text;column:provider" json:"provider"`
	ProviderId      *string    `gorm:"type:text;column:provider_id" json:"provider_id"`
	Email           *string    `gorm:"type:text;column:email" json:"email"`
	EmailHash       *string    `gorm:"type:text;index;column:email_hash" json:"-"` // For fast searching
	
	CreatedBy       string     `gorm:"type:text;not null;default:'';column:created_by" json:"created_by"`
	CreatedDate     time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:created_date" json:"created_date"`
	UpdatedBy       string     `gorm:"type:text;not null;default:'';column:updated_by" json:"updated_by"`
	UpdatedDate     time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:updated_date" json:"updated_date"`
	
	// Relation to Profile with Cascade Delete
	Information     *UserInformation `gorm:"foreignKey:UserId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"information,omitempty"`
}

func (AppUser) TableName() string {
	return "users"
}

// UserInformation (Table: user_informations) - Citizen Profile Data
type UserInformation struct {
	UserId                    uuid.UUID  `gorm:"type:uuid;primaryKey;column:user_id" json:"user_id"`
	Prefix                    string     `gorm:"type:text;not null;default:'';column:prefix" json:"prefix"`
	Name                      string     `gorm:"type:text;not null;column:name" json:"name"`
	LastName                  string     `gorm:"type:text;not null;column:last_name" json:"last_name"`
	Phone                     string     `gorm:"type:text;not null;column:phone" json:"phone"`
	Email                     *string    `gorm:"type:text;column:email" json:"email"`
	Birthday                  *time.Time `gorm:"type:timestamptz;column:birthday" json:"birthday"`
	
	// Sensitive Data (Encrypted & Reversible)
	IdentityNumberEncrypted   string     `gorm:"type:text;column:identity_number_encrypted" json:"-"`
	IdentityNumberHash        string     `gorm:"type:text;index;column:identity_number_hash" json:"-"` // Blind Index for searching
	
	LaserIdEncrypted          string     `gorm:"type:text;column:laser_id_encrypted" json:"-"`
	LaserIdHash               string     `gorm:"type:text;index;column:laser_id_hash" json:"-"` // Blind Index for searching
	
	IdCardType                *int       `gorm:"type:int4;column:id_card_type" json:"id_card_type"`
	IdCardPhotoUrl            *string    `gorm:"type:text;column:id_card_photo_url" json:"id_card_photo_url"`
	IdCardExpiry              *time.Time `gorm:"type:date;column:id_card_expiry" json:"id_card_expiry"`
	
	// Verification Status
	Status                    string     `gorm:"type:text;not null;default:'active';column:status" json:"status"`
	VerificationStatus        string     `gorm:"type:text;not null;default:'unverified';column:verification_status" json:"verification_status"`
	VerifiedDate              *time.Time `gorm:"type:timestamptz;column:verified_date" json:"verified_date"`
	RejectionReason           *string    `gorm:"type:text;column:rejection_reason" json:"rejection_reason"`
	
	// Address Data
	HouseNumber               string     `gorm:"type:text;not null;default:'';column:house_number" json:"house_number"`
	VillageNumber             string     `gorm:"type:text;not null;default:'';column:village_number" json:"village_number"`
	Alley                     string     `gorm:"type:text;not null;default:'';column:alley" json:"alley"`
	Intersection              string     `gorm:"type:text;not null;default:'';column:intersection" json:"intersection"`
	Road                      string     `gorm:"type:text;not null;default:'';column:road" json:"road"`
	Subdistrict               string     `gorm:"type:text;not null;default:'';column:subdistrict" json:"subdistrict"`
	District                  string     `gorm:"type:text;not null;default:'';column:district" json:"district"`
	Province                  string     `gorm:"type:text;not null;default:'';column:province" json:"province"`
	PostalCode                int        `gorm:"type:int4;not null;default:0;column:postal_code" json:"postal_code"`
	BuildingName              string     `gorm:"type:text;not null;default:'';column:building_name" json:"building_name"`
	RoomNumber                string     `gorm:"type:text;not null;default:'';column:room_number" json:"room_number"`
	
	// Consent & Status Flags
	IsConsent                 bool       `gorm:"not null;default:false;column:is_consent" json:"is_consent"`
	IsWasteFeeReceipt         bool       `gorm:"not null;default:false;column:is_waste_fee_receipt" json:"is_waste_fee_receipt"`
	IsOnlineTaxPaymentFile    bool       `gorm:"not null;default:false;column:is_online_tax_payment_file" json:"is_online_tax_payment_file"`
	IsOnlineTaxPaymentReceipt bool       `gorm:"not null;default:false;column:is_online_tax_payment_receipt" json:"is_online_tax_payment_receipt"`
	LogoUrl                   string     `gorm:"type:text;not null;default:'';column:logo_url" json:"logo_url"`

	CreatedBy                 string     `gorm:"type:text;not null;default:'';column:created_by" json:"created_by"`
	CreatedDate               time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:created_date" json:"created_date"`
	UpdatedBy                 string     `gorm:"type:text;not null;default:'';column:updated_by" json:"updated_by"`
	UpdatedDate               time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:updated_date" json:"updated_date"`
}

func (UserInformation) TableName() string {
	return "user_informations"
}

type AuthRepository interface {
	GetByProviderID(provider, providerID string) (*AppUser, error)
	GetByEmail(email string) (*AppUser, error)
	GetByPhoneNumber(phoneNumber string) (*AppUser, error)
	Create(user *AppUser) error
	Update(user *AppUser) error
}

type AuthUseCase interface {
	LoginWithGoogle(idToken string) (*AuthResponse, error)
	LoginWithFacebook(accessToken string) (*AuthResponse, error)
	RefreshToken(refreshToken string) (*AuthResponse, error)
}

type AuthResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	User         *AppUser `json:"user"`
}
