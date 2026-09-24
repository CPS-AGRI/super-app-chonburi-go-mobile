package repository

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"super-app-chonburi-go-mobile/internal/domain"
)

type authRepository struct {
	db *gorm.DB
}

func NewAuthRepository(db *gorm.DB) domain.AuthRepository {
	return &authRepository{db: db}
}

func (r *authRepository) GetByID(id uuid.UUID) (*domain.AppUser, error) {
	var user domain.AppUser
	err := r.db.Preload("Information").Preload("OauthAccounts").First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *authRepository) GetByProviderID(provider, providerID string) (*domain.AppUser, error) {
	var oauthAcc domain.UserOauthAccount
	err := r.db.Where("provider = ? AND provider_id = ?", provider, providerID).First(&oauthAcc).Error
	if err != nil {
		return nil, err
	}

	var user domain.AppUser
	err = r.db.Preload("Information").Preload("OauthAccounts").First(&user, oauthAcc.UserId).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *authRepository) GetByEmail(email string) (*domain.AppUser, error) {
	if email == "" {
		return nil, gorm.ErrRecordNotFound
	}

	// 1. Search in users table
	var user domain.AppUser
	if err := r.db.Preload("Information").Preload("OauthAccounts").Where("email = ?", email).First(&user).Error; err == nil {
		return &user, nil
	}

	// 2. Search in user_informations table
	var info domain.UserInformation
	if err := r.db.Where("email = ?", email).First(&info).Error; err == nil {
		if err := r.db.Preload("Information").Preload("OauthAccounts").First(&user, info.UserId).Error; err == nil {
			return &user, nil
		}
	}

	// 3. Search in user_oauth_accounts table
	var oauthAcc domain.UserOauthAccount
	if err := r.db.Where("email = ?", email).First(&oauthAcc).Error; err == nil {
		if err := r.db.Preload("Information").Preload("OauthAccounts").First(&user, oauthAcc.UserId).Error; err == nil {
			return &user, nil
		}
	}

	return nil, gorm.ErrRecordNotFound
}

func (r *authRepository) GetByPhoneNumber(phoneNumber string) (*domain.AppUser, error) {
	if phoneNumber == "" {
		return nil, gorm.ErrRecordNotFound
	}
	clean := phoneNumber
	for len(clean) > 0 && (clean[0] == '+' || clean[0] == ' ') {
		clean = clean[1:]
	}

	var formats []string
	formats = append(formats, phoneNumber)

	if len(clean) == 10 && clean[0] == '0' {
		// e.g. 0812345678 -> +66812345678, 66812345678
		suffix := clean[1:]
		formats = append(formats, "+66"+suffix, "66"+suffix)
	} else if len(clean) == 11 && clean[:2] == "66" {
		// e.g. 66812345678 -> 0812345678, +66812345678
		suffix := clean[2:]
		formats = append(formats, "0"+suffix, "+66"+suffix)
	}

	var user domain.AppUser
	err := r.db.Preload("Information").Preload("OauthAccounts").Where("phone_number IN ?", formats).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *authRepository) Create(user *domain.AppUser) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Information").Create(user).Error; err != nil {
			return err
		}
		if user.Information != nil {
			user.Information.UserId = user.ID
			if err := tx.Create(user.Information).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *authRepository) Update(user *domain.AppUser) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(user).Error; err != nil {
			return err
		}
		if user.Information != nil {
			if err := tx.Save(user.Information).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *authRepository) CreateOauthAccount(oauth *domain.UserOauthAccount) error {
	return r.db.Create(oauth).Error
}

func (r *authRepository) UpdateOauthAccount(oauth *domain.UserOauthAccount) error {
	return r.db.Save(oauth).Error
}

func (r *authRepository) DeleteOauthAccount(id uuid.UUID) error {
	return r.db.Where("id = ?", id).Delete(&domain.UserOauthAccount{}).Error
}

func (r *authRepository) Delete(user *domain.AppUser) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", user.ID).Delete(&domain.UserInformation{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&domain.UserOauthAccount{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&domain.UserFCMToken{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(user).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *authRepository) GetSocialLinks(userID uuid.UUID) (*domain.SocialAccountsResponse, error) {
	var accounts []domain.UserOauthAccount
	err := r.db.Where("user_id = ?", userID).Find(&accounts).Error
	if err != nil {
		return nil, err
	}

	resp := &domain.SocialAccountsResponse{
		Google: domain.SocialAccountItem{
			Provider: "Google",
			IsLinked: false,
		},
		Facebook: domain.SocialAccountItem{
			Provider: "Facebook",
			IsLinked: false,
		},
		Line: domain.SocialAccountItem{
			Provider: "Line",
			IsLinked: false,
		},
		Apple: &domain.SocialAccountItem{
			Provider: "Apple",
			IsLinked: false,
		},
	}

	for _, acc := range accounts {
		formattedDate := acc.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
		item := domain.SocialAccountItem{
			ID:          acc.ID.String(),
			Provider:    acc.Provider,
			Email:       acc.Email,
			DisplayName: acc.DisplayName,
			AvatarURL:   acc.AvatarUrl,
			IsLinked:    true,
			CreatedAt:   &formattedDate,
		}

		switch strings.ToLower(acc.Provider) {
		case "google":
			item.Provider = "Google"
			resp.Google = item
		case "facebook":
			item.Provider = "Facebook"
			resp.Facebook = item
		case "line":
			item.Provider = "Line"
			resp.Line = item
		case "apple":
			item.Provider = "Apple"
			resp.Apple = &item
		}
	}

	return resp, nil
}

