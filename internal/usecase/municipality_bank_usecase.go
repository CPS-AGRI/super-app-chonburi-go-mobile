package usecase

import "super-app-chonburi-go-mobile/internal/domain"

type municipalityBankUseCase struct {
	repo domain.MunicipalityBankRepository
}

func NewMunicipalityBankUseCase(repo domain.MunicipalityBankRepository) domain.MunicipalityBankUseCase {
	return &municipalityBankUseCase{repo: repo}
}

func (u *municipalityBankUseCase) GetActiveBank() (*domain.MunicipalityBank, error) {
	return u.repo.GetActive()
}
