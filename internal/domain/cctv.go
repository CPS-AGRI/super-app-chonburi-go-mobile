package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CCTV represents a CCTV camera device.
type CCTV struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4();column:id" json:"id"`
	Name        string         `gorm:"type:varchar(100);not null;column:name;index:idx_cctv_name" json:"name"`
	Address     string         `gorm:"type:text;column:address" json:"address"`
	Latitude    float64        `gorm:"type:double precision;not null;column:latitude;index:idx_cctv_lat_lng" json:"latitude"`
	Longitude   float64        `gorm:"type:double precision;not null;column:longitude;index:idx_cctv_lat_lng" json:"longitude"`
	StreamURL   string         `gorm:"type:text;column:stream_url" json:"stream_url"`
	Status      string         `gorm:"type:varchar(20);not null;default:'ONLINE';column:status;index:idx_cctv_status" json:"status"` // 'ONLINE', 'OFFLINE'
	AccessLevel string         `gorm:"type:varchar(20);not null;default:'PUBLIC';column:access_level;index:idx_cctv_access" json:"access_level"` // 'PUBLIC', 'STAFF_ONLY'
	SnapshotURL string         `gorm:"type:text;column:snapshot_url" json:"snapshot_url"`
	CreatedBy   *string        `gorm:"type:varchar(50);column:created_by" json:"created_by,omitempty"`
	DeletedBy   *string        `gorm:"type:varchar(50);column:deleted_by" json:"deleted_by,omitempty"`
	CreatedAt   time.Time      `gorm:"type:timestamptz;not null;default:now();column:created_at" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"type:timestamptz;not null;default:now();column:updated_at" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index;column:deleted_at" json:"-"`
}

// TableName sets the table name for GORM.
func (CCTV) TableName() string {
	return "module_cctv"
}

