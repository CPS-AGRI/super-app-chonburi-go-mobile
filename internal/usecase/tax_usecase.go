package usecase

import (
	"fmt"
	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/google/uuid"
)

type taxUseCase struct {
	taxRepo domain.TaxRepository
}

func NewTaxUseCase(taxRepo domain.TaxRepository) domain.TaxUseCase {
	return &taxUseCase{taxRepo: taxRepo}
}

func (u *taxUseCase) GetMyTaxes(identityNumber string) ([]domain.ModuleOnlineTaxPaymentInformation, error) {
	// Mobile version: we simply fetch by identity number
	return u.taxRepo.GetInformationsByIdentityNumber(identityNumber)
}

func (u *taxUseCase) UpdateTaxStatus(id uuid.UUID, status string, userID uuid.UUID, fileUrl *string) error {
	existing, err := u.taxRepo.GetInformationByID(id.String())
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("tax record not found")
	}

	// Simple validation: user can only update their own record
	if existing.UserId != nil && *existing.UserId != userID {
		return fmt.Errorf("unauthorized")
	}

	existing.Status = status
	if fileUrl != nil {
		existing.FileUrl = fileUrl // In mobile, this is usually the proof of payment
	}

	return u.taxRepo.UpdateInformation(existing)
}

// Stubs for admin methods not used in mobile
func (u *taxUseCase) ImportTaxRecords(importData *domain.ModuleOnlineTaxPayment, records []domain.ModuleOnlineTaxPaymentInformation, adminID uuid.UUID) (int, int, []string) {
	return 0, 0, nil
}

func (u *taxUseCase) LinkUser(infoID uuid.UUID, userID uuid.UUID, adminID uuid.UUID) error {
	return nil
}
