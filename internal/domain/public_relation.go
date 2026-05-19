package domain

import (
	"time"

	"github.com/google/uuid"
)

// module_public_relations
type PublicRelation struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleId      uuid.UUID `gorm:"type:uuid;index;not null;column:module_id" json:"module_id"`
	AdminUserId   uuid.UUID `gorm:"type:uuid;index;not null;column:admin_user_id" json:"admin_user_id"`
	Title         string    `gorm:"type:text;not null;column:title" json:"title"`
	DescriptionTh *string   `gorm:"type:text;column:description_th" json:"description_th"`
	DescriptionEn *string   `gorm:"type:text;column:description_en" json:"description_en"`
	Type          string    `gorm:"type:text;not null;column:type" json:"type"`
	Priority      string    `gorm:"type:text;not null;column:priority" json:"priority"`
	StartDate     time.Time `gorm:"type:timestamptz;not null;column:start_date" json:"start_date"`
	EndDate       time.Time `gorm:"type:timestamptz;not null;column:end_date" json:"end_date"`
	Status        string    `gorm:"type:text;not null;column:status" json:"status"`
	CreatedDate   time.Time `gorm:"type:timestamptz;not null;column:created_date" json:"created_date"`
	UpdatedDate   time.Time `gorm:"type:timestamptz;not null;column:updated_date" json:"updated_date"`
	CreatedBy     string    `gorm:"type:text;not null;column:created_by" json:"created_by"`
	UpdatedBy     string    `gorm:"type:text;not null;column:updated_by" json:"updated_by"`

	// Transient fields for Mobile
	LikesCount    int64 `gorm:"-" json:"likes_count"`
	CommentsCount int64 `gorm:"-" json:"comments_count"`
	ViewCount     int   `gorm:"-" json:"view_count"`
	Liked         bool  `gorm:"-" json:"liked"`

	// Relations
	Images       []PublicRelationImage       `gorm:"foreignKey:ModulePublicRelationId" json:"images,omitempty"`
	Likes        []PublicRelationLike        `gorm:"foreignKey:ModulePublicRelationId" json:"likes,omitempty"`
	Comments     []PublicRelationComment     `gorm:"foreignKey:ModulePublicRelationId" json:"comments,omitempty"`
	VisitorCount *PublicRelationVisitorCount `gorm:"foreignKey:ModulePublicRelationId" json:"visitor_count,omitempty"`
}

func (PublicRelation) TableName() string { return "module_public_relations" }

// module_public_relation_visitor_count
type PublicRelationVisitorCount struct {
	ModulePublicRelationId uuid.UUID `gorm:"type:uuid;primaryKey;column:module_public_relation_id" json:"module_public_relation_id"`
	Count                  int       `gorm:"type:int4;not null;column:count" json:"count"`
	CreatedDate            time.Time `gorm:"type:timestamptz;not null;column:created_date" json:"created_date"`
	UpdatedDate            time.Time `gorm:"type:timestamptz;not null;column:updated_date" json:"updated_date"`
	CreatedBy              string    `gorm:"type:text;not null;column:created_by" json:"created_by"`
	UpdatedBy              string    `gorm:"type:text;not null;column:updated_by" json:"updated_by"`
}

func (PublicRelationVisitorCount) TableName() string { return "module_public_relation_visitor_count" }

// module_public_relation_notifications
type PublicRelationNotification struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleId      uuid.UUID  `gorm:"type:uuid;index;not null;column:module_id" json:"module_id"`
	AdminUserId   uuid.UUID  `gorm:"type:uuid;index;not null;column:admin_user_id" json:"admin_user_id"`
	Title         string     `gorm:"type:text;not null;column:title" json:"title"`
	Description   *string    `gorm:"type:text;column:description" json:"description"`
	SendDate      *time.Time `gorm:"type:timestamptz;column:send_date" json:"send_date"`
	Type          string     `gorm:"type:text;not null;column:type" json:"type"`
	Status        string     `gorm:"type:text;not null;column:status" json:"status"`
	ProcessStatus string     `gorm:"type:text;not null;column:process_status" json:"process_status"`
	CreatedDate   time.Time  `gorm:"type:timestamptz;not null;column:created_date" json:"created_date"`
	UpdatedDate   time.Time  `gorm:"type:timestamptz;not null;column:updated_date" json:"updated_date"`
	CreatedBy     string     `gorm:"type:text;not null;column:created_by" json:"created_by"`
	UpdatedBy     string     `gorm:"type:text;not null;column:updated_by" json:"updated_by"`
}

func (PublicRelationNotification) TableName() string { return "module_public_relation_notifications" }

