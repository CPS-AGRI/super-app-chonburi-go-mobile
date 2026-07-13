package repository

import (
	"crypto/sha256"
	"encoding/hex"
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
	h := sha256.New()
	h.Write([]byte(req.IdentityNumber))
	identityHash := hex.EncodeToString(h.Sum(nil))

	h2 := sha256.New()
	h2.Write([]byte(req.LaserID))
	laserHash := hex.EncodeToString(h2.Sum(nil))

	updates := map[string]interface{}{
		"verification_status":       string(domain.VerificationStatusPending),
		"id_card_type":              req.IdCardType,
		"id_card_photo_url":         req.IdCardPhotoUrl,
		"id_card_expiry":            req.IdCardExpiry,
		"identity_number_encrypted": "ENC_" + req.IdentityNumber,
		"identity_number_hash":      identityHash,
		"laser_id_encrypted":        "ENC_" + req.LaserID,
		"laser_id_hash":             laserHash,
		"prefix":                    req.Prefix,
		"name":                      req.Name,
		"last_name":                 req.LastName,
		"email":                     req.Email,
		"birthday":                  req.Birthday,
		"house_number":              req.HouseNumber,
		"village_number":            req.VillageNumber,
		"alley":                     req.Alley,
		"intersection":              req.Intersection,
		"road":                      req.Road,
		"subdistrict":               req.Subdistrict,
		"district":                  req.District,
		"province":                  req.Province,
		"postal_code":               req.PostalCode,
		"building_name":             req.BuildingName,
		"room_number":               req.RoomNumber,
		"updated_date":              time.Now(),
	}

	err := r.db.Model(&domain.UserInformation{}).
		Where("user_id = ?", userID).
		Updates(updates).Error
	if err != nil {
		return err
	}

	// Create admin notification
	var moduleID uuid.UUID
	_ = r.db.Table("modules").Select("id").Where("key = ? OR key = ?", "register", "ModuleIdentityVerifications").Limit(1).Scan(&moduleID)
	if moduleID == uuid.Nil {
		moduleID = uuid.MustParse("8c7ce421-5d1f-41de-840b-14ac192d4778") // Fallback valid Module ID (การยืนยันตัวตน)
	}

	var deptID string
	_ = r.db.Table("department_modules").Select("department_id").Where("module_id = ?", moduleID).Limit(1).Scan(&deptID)

	var deptUUID *uuid.UUID
	if deptID != "" {
		if parsed, err := uuid.Parse(deptID); err == nil {
			deptUUID = &parsed
		}
	}

	roleEmp := "Employees"
	roleMgr := "Managers"
	title := "มีคำขอยืนยันตัวตนใหม่"
	body := "คำขอตรวจสอบการยืนยันตัวตนจากคุณ " + req.Name + " " + req.LastName

	// 1. Notification for Employees
	newNotifEmp := domain.ModuleNotification{
		ID:              uuid.New(),
		ModuleID:        moduleID,
		DepartmentID:    deptUUID,
		Role:            &roleEmp,
		ReferenceID:     userID.String(),
		ReferenceTitle:  title,
		ReferenceBody:   body,
		ReferenceStatus: "pending",
		Type:            "admin",
		Status:          "published",
		State:           "unread",
		IsRead:          false,
		CreatedBy:       "mobile_submit",
		CreatedDate:     time.Now(),
		UpdatedBy:       "mobile_submit",
		UpdatedDate:     time.Now(),
	}
	_ = r.db.Create(&newNotifEmp)

	// 2. Notification for Managers
	newNotifMgr := domain.ModuleNotification{
		ID:              uuid.New(),
		ModuleID:        moduleID,
		DepartmentID:    deptUUID,
		Role:            &roleMgr,
		ReferenceID:     userID.String(),
		ReferenceTitle:  title,
		ReferenceBody:   body,
		ReferenceStatus: "pending",
		Type:            "admin",
		Status:          "published",
		State:           "unread",
		IsRead:          false,
		CreatedBy:       "mobile_submit",
		CreatedDate:     time.Now(),
		UpdatedBy:       "mobile_submit",
		UpdatedDate:     time.Now(),
	}
	_ = r.db.Create(&newNotifMgr)

	return nil
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
