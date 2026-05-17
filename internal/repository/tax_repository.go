package repository

import (
	"errors"
	"super-app-chonburi-go-mobile/internal/domain"

	"gorm.io/gorm"
)

type taxRepository struct {
	db *gorm.DB
}

func NewTaxRepository(db *gorm.DB) domain.TaxRepository {
	return &taxRepository{db: db}
}

func (r *taxRepository) GetInformationsByIdentityNumber(identityNumber string) ([]domain.ModuleOnlineTaxPaymentInformation, error) {
	var items []domain.ModuleOnlineTaxPaymentInformation
	err := r.db.Where("identity_number = ?", identityNumber).
		Order("created_date DESC").
		Find(&items).Error
	return items, err
}

func (r *taxRepository) GetInformationByID(id string) (*domain.ModuleOnlineTaxPaymentInformation, error) {
	var item domain.ModuleOnlineTaxPaymentInformation
	err := r.db.Where("id = ?", id).First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *taxRepository) UpdateInformation(info *domain.ModuleOnlineTaxPaymentInformation) error {
	return r.db.Save(info).Error
}

// Admin-only methods (stubs or implemented if shared logic exists)
func (r *taxRepository) CreateImport(importHead *domain.ModuleOnlineTaxPayment) error { return nil }
func (r *taxRepository) CreateInformation(info *domain.ModuleOnlineTaxPaymentInformation) error { return nil }
func (r *taxRepository) GetImportByYearAndName(year, name string) (*domain.ModuleOnlineTaxPayment, error) { return nil, nil }
func (r *taxRepository) GetInformationByRefs(ref1, ref2 string) (*domain.ModuleOnlineTaxPaymentInformation, error) { return nil, nil }
func (r *taxRepository) GetInformationsPaginated(query domain.TaxQuery) (*domain.PaginatedTaxResponse, error) { return nil, nil }
func (r *taxRepository) GetImportsPaginated(query domain.TaxQuery) ([]domain.ModuleOnlineTaxPayment, int64, error) { return nil, 0, nil }
func (r *taxRepository) CreateLog(log *domain.ModuleOnlineTaxPaymentLog) error { return nil }
