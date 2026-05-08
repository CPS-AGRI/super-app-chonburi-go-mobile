package domain

import (
	"time"
)

// Complaint Statuses based on mockup
const (
	ComplaintStatusDraft      = "draft"       // แบบร่าง
	ComplaintStatusSubmitted  = "submitted"   // ส่งเรื่อง
	ComplaintStatusUpdated    = "updated"     // อัพเดท
	ComplaintStatusInProgress = "in_progress" // ดำเนินการ
	ComplaintStatusCompleted  = "completed"  // เสร็จสมบูรณ์
	ComplaintStatusCanceled   = "canceled"   // ยกเลิก
)

type Complaint struct {
	ID                string         `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleTypeId      string         `gorm:"type:uuid;index;not null;column:module_type_id" json:"module_type_id"`
	UserId            string         `gorm:"type:uuid;index;not null;column:user_id" json:"user_id"`
	DocumentId        string         `gorm:"not null;column:document_id;index" json:"document_id"`
	Title             string         `gorm:"type:text;column:title" json:"title"` // e.g. "ถนนเป็นหลุมบ่อ"
	Description       string         `gorm:"type:text;column:description" json:"description"`
	Latitude          float64        `gorm:"column:latitude" json:"latitude"`
	Longitude         float64        `gorm:"column:longitude" json:"longitude"`
	Status            string         `gorm:"not null;column:status;index" json:"status"`
	CreatedDate       time.Time      `gorm:"not null;type:timestamptz;column:created_date;index" json:"created_at"`
	UpdatedDate       time.Time      `gorm:"not null;type:timestamptz;column:updated_date" json:"updated_at"`
	CreatedBy         string         `gorm:"not null;column:created_by" json:"created_by"`
	UpdatedBy         string         `gorm:"not null;column:updated_by" json:"updated_by"`
	
	// Relations
	Images     []ComplaintImage    `gorm:"foreignKey:ModuleComplaintId" json:"images,omitempty"`
}

func (Complaint) TableName() string { return "module_complaints" }

type ComplaintImage struct {
	ID                string    `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleComplaintId string    `gorm:"type:uuid;index;not null;column:module_complaint_id" json:"module_complaint_id"`
	Url               string    `gorm:"not null;column:url" json:"url"`
	Sequence          int       `gorm:"not null;column:sequence" json:"sequence"`
	CreatedDate       time.Time `gorm:"not null;type:timestamptz;column:created_date" json:"created_at"`
}

func (ComplaintImage) TableName() string { return "module_complaint_images" }

// Repository & UseCase Interfaces
type ComplaintRepository interface {
	Create(complaint *Complaint) error
	GetByUserID(userID string, status string, search string) ([]Complaint, error)
	GetByID(id string, userID string) (*Complaint, error)
	UpdateStatus(id string, status string) error
}

type ComplaintUseCase interface {
	AddComplaint(complaint *Complaint, imageURLs []string) error
	GetMyComplaints(userID string, status string, search string) ([]Complaint, error)
	GetDetail(id string, userID string) (*Complaint, error)
	CancelComplaint(id string, userID string) error
}
