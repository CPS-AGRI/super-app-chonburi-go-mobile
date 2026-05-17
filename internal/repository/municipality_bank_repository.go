package repository

import (
	"super-app-chonburi-go-mobile/internal/domain"

	"gorm.io/gorm"
)

type municipalityBankRepository struct {
	db *gorm.DB
}

func NewMunicipalityBankRepository(db *gorm.DB) domain.MunicipalityBankRepository {
	return &municipalityBankRepository{db: db}
}

func (r *municipalityBankRepository) GetActive() (*domain.MunicipalityBank, error) {
	var bank domain.MunicipalityBank
	err := r.db.Where("bank_status = 'active' AND status = 'active'").First(&bank).Error
	if err != nil {
		return nil, err
	}
	return &bank, nil
}