// module_public_relation_likes
type PublicRelationLike struct {
	ModulePublicRelationId uuid.UUID `gorm:"type:uuid;primaryKey;column:module_public_relation_id" json:"module_public_relation_id"`
	UserId                 uuid.UUID `gorm:"type:uuid;primaryKey;column:user_id" json:"user_id"`
	CreatedDate            time.Time `gorm:"type:timestamptz;not null;column:created_date" json:"created_date"`
	UpdatedDate            time.Time `gorm:"type:timestamptz;not null;column:updated_date" json:"updated_date"`
	CreatedBy              string    `gorm:"type:text;not null;column:created_by" json:"created_by"`
	UpdatedBy              string    `gorm:"type:text;not null;column:updated_by" json:"updated_by"`
}

func (PublicRelationLike) TableName() string { return "module_public_relation_likes" }

// module_public_relation_images
type PublicRelationImage struct {
	ID                     uuid.UUID `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModulePublicRelationId uuid.UUID `gorm:"type:uuid;index;not null;column:module_public_relation_id" json:"module_public_relation_id"`
	Url                    string    `gorm:"type:text;not null;column:url" json:"url"`
	Sequence               int       `gorm:"type:int4;not null;column:sequence" json:"sequence"`
	CreatedDate            time.Time `gorm:"type:timestamptz;not null;column:created_date" json:"created_date"`
	UpdatedDate            time.Time `gorm:"type:timestamptz;not null;column:updated_date" json:"updated_date"`
	CreatedBy              string    `gorm:"type:text;not null;column:created_by" json:"created_by"`
	UpdatedBy              string    `gorm:"type:text;not null;column:updated_by" json:"updated_by"`
}

func (PublicRelationImage) TableName() string { return "module_public_relation_images" }

// module_public_relation_comments
type PublicRelationComment struct {
	ID                     uuid.UUID `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModulePublicRelationId uuid.UUID `gorm:"type:uuid;index;not null;column:module_public_relation_id" json:"module_public_relation_id"`
	UserId                 uuid.UUID `gorm:"type:uuid;index;not null;column:user_id" json:"user_id"`
	Comment                string    `gorm:"type:text;not null;column:comment" json:"comment"`
	Status                 string    `gorm:"type:text;not null;default:'active';column:status" json:"status"`
	CreatedDate            time.Time `gorm:"type:timestamptz;not null;column:created_date" json:"created_date"`
	UpdatedDate            time.Time `gorm:"type:timestamptz;not null;column:updated_date" json:"updated_date"`
	CreatedBy              string    `gorm:"type:text;not null;column:created_by" json:"created_by"`
	UpdatedBy              string    `gorm:"type:text;not null;column:updated_by" json:"updated_by"`

	User        *AppUser         `gorm:"foreignKey:UserId;references:ID" json:"user,omitempty"`
	Information *UserInformation `gorm:"foreignKey:UserId;references:UserId" json:"user_information,omitempty"`
}

func (PublicRelationComment) TableName() string { return "module_public_relation_comments" }

// municipality_welcome_screens
type MunicipalityWelcomeScreen struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ImageUrl    string    `gorm:"type:text;not null;column:image_url" json:"image_url"`
	IsActive    bool      `gorm:"type:boolean;not null;default:false;column:is_active" json:"is_active"`
	Type        string    `gorm:"type:text;not null;column:type" json:"type"`
	CreatedDate time.Time `gorm:"type:timestamptz;not null;column:created_date" json:"created_date"`
	UpdatedDate time.Time `gorm:"type:timestamptz;not null;column:updated_date" json:"updated_date"`
	CreatedBy   string    `gorm:"type:text;not null;column:created_by" json:"created_by"`
	UpdatedBy   string    `gorm:"type:text;not null;column:updated_by" json:"updated_by"`
}

func (MunicipalityWelcomeScreen) TableName() string { return "municipality_welcome_screens" }

// Mobile Interfaces
type PublicRelationMobileRepository interface {
	GetPaginated(moduleId string, page int, limit int, userId string) ([]PublicRelation, error)
	GetByID(moduleId string, id string) (*PublicRelation, error)
	IncrementVisitorCount(prId string) error
	GetLike(prId string, userId string) (*PublicRelationLike, error)
	ToggleLike(prId string, userId string) (bool, error)
	CreateComment(comment *PublicRelationComment) error
	GetComments(prId string) ([]PublicRelationComment, error)
	DeleteComment(commentId string, userId string) error
	ReportComment(commentId string) error
	HideComment(commentId string) error
	GetActiveWelcomeScreen() (*MunicipalityWelcomeScreen, error)
}

type PublicRelationMobileUseCase interface {
	GetNewsFeed(moduleId string, page int, limit int, userId string) ([]PublicRelation, error)
	GetNewsDetail(moduleId string, id string, userId string) (*PublicRelation, bool, error)
	ToggleLike(prId string, userId string) (bool, error)
	AddComment(moduleId string, prId string, userId string, commentText string) (*PublicRelationComment, error)
	GetComments(prId string) ([]PublicRelationComment, error)
	DeleteComment(commentId string, userId string) error
	ReportComment(commentId string) error
	HideComment(commentId string) error
	GetWelcomeScreen() (*MunicipalityWelcomeScreen, error)
}
