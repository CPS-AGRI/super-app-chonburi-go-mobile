package usecase

import (
	"super-app-chonburi-go-mobile/internal/domain"
)

type moduleUseCase struct {
	repo domain.ModuleRepository
}

func NewModuleUseCase(repo domain.ModuleRepository) domain.ModuleUseCase {
	return &moduleUseCase{repo: repo}
}

func (u *moduleUseCase) GetModuleTypesByModuleID(moduleID string) ([]domain.ModuleType, error) {
	return u.repo.GetModuleTypesByModuleID(moduleID)
}
