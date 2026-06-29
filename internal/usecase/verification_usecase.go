package usecase

import (
	"errors"
	"fmt"

	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/google/uuid"
)

type verificationUseCase struct {
	repo domain.VerificationRepository
}

func NewVerificationUseCase(repo domain.VerificationRepository) domain.VerificationUseCase {
	return &verificationUseCase{repo: repo}
}

func formatThaiAddress(info *domain.UserInformation) string {
	if info == nil {
		return ""
	}
	var addr string
	if info.HouseNumber != "" {
		addr += info.HouseNumber
	}
	if info.BuildingName != "" {
		addr += " อาคาร " + info.BuildingName
	}
	if info.RoomNumber != "" {
		addr += " ห้อง " + info.RoomNumber
	}
	if info.VillageNumber != "" && info.VillageNumber != "-" {
		addr += " หมู่ " + info.VillageNumber
	}
	if info.Alley != "" && info.Alley != "-" {
		addr += " ซอย" + info.Alley
	}
	if info.Intersection != "" && info.Intersection != "-" {
		addr += " แยก " + info.Intersection
	}
	if info.Road != "" && info.Road != "-" {
		addr += " ถนน" + info.Road
	}
	
	isBkk := info.Province == "กรุงเทพมหานคร" || info.Province == "กรุงเทพฯ"
	
	if info.Subdistrict != "" {
		if isBkk {
			addr += " แขวง" + info.Subdistrict
		} else {
			addr += " ต." + info.Subdistrict
		}
	}
	if info.District != "" {
		if isBkk {
			addr += " เขต" + info.District
		} else {
			addr += " อ." + info.District
		}
	}
	if info.Province != "" {
		if isBkk {
			addr += " " + info.Province
		} else {
			addr += " จ." + info.Province
		}
	}
	if info.PostalCode > 0 {
		addr += fmt.Sprintf(" %d", info.PostalCode)
	}
	return addr
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
	var address string

	if user.Information != nil {
		verificationStatus = user.Information.VerificationStatus
		name = user.Information.Name
		lastName = user.Information.LastName
		phone = user.Information.Phone
		email = user.Information.Email
		if verificationStatus == "verified" {
			address = formatThaiAddress(user.Information)
		}
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
		Address:            address,
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