func (r *authRepository) UnlinkSocial(userID uuid.UUID, provider string) error {
	var user domain.AppUser
	if err := r.db.First(&user, userID).Error; err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Safety check: Don't allow unlink if this is the user's ONLY login method
	hasPhonePin := user.PhoneNumber != "" && user.PinHash != ""
	if !hasPhonePin {
		var otherCount int64
		_ = r.db.Model(&domain.UserOauthAccount{}).
			Where("user_id = ? AND LOWER(provider) != LOWER(?)", userID, provider).
			Count(&otherCount)

		if otherCount == 0 {
			return errors.New("ไม่สามารถยกเลิกการเชื่อมต่อได้ เนื่องจากเป็นช่องทางเข้าสู่ระบบเดียวของคุณ กรุณาตั้งค่าเบอร์โทรศัพท์และรหัส PIN ก่อน")
		}
	}

	res := r.db.Where("user_id = ? AND LOWER(provider) = LOWER(?)", userID, provider).Delete(&domain.UserOauthAccount{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("ไม่พบบัญชีที่เชื่อมต่อสำหรับบริการนี้")
	}

	return nil
}

func (r *authRepository) LinkSocialAccount(userID uuid.UUID, account *domain.UserOauthAccount) error {
	account.UserId = userID
	// Check if already linked to another user
	var existing domain.UserOauthAccount
	if err := r.db.Where("provider = ? AND provider_id = ?", account.Provider, account.ProviderId).First(&existing).Error; err == nil {
		if existing.UserId != userID {
			return fmt.Errorf("บัญชี %s นี้ถูกเชื่อมต่อกับผู้ใช้อื่นในระบบแล้ว", account.Provider)
		}
		// Update existing
		account.ID = existing.ID
		return r.db.Save(account).Error
	}

	return r.db.Create(account).Error
}

func (r *authRepository) UpdateProfileImage(userID uuid.UUID, imageURL string) error {
	if err := r.db.Model(&domain.AppUser{}).Where("id = ?", userID).Update("image_profile_url", imageURL).Error; err != nil {
		return err
	}
	_ = r.db.Model(&domain.UserInformation{}).Where("user_id = ?", userID).Update("logo_url", imageURL)
	return nil
}

func (r *authRepository) GetExistingSocialProfile(userID uuid.UUID, provider string) (*domain.UserOauthAccount, error) {
	var acc domain.UserOauthAccount
	if err := r.db.Where("user_id = ? AND LOWER(provider) = LOWER(?)", userID, provider).First(&acc).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (r *authRepository) BindOverrideSocialAccount(userID uuid.UUID, provider string, newOAuthID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Delete any existing profile for this provider for this user
		if err := tx.Where("user_id = ? AND LOWER(provider) = LOWER(?)", userID, provider).Delete(&domain.UserOauthAccount{}).Error; err != nil {
			return fmt.Errorf("failed to delete old oauth account: %w", err)
		}

		// 2. Attach new OAuth profile to user_id
		if err := tx.Model(&domain.UserOauthAccount{}).Where("id = ?", newOAuthID).Update("user_id", userID).Error; err != nil {
			return fmt.Errorf("failed to re-bind new oauth account: %w", err)
		}

		// 3. Sync avatar if user does not have one
		var newAcc domain.UserOauthAccount
		if err := tx.First(&newAcc, newOAuthID).Error; err == nil && newAcc.AvatarUrl != "" {
			var user domain.AppUser
			if err := tx.First(&user, userID).Error; err == nil {
				if user.ImageProfileUrl == nil || *user.ImageProfileUrl == "" {
					_ = tx.Model(&domain.AppUser{}).Where("id = ?", userID).Update("image_profile_url", newAcc.AvatarUrl)
					_ = tx.Model(&domain.UserInformation{}).Where("user_id = ?", userID).Update("logo_url", newAcc.AvatarUrl)
				}
			}
		}

		return nil
	})
}

