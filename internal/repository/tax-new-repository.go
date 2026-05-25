package repository

import (
	"errors"

	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type taxNewMobileRepository struct {
	db *gorm.DB
}

// NewTaxNewMobileRepository creates a new mobile repository for the self-declaration tax module.
func NewTaxNewMobileRepository(db *gorm.DB) domain.TaxNewMobileRepository {
	return &taxNewMobileRepository{db: db}
}

func (r *taxNewMobileRepository) GetBusinessByRegNumber(regNumber string) (*domain.TaxBusiness, error) {
	var business domain.TaxBusiness
	err := r.db.Where("business_reg_number = ?", regNumber).First(&business).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &business, nil
}

func (r *taxNewMobileRepository) GetActiveTaxRate(taxType string) (*domain.TaxRate, error) {
	var rate domain.TaxRate
	err := r.db.Where("tax_type = ? AND is_active = ?", taxType, true).First(&rate).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rate, nil
}

func (r *taxNewMobileRepository) GetLatestDeclarationVersion(regNumber string, taxType string, month, year int) (int, error) {
	var maxVersion int
	err := r.db.Model(&domain.TaxDeclaration{}).
		Where("business_reg_number = ? AND tax_type = ? AND tax_month = ? AND tax_year = ?", regNumber, taxType, month, year).
		Select("COALESCE(MAX(declaration_version), 0)").
		Row().Scan(&maxVersion)
	return maxVersion, err
}

func (r *taxNewMobileRepository) CreateDeclaration(declaration *domain.TaxDeclaration) error {
	return r.db.Create(declaration).Error
}

func (r *taxNewMobileRepository) GetDeclarationByID(id uuid.UUID) (*domain.TaxDeclaration, error) {
	var declaration domain.TaxDeclaration
	err := r.db.Preload("Business").Where("id = ?", id).First(&declaration).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &declaration, nil
}
