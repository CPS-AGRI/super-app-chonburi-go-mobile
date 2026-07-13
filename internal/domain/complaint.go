package domain

import (
	"time"
)

const (
	ComplaintStatusDraft      = "draft"
	ComplaintStatusPending    = "pending"
	ComplaintStatusReceived   = "received"
	ComplaintStatusInProgress = "in_progress"
	ComplaintStatusCompleted  = "completed"
	ComplaintStatusRejected   = "rejected"

	ActivityStatusUserRating     = "user_rating"
	ActivityStatusDisputeRequest = "dispute_request"
)

type ComplaintRating struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

type Complaint struct {
	ID           string     `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleTypeId string     `gorm:"type:uuid;index;not null;column:module_type_id" json:"module_type_id"`
	UserId       string     `gorm:"type:uuid;index;not null;column:user_id" json:"user_id"`
	DocumentId   string     `gorm:"not null;column:document_id;index" json:"document_id"`
	Description  string     `gorm:"type:text;column:description" json:"description"`
	Latitude     float64    `gorm:"column:latitude" json:"latitude"`
	Longitude    float64    `gorm:"column:longitude" json:"longitude"`
	Status       string     `gorm:"not null;column:status;index" json:"status"`
	CreatedDate  time.Time  `gorm:"not null;type:timestamptz;column:created_date;index" json:"created_at"`
	UpdatedDate  time.Time  `gorm:"not null;type:timestamptz;column:updated_date" json:"updated_at"`
	CreatedBy    string     `gorm:"not null;column:created_by" json:"created_by"`
	UpdatedBy    string     `gorm:"not null;column:updated_by" json:"updated_by"`
	DeletedAt    *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`
	DepartmentId *string    `gorm:"type:uuid;index;column:department_id" json:"department_id"`
	AssigneeId   *string    `gorm:"type:uuid;index;column:assignee_id" json:"assignee_id"`
	IsDisputed   bool       `gorm:"not null;default:false;column:is_disputed;index" json:"is_disputed"`

	Images     []ComplaintImage    `gorm:"foreignKey:ModuleComplaintId" json:"images,omitempty"`
	ModuleType *ModuleType         `gorm:"foreignKey:ModuleTypeId" json:"module_type,omitempty"`
	Activities []ComplaintActivity `gorm:"foreignKey:ModuleComplaintId" json:"activities,omitempty"`

	CurrentRating     *ComplaintRating `gorm:"-" json:"current_rating,omitempty"`
	HasRatedThisRound bool             `gorm:"-" json:"has_rated_this_round"`
}

func (Complaint) TableName() string { return "module_complaints" }

type ComplaintImage struct {
	ID                string    `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleComplaintId string    `gorm:"type:uuid;index;not null;column:module_complaint_id" json:"module_complaint_id"`
	Url               string    `gorm:"not null;column:url" json:"url"`
	Sequence          int       `gorm:"not null;column:sequence" json:"sequence"`
	CreatedDate       time.Time `gorm:"not null;type:timestamptz;column:created_date" json:"created_at"`
	UpdatedDate       time.Time `gorm:"not null;type:timestamptz;column:updated_date" json:"updated_at"`
	CreatedBy         string    `gorm:"not null;column:created_by" json:"created_by"`
	UpdatedBy         string    `gorm:"not null;column:updated_by" json:"updated_by"`
}

func (ComplaintImage) TableName() string { return "module_complaint_images" }

type ComplaintActivity struct {
	ID                string    `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleComplaintId string    `gorm:"type:uuid;index;not null;column:module_complaint_id" json:"module_complaint_id"`
	Description       string    `gorm:"type:text;column:description" json:"description"`
	Status            string    `gorm:"column:status;index" json:"status"`
	CreatedDate       time.Time `gorm:"not null;type:timestamptz;column:created_date" json:"created_at"`
	UpdatedDate       time.Time `gorm:"not null;type:timestamptz;column:updated_date" json:"updated_at"`
	CreatedBy         string    `gorm:"not null;column:created_by" json:"created_by"`
	UpdatedBy         string    `gorm:"not null;column:updated_by" json:"updated_by"`

	Images []ComplaintActivityImage `gorm:"foreignKey:ModuleComplaintActivityId" json:"images,omitempty"`
}

func (ComplaintActivity) TableName() string { return "module_complaint_activities" }

type ComplaintActivityImage struct {
	ID                        string    `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleComplaintActivityId string    `gorm:"type:uuid;index;not null;column:module_complaint_activity_id" json:"module_complaint_activity_id"`
	Url                       string    `gorm:"not null;column:url" json:"url"`
	Sequence                  int       `gorm:"not null;column:sequence" json:"sequence"`
	CreatedDate               time.Time `gorm:"not null;type:timestamptz;column:created_date" json:"created_at"`
	UpdatedDate               time.Time `gorm:"not null;type:timestamptz;column:updated_date" json:"updated_at"`
	CreatedBy                 string    `gorm:"not null;column:created_by" json:"created_by"`
	UpdatedBy                 string    `gorm:"not null;column:updated_by" json:"updated_by"`
}

func (ComplaintActivityImage) TableName() string { return "module_complaint_activity_images" }

type ComplaintRatingHistory struct {
	ID                string    `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleComplaintId string    `gorm:"type:uuid;index;not null;column:module_complaint_id" json:"module_complaint_id"`
	AssigneeId        *string   `gorm:"type:uuid;index;column:assignee_id" json:"assignee_id"`
	DepartmentId      *string   `gorm:"type:uuid;index;column:department_id" json:"department_id"`
	RatingScore       *int      `gorm:"column:rating_score;index" json:"rating_score"`
	IsDisputed        bool      `gorm:"not null;default:false;column:is_disputed;index" json:"is_disputed"`
	CreatedDate       time.Time `gorm:"not null;type:timestamptz;column:created_date;index" json:"created_at"`
	UpdatedDate       time.Time `gorm:"not null;type:timestamptz;column:updated_date" json:"updated_at"`
	CreatedBy         string    `gorm:"not null;column:created_by" json:"created_by"`
	UpdatedBy         string    `gorm:"not null;column:updated_by" json:"updated_by"`
}

func (ComplaintRatingHistory) TableName() string { return "module_complaint_rating_histories" }

type ComplaintRepository interface {
	Create(complaint *Complaint) error
	GetByUserID(userID string, status string, search string, page int, limit int) ([]Complaint, error)
	GetByID(id string, userID string) (*Complaint, error)
	Update(complaint *Complaint) error
	UpdateStatus(id string, status string) error
	Delete(id string) error
	GetFirstUserID() (string, error)
	CreateActivity(activity *ComplaintActivity) error
	CreateRatingHistory(history *ComplaintRatingHistory) error
	GetCompleterInfo(complaintID string) (*string, *string, error)
	GetComplaintMode() (string, error)
}

type ComplaintUseCase interface {
	AddComplaint(complaint *Complaint, imageURLs []string) error
	UpdateComplaint(complaint *Complaint, imageURLs []string) error
	GetMyComplaints(userID string, status string, search string, page int, limit int) ([]Complaint, error)
	GetDetail(id string, userID string) (*Complaint, error)
	DeleteComplaint(id string, userID string) error
	GetFirstUserID() (string, error)
	RateComplaint(id string, userID string, rating int, comment string) error
	DisputeComplaint(id string, userID string, reason string, images []string) error
}
