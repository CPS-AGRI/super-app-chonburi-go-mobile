package usecase

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
	// หาก Subdistrict เป็นข้อความรวมที่อยู่เต็มมาจาก DOPA (มี "ต.", "อ.", "จ.", "แขวง", "เขต")
	if strings.Contains(info.Subdistrict, "ต.") || strings.Contains(info.Subdistrict, "อ.") || strings.Contains(info.Subdistrict, "แขวง") || strings.Contains(info.Subdistrict, "เขต") {
		return info.Subdistrict
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
		if !strings.Contains(addr, "จ.") && !strings.Contains(addr, "จังหวัด") {
			if isBkk {
				addr += " " + info.Province
			} else {
				addr += " จ." + info.Province
			}
		}
	}
	if info.PostalCode > 0 {
		addr += fmt.Sprintf(" %d", info.PostalCode)
	}
	return strings.TrimSpace(addr)
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

	var registeredAddress string
	for _, acc := range user.OauthAccounts {
		if strings.ToLower(acc.Provider) == "thaiid" && acc.RawData != "" {
			var profileMap map[string]interface{}
			if err := json.Unmarshal([]byte(acc.RawData), &profileMap); err == nil {
				hNo, vNo, al, rd, sub, dist, prov, pCode := parseThaiIDAddress(profileMap)
				if hNo != "" || sub != "" || dist != "" || prov != "" {
					dopaInfo := domain.UserInformation{
						HouseNumber:   hNo,
						VillageNumber: vNo,
						Alley:         al,
						Road:          rd,
						Subdistrict:   sub,
						District:      dist,
						Province:      prov,
						PostalCode:    pCode,
					}
					registeredAddress = formatThaiAddress(&dopaInfo)
				}
			}
			break
		}
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
		Information:        user.Information,
		RegisteredAddress:  registeredAddress,
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

func (u *verificationUseCase) UpdateAddress(userID uuid.UUID, req *domain.UpdateAddressRequest) error {
	return u.repo.UpdateAddress(userID, req)
}

func (u *verificationUseCase) RegisterFCMToken(userID uuid.UUID, req *domain.RegisterFCMTokenRequest) error {
	return u.repo.RegisterFCMToken(userID, req)
}
