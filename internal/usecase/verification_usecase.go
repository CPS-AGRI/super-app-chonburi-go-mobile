package usecase

import (
	"errors"

	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/google/uuid"
)

type verificationUseCase struct {
	repo domain.VerificationRepository
}

func NewVerificationUseCase(repo domain.VerificationRepository) domain.VerificationUseCase {
	return &verificationUseCase{repo: repo}
}

func (u *verificationUseCase) GetMe(userID uuid.UUID) (*domain.MeResponse, error) {
	user, err := u.repo.GetUserWithInfo(userID)
	if err != nil {
		return nil, err
	}

	modules, err := u.repo.GetModulesForMenu()
	if err != nil {
		return nil, err
	}

	verificationStatus := "unverified"
	var name, lastName, phone string
	var email *string

	if user.Information != nil {
		verificationStatus = user.Information.VerificationStatus
		name = user.Information.Name
		lastName = user.Information.LastName
		phone = user.Information.Phone
		email = user.Information.Email
	} else {
		name = "User"
		phone = user.PhoneNumber
	}

	menuItems := []domain.MenuItemResponse{}
	for _, m := range modules {
		status := "active"
		if m.IsUsedForUserRegistrationOnly && verificationStatus != "verified" {
			status = "lock"
		}

		key := ""
		if m.Key != nil {
			key = *m.Key
		} else {
			key = m.ID
		}

		menuItems = append(menuItems, domain.MenuItemResponse{
			Key:    key,
			NameTh: m.NameTh,
			NameEn: m.NameEn,
			Status: status,
		})
	}

	return &domain.MeResponse{
		UserID:             user.ID,
		Name:               name,
		LastName:           lastName,
		Phone:              phone,
		Email:              email,
		ImageProfileUrl:    user.ImageProfileUrl,
		VerificationStatus: verificationStatus,
		MenuItems:          menuItems,
	}, nil
}

func (u *verificationUseCase) SubmitVerification(userID uuid.UUID, req *domain.SubmitVerificationRequest) error {
	user, err := u.repo.GetUserWithInfo(userID)
	if err != nil {
		return err
	}
	if user.Information == nil {
		return errors.New("user information profile not found")
	}

	return u.repo.SubmitVerification(userID, req)
}

func (u *verificationUseCase) GetVerificationStatus(userID uuid.UUID) (*domain.VerificationStatusResponse, error) {
	return u.repo.GetVerificationStatus(userID)
}

func (u *verificationUseCase) RegisterFCMToken(userID uuid.UUID, req *domain.RegisterFCMTokenRequest) error {
	return u.repo.RegisterFCMToken(userID, req)
}
