package repository

import (
	"time"

	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type verificationRepository struct {
	db *gorm.DB
}

func NewVerificationRepository(db *gorm.DB) domain.VerificationRepository {
	return &verificationRepository{db: db}
}

func (r *verificationRepository) GetUserWithInfo(userID uuid.UUID) (*domain.AppUser, error) {
	var user domain.AppUser
	err := r.db.Preload("Information").First(&user, "id = ?", userID).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *verificationRepository) GetModulesForMenu() ([]domain.Module, error) {
	var modules []domain.Module
	err := r.db.Where("is_admin_only = false").Order("sequence ASC NULLS LAST").Find(&modules).Error
	if err != nil {
		return nil, err
	}
	return modules, nil
}

func (r *verificationRepository) SubmitVerification(userID uuid.UUID, req *domain.SubmitVerificationRequest) error {
	updates := map[string]interface{}{
		"verification_status": string(domain.VerificationStatusPending),
		"id_card_type":        req.IdCardType,
		"id_card_photo_url":   req.IdCardPhotoUrl,
		"id_card_expiry":      req.IdCardExpiry,
		"updated_date":        time.Now(),
	}
	return r.db.Model(&domain.UserInformation{}).
		Where("user_id = ?", userID).
		Updates(updates).Error
}

func (r *verificationRepository) GetVerificationStatus(userID uuid.UUID) (*domain.VerificationStatusResponse, error) {
	var info domain.UserInformation
	err := r.db.Select("verification_status, verified_date").
		Where("user_id = ?", userID).
		First(&info).Error
	if err != nil {
		return nil, err
	}
	return &domain.VerificationStatusResponse{
		VerificationStatus: info.VerificationStatus,
		VerifiedDate:       info.VerifiedDate,
	}, nil
}

func (r *verificationRepository) RegisterFCMToken(userID uuid.UUID, req *domain.RegisterFCMTokenRequest) error {
	token := domain.UserFCMToken{
		UserID:      userID,
		DeviceID:    req.DeviceID,
		Token:       req.Token,
		CreatedDate: time.Now(),
		UpdatedDate: time.Now(),
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "device_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"token", "updated_date"}),
	}).Create(&token).Error
}

func (r *verificationRepository) GetFCMTokensByUserID(userID uuid.UUID) ([]string, error) {
	var tokens []domain.UserFCMToken
	err := r.db.Where("user_id = ?", userID).Find(&tokens).Error
	if err != nil {
		return nil, err
	}
	result := make([]string, len(tokens))
	for i, t := range tokens {
		result[i] = t.Token
	}
	return result, nil
}