// CCTVRequest represents a user request to view or get footage from a CCTV camera.
type CCTVRequest struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4();column:id" json:"id"`
	UserID          uuid.UUID      `gorm:"type:uuid;not null;column:user_id;index:idx_cctv_req_user" json:"user_id"`
	CCTVID          uuid.UUID      `gorm:"type:uuid;not null;column:cctv_id;index:idx_cctv_req_cctv" json:"cctv_id"`
	IncidentDate    time.Time      `gorm:"type:date;not null;column:incident_date" json:"incident_date"`
	StartTime       string         `gorm:"type:varchar(10);not null;column:start_time" json:"start_time"`
	EndTime         string         `gorm:"type:varchar(10);not null;column:end_time" json:"end_time"`
	Reason          string         `gorm:"type:text;not null;column:reason" json:"reason"`
	EvidenceFileURL string         `gorm:"type:text;not null;column:evidence_file_url" json:"evidence_file_url"`
	Status          string         `gorm:"type:varchar(20);not null;default:'PENDING';column:status;index:idx_cctv_req_status" json:"status"` // 'PENDING', 'PROCESSING', 'APPROVED', 'REJECTED'
	ApprovedByID    *uuid.UUID     `gorm:"type:uuid;column:approved_by_id" json:"approved_by_id"`
	ResponseFileURL *string        `gorm:"type:text;column:response_file_url" json:"response_file_url"`
	RejectReason    *string        `gorm:"type:text;column:reject_reason" json:"reject_reason"`
	CreatedAt       time.Time      `gorm:"type:timestamptz;not null;default:now();column:created_at" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"type:timestamptz;not null;default:now();column:updated_at" json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index;column:deleted_at" json:"-"`

	// Relations
	CCTV CCTV     `gorm:"foreignKey:CCTVID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"cctv,omitempty"`
	User *AppUser `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
}

// TableName sets the table name for GORM.
func (CCTVRequest) TableName() string {
	return "module_cctv_requests"
}

// MobileCCTVQuery lists query filters for Bounding Box search.
type MobileCCTVQuery struct {
	LatMin float64 `json:"lat_min"`
	LatMax float64 `json:"lat_max"`
	LngMin float64 `json:"lng_min"`
	LngMax float64 `json:"lng_max"`
}

// MobileCCTVRepository defines the data layer interface for Mobile CCTV API.
type MobileCCTVRepository interface {
	GetCameras(query MobileCCTVQuery) ([]CCTV, error)
	CreateRequest(req *CCTVRequest) error
	GetRequestsByUserID(userID uuid.UUID) ([]CCTVRequest, error)
}

// MobileCCTVUseCase defines the business layer interface for Mobile CCTV API.
type MobileCCTVUseCase interface {
	GetCameras(query MobileCCTVQuery) ([]CCTV, error)
	CreateRequest(req *CCTVRequest) error
	GetMyRequests(userID uuid.UUID) ([]CCTVRequest, error)

	// Unified Ticket Handshake System (Channel 1 & Channel 2)
	CreateHandshakeTicket(userID uuid.UUID, input CreateHandshakeTicketInput) (*CreateHandshakeTicketResponse, error)
	VerifyHandshakeTicket(partnerHeaderKey string, input VerifyTicketInput) (*VerifyTicketResponse, error)
	CreateWebHandshakeTicket(userID uuid.UUID, callbackURL, partnerID string) (string, error)
	ValidateCallbackURL(callbackURL string) bool
}

// CCTVHandshakeTicket represents a single-use ephemeral ticket for partner handshake.
// Strictly adheres to PDPA Data Minimization: NO citizen ID or personal sensitive PII is stored.
type CCTVHandshakeTicket struct {
	TicketID        string    `json:"ticket_id"`
	UserReferenceID string    `json:"user_reference_id"` // Pseudonymous ID (e.g. UUID) - NEVER citizen ID
	DisplayName     string    `json:"display_name,omitempty"`
	AuthChannel     string    `json:"auth_channel"`      // "thaid"
	IsThaIDVerified bool      `json:"is_thaid_verified"` // true
	CCTVID          string    `json:"cctv_id,omitempty"`
	Action          string    `json:"action"`            // "view" | "download"
	StartTime       string    `json:"start_time,omitempty"`
	EndTime         string    `json:"end_time,omitempty"`
	IssuedAt        time.Time `json:"issued_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type CreateHandshakeTicketInput struct {
	CCTVID    string `json:"cctv_id"`
	Action    string `json:"action"` // "view" | "download"
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type CreateHandshakeTicketResponse struct {
	Ticket           string `json:"ticket"`
	RedirectURL      string `json:"redirect_url"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

type VerifyTicketInput struct {
	Ticket       string `json:"ticket"`
	ClientID     string `json:"client_id,omitempty"`     // Ph6 client_id
	ClientSecret string `json:"client_secret,omitempty"` // Ph6 client_secret
}

type TimeWindow struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type TicketPermissions struct {
	AllowedCCTVID string      `json:"allowed_cctv_id,omitempty"`
	CanViewLive   bool        `json:"can_view_live"`
	CanDownload   bool        `json:"can_download"`
	TimeWindow    *TimeWindow `json:"time_window,omitempty"`
}

type VerifyTicketResponse struct {
	Success         bool              `json:"success"`
	UserIdentity    string            `json:"user_identity"`               // Required by Ph6 (Unique ID of user)
	Name            string            `json:"name,omitempty"`              // Required by Ph6 (Display Name)
	UserReferenceID string            `json:"user_reference_id,omitempty"` // Pseudonymous ID
	UserStatus      string            `json:"user_status"`                 // "active"
	AuthChannel     string            `json:"auth_channel"`                // "thaid"
	IsThaIDVerified bool              `json:"is_thaid_verified"`           // true
	Permissions     TicketPermissions `json:"permissions,omitempty"`
	IssuedAt        string            `json:"issued_at,omitempty"`
	ExpiresAt       string            `json:"expires_at,omitempty"`
}

