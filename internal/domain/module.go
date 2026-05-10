package domain

import "time"

type Module struct {
	ID                            string    `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	NameTh                        string    `gorm:"not null;default:'';column:name_th" json:"name_th"`
	NameEn                        string    `gorm:"not null;default:'';column:name_en" json:"name_en"`
	CreatedDate                   time.Time `gorm:"not null;type:timestamptz;column:created_date" json:"created_date"`
	UpdatedDate                   time.Time `gorm:"not null;type:timestamptz;column:updated_date" json:"updated_date"`
	IsUsedForUserRegistrationOnly bool      `gorm:"not null;default:false;column:is_used_for_user_registration_only" json:"is_used_for_user_registration_only"`
}

func (Module) TableName() string { return "modules" }

type ModuleType struct {
	ID          string    `gorm:"type:uuid;primaryKey;column:id" json:"id"`
	ModuleId    string    `gorm:"type:uuid;not null;column:module_id" json:"module_id"`
	NameTh      string    `gorm:"not null;default:'';column:name_th" json:"name_th"`
	NameEn      string    `gorm:"not null;default:'';column:name_en" json:"name_en"`
	CreatedDate time.Time `gorm:"not null;type:timestamptz;column:created_date" json:"created_date"`
	UpdatedDate time.Time `gorm:"not null;type:timestamptz;column:updated_date" json:"updated_date"`
}

func (ModuleType) TableName() string { return "module_types" }

type ModuleRepository interface {
	GetModuleTypesByModuleID(moduleID string) ([]ModuleType, error)
}

type ModuleUseCase interface {
	GetModuleTypesByModuleID(moduleID string) ([]ModuleType, error)
}
