package repository

import (
	"gorm.io/gorm"
	"super-app-chonburi-go-mobile/internal/domain"
)

type moduleRepository struct {
	db *gorm.DB
}

func NewModuleRepository(db *gorm.DB) domain.ModuleRepository {
	return &moduleRepository{db: db}
}

func (r *moduleRepository) GetModuleTypesByModuleID(moduleID string) ([]domain.ModuleType, error) {
	var moduleTypes []domain.ModuleType
	err := r.db.Where("module_id = ?", moduleID).Find(&moduleTypes).Error
	return moduleTypes, err
}
