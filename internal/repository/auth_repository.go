package repository

import (
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
	var info domain.UserInformation
	err := r.db.Where("email = ?", email).First(&info).Error
	if err != nil {
		return nil, err
	}

	var user domain.AppUser
	err = r.db.Preload("Information").Preload("OauthAccounts").First(&user, info.UserId).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *authRepository) GetByPhoneNumber(phoneNumber string) (*domain.AppUser, error) {
	var user domain.AppUser
	err := r.db.Preload("Information").Preload("OauthAccounts").Where("phone_number = ?", phoneNumber).First(&user).Error
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
