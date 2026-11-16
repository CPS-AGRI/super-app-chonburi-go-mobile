package repository

import (
	"errors"
	"fmt"
	"strings"
	"time"

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

func isUserSoftDeleted(user *domain.AppUser) bool {
	if user == nil {
		return true
	}
	if strings.HasPrefix(user.PhoneNumber, "DELETED_") {
		return true
	}
	if user.Information != nil {
		status := strings.ToLower(strings.TrimSpace(user.Information.Status))
		if status == "inactive" || status == "deleted" {
			return true
		}
	}
	return false
}

func (r *authRepository) GetByID(id uuid.UUID) (*domain.AppUser, error) {
	var user domain.AppUser
	err := r.db.Preload("Information").Preload("OauthAccounts").First(&user, id).Error
	if err != nil {
		return nil, err
	}
	if isUserSoftDeleted(&user) {
		return nil, gorm.ErrRecordNotFound
	}
	return &user, nil
}

func (r *authRepository) GetByProviderID(provider, providerID string) (*domain.AppUser, error) {
	var oauthAccs []domain.UserOauthAccount
	err := r.db.Where("LOWER(provider) = LOWER(?) AND provider_id = ?", provider, providerID).
		Order("created_at DESC").
		Find(&oauthAccs).Error
	if err != nil {
		return nil, err
	}
	if len(oauthAccs) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	for _, oauthAcc := range oauthAccs {
		var user domain.AppUser
		if err := r.db.Preload("Information").Preload("OauthAccounts").First(&user, oauthAcc.UserId).Error; err == nil {
			if !isUserSoftDeleted(&user) {
				return &user, nil
			}
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *authRepository) GetByOAuthAccountID(oauthID uuid.UUID) (*domain.AppUser, error) {
	var oauthAcc domain.UserOauthAccount
	if err := r.db.Where("id = ?", oauthID).First(&oauthAcc).Error; err != nil {
		return nil, err
	}

	var user domain.AppUser
	if err := r.db.Preload("Information").Preload("OauthAccounts").First(&user, oauthAcc.UserId).Error; err != nil {
		return nil, err
	}
	if isUserSoftDeleted(&user) {
		return nil, gorm.ErrRecordNotFound
	}
	return &user, nil
}

func (r *authRepository) GetByEmail(email string) (*domain.AppUser, error) {
	if email == "" {
		return nil, gorm.ErrRecordNotFound
	}

	// Only match verified OAuth accounts in user_oauth_accounts table to prevent pre-account takeover
	// of unverified email addresses in users / user_informations.
	var oauthAccs []domain.UserOauthAccount
	if err := r.db.Where("email = ?", email).Order("created_at DESC").Find(&oauthAccs).Error; err == nil {
		for _, oauthAcc := range oauthAccs {
			var user domain.AppUser
			if err := r.db.Preload("Information").Preload("OauthAccounts").First(&user, oauthAcc.UserId).Error; err == nil {
				if !isUserSoftDeleted(&user) {
					return &user, nil
				}
			}
		}
	}

	return nil, gorm.ErrRecordNotFound
}

func (r *authRepository) GetByPhoneNumber(phoneNumber string) (*domain.AppUser, error) {
	if phoneNumber == "" || strings.HasPrefix(phoneNumber, "DELETED_") {
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

	var users []domain.AppUser
	err := r.db.Preload("Information").Preload("OauthAccounts").
		Where("phone_number IN ?", formats).
		Order("updated_date DESC").
		Find(&users).Error
	if err != nil {
		return nil, err
	}
	for i := range users {
		if !isUserSoftDeleted(&users[i]) {
			return &users[i], nil
		}
	}
	return nil, gorm.ErrRecordNotFound
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

func (r *authRepository) SoftDeleteUser(userID uuid.UUID) error {
	now := time.Now()
	shortID := userID.String()
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var user domain.AppUser
		if err := tx.First(&user, "id = ?", userID).Error; err != nil {
			return err
		}

		// 1. Scrub PII on users row while preserving row for foreign-key referential integrity
		if err := tx.Model(&domain.AppUser{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"phone_number":      "DELETED_" + userID.String(),
			"phone_number_hash": "DELETED_" + userID.String(),
			"pin_hash":          "",
			"email":             nil,
			"email_hash":        nil,
			"image_profile_url": nil,
			"is_consent":        false,
			"updated_by":        "user_soft_delete",
			"updated_date":      now,
		}).Error; err != nil {
			return err
		}

		// 2. Scrub PII on user_informations row and set status = inactive
		if err := tx.Model(&domain.UserInformation{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
			"status":                    "inactive",
			"verification_status":       "unverified",
			"prefix":                    "",
			"name":                      "ผู้ใช้งานที่ยกเลิกบัญชี",
			"last_name":                 "",
			"phone":                     "DELETED_" + shortID,
			"email":                     nil,
			"birthday":                  nil,
			"identity_number_encrypted": "",
			"identity_number_hash":      "DELETED_" + userID.String(),
			"id_card_photo_url":         nil,
			"id_card_expiry":            nil,
			"house_number":              "",
			"village_number":            "",
			"alley":                     "",
			"intersection":              "",
			"road":                      "",
			"building_name":             "",
			"room_number":               "",
			"card_address":              "",
			"logo_url":                  "",
			"is_consent":                false,
			"updated_by":                "user_soft_delete",
			"updated_date":              now,
		}).Error; err != nil {
			return err
		}

		// 3. Delete linked OAuth accounts, FCM tokens, and weather subscriptions
		if err := tx.Where("user_id = ?", userID).Delete(&domain.UserOauthAccount{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&domain.UserFCMToken{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&domain.UserWeatherSubscription{}).Error; err != nil {
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

// SocialUnlinkCleanup describes which profile fields should be cleared when unlinking a social account.
type SocialUnlinkCleanup struct {
	ClearImageProfileURL bool
	ClearInfoLogoURL     bool
	ClearUserEmail       bool
	ClearInfoEmail       bool
}

// ResolveSocialUnlinkCleanup determines whether avatar/email came solely from the unlinked OAuth account
// and should be cleared upon unlinking without disturbing manually entered data or remaining linked accounts.
func ResolveSocialUnlinkCleanup(user *domain.AppUser, target *domain.UserOauthAccount, remaining []domain.UserOauthAccount) SocialUnlinkCleanup {
	var cleanup SocialUnlinkCleanup
	if user == nil || target == nil {
		return cleanup
	}

	if target.AvatarUrl != "" {
		sharedAvatar := false
		for _, other := range remaining {
			if other.AvatarUrl == target.AvatarUrl {
				sharedAvatar = true
				break
			}
		}
		if !sharedAvatar {
			if user.ImageProfileUrl != nil && *user.ImageProfileUrl == target.AvatarUrl {
				cleanup.ClearImageProfileURL = true
			}
			if user.Information != nil && user.Information.LogoUrl == target.AvatarUrl {
				cleanup.ClearInfoLogoURL = true
			}
		}
	}

	hasManualEmail := user.EmailHash != nil && *user.EmailHash != ""
	if target.Email != "" && !hasManualEmail {
		sharedEmail := false
		for _, other := range remaining {
			if other.Email != "" && strings.EqualFold(other.Email, target.Email) {
				sharedEmail = true
				break
			}
		}
		if !sharedEmail {
			if user.Email != nil && strings.EqualFold(*user.Email, target.Email) {
				cleanup.ClearUserEmail = true
			}
			if user.Information != nil && user.Information.Email != nil && strings.EqualFold(*user.Information.Email, target.Email) {
				cleanup.ClearInfoEmail = true
			}
		}
	}

	return cleanup
}

func (r *authRepository) UnlinkSocial(userID uuid.UUID, provider string) error {
	var user domain.AppUser
	if err := r.db.Preload("Information").Preload("OauthAccounts").First(&user, userID).Error; err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	var target *domain.UserOauthAccount
	var remaining []domain.UserOauthAccount
	for i := range user.OauthAccounts {
		acc := user.OauthAccounts[i]
		if target == nil && strings.EqualFold(acc.Provider, provider) {
			target = &acc
		} else {
			remaining = append(remaining, acc)
		}
	}

	if target == nil {
		return errors.New("ไม่พบบัญชีที่เชื่อมต่อสำหรับบริการนี้")
	}

	// Safety check: Don't allow unlink if this is the user's ONLY login method
	hasPhonePin := user.PhoneNumber != "" && user.PinHash != ""
	if !hasPhonePin && len(remaining) == 0 {
		return errors.New("ไม่สามารถยกเลิกการเชื่อมต่อได้ เนื่องจากเป็นช่องทางเข้าสู่ระบบเดียวของคุณ กรุณาตั้งค่าเบอร์โทรศัพท์และรหัส PIN ก่อน")
	}

	cleanup := ResolveSocialUnlinkCleanup(&user, target, remaining)

	return r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("user_id = ? AND LOWER(provider) = LOWER(?)", userID, provider).Delete(&domain.UserOauthAccount{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("ไม่พบบัญชีที่เชื่อมต่อสำหรับบริการนี้")
		}

		userUpdates := map[string]interface{}{}
		if cleanup.ClearImageProfileURL {
			userUpdates["image_profile_url"] = nil
		}
		if cleanup.ClearUserEmail {
			userUpdates["email"] = nil
		}
		if len(userUpdates) > 0 {
			if err := tx.Model(&domain.AppUser{}).Where("id = ?", userID).Updates(userUpdates).Error; err != nil {
				return err
			}
		}

		infoUpdates := map[string]interface{}{}
		if cleanup.ClearInfoLogoURL {
			infoUpdates["logo_url"] = ""
		}
		if cleanup.ClearInfoEmail {
			infoUpdates["email"] = nil
		}
		if len(infoUpdates) > 0 {
			if err := tx.Model(&domain.UserInformation{}).Where("user_id = ?", userID).Updates(infoUpdates).Error; err != nil {
				return err
			}
		}

		return nil
	})
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

func (r *authRepository) MigrateThaiIDAccount(oldUserID, newUserID uuid.UUID, oauthAcc *domain.UserOauthAccount, updateInfo *domain.UserInformation) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Delete ThaiID oauth account from old user
		if err := tx.Where("user_id = ? AND LOWER(provider) = 'thaiid'", oldUserID).Delete(&domain.UserOauthAccount{}).Error; err != nil {
			return fmt.Errorf("failed to unlink thaiid from old user: %w", err)
		}

		// 2. Demote old user's verification status
		if err := tx.Model(&domain.UserInformation{}).Where("user_id = ?", oldUserID).Updates(map[string]interface{}{
			"verification_status":       "unverified",
			"identity_number_hash":      "",
			"identity_number_encrypted": "",
		}).Error; err != nil {
			return fmt.Errorf("failed to reset old user verification: %w", err)
		}

		// 3. Delete any existing ThaiID oauth record for new user (prevent duplicate)
		if err := tx.Where("user_id = ? AND LOWER(provider) = 'thaiid'", newUserID).Delete(&domain.UserOauthAccount{}).Error; err != nil {
			return fmt.Errorf("failed to clean existing thaiid for new user: %w", err)
		}

		// 4. Create new ThaiID oauth account for new user
		oauthAcc.UserId = newUserID
		if oauthAcc.ID == uuid.Nil {
			oauthAcc.ID = uuid.New()
		}
		if err := tx.Create(oauthAcc).Error; err != nil {
			return fmt.Errorf("failed to create thaiid oauth account for new user: %w", err)
		}

		// 5. Update new user's information with DOPA verified data
		if updateInfo != nil {
			updateInfo.UserId = newUserID
			var count int64
			tx.Model(&domain.UserInformation{}).Where("user_id = ?", newUserID).Count(&count)
			if count > 0 {
				if err := tx.Model(&domain.UserInformation{}).Where("user_id = ?", newUserID).Updates(updateInfo).Error; err != nil {
					return fmt.Errorf("failed to update new user information: %w", err)
				}
			} else {
				if err := tx.Create(updateInfo).Error; err != nil {
					return fmt.Errorf("failed to create new user information: %w", err)
				}
			}
		}

		return nil
	})
}

