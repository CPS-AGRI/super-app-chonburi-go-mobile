package repository

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type verificationRepository struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewVerificationRepository(db *gorm.DB, cfgs ...*config.Config) domain.VerificationRepository {
	var cfg *config.Config
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	return &verificationRepository{db: db, cfg: cfg}
}

func (r *verificationRepository) getPIDHmacSecret() string {
	if r.cfg != nil && r.cfg.PIDHmacSecret != "" {
		return r.cfg.PIDHmacSecret
	}
	if envVal := os.Getenv("PID_HMAC_SECRET"); envVal != "" {
		return envVal
	}
	seed := os.Getenv("JWT_SECRET")
	if r.cfg != nil && r.cfg.JWTSecret != "" {
		seed = r.cfg.JWTSecret
	}
	if seed == "" {
		seed = "default-fallback-secret"
	}
	sum := sha256.Sum256([]byte(seed + ":pid_hmac_secret"))
	return hex.EncodeToString(sum[:])
}

func (r *verificationRepository) getPIDEncryptionKey() string {
	if r.cfg != nil && len(r.cfg.PIDEncryptionKey) == 64 {
		if b, err := hex.DecodeString(r.cfg.PIDEncryptionKey); err == nil && len(b) == 32 {
			return r.cfg.PIDEncryptionKey
		}
	}
	if envKey := os.Getenv("PID_ENCRYPTION_KEY"); len(envKey) == 64 {
		if b, err := hex.DecodeString(envKey); err == nil && len(b) == 32 {
			return envKey
		}
	}
	seed := os.Getenv("JWT_SECRET")
	if r.cfg != nil && r.cfg.JWTSecret != "" {
		seed = r.cfg.JWTSecret
	}
	if seed == "" {
		seed = "default-fallback-secret"
	}
	sum := sha256.Sum256([]byte(seed + ":pid_encryption_key"))
	return hex.EncodeToString(sum[:])
}

func hmacSHA256Repo(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func encryptAES256GCMRepo(plaintext, keyHex string) (string, error) {
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil || len(keyBytes) != 32 {
		return "", fmt.Errorf("PID_ENCRYPTION_KEY must be a 64-char hex string (32 bytes): %w", err)
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (r *verificationRepository) GetUserWithInfo(userID uuid.UUID) (*domain.AppUser, error) {
	var user domain.AppUser
	err := r.db.Preload("Information").Preload("OauthAccounts").First(&user, "id = ?", userID).Error
	if err != nil {
		return nil, err
	}
	if isUserSoftDeleted(&user) {
		return nil, gorm.ErrRecordNotFound
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
	identityHash := hmacSHA256Repo(req.IdentityNumber, r.getPIDHmacSecret())
	encryptedIdentity, err := encryptAES256GCMRepo(req.IdentityNumber, r.getPIDEncryptionKey())
	if err != nil {
		return fmt.Errorf("failed to encrypt identity number: %w", err)
	}

	var currentStatus string
	_ = r.db.Model(&domain.UserInformation{}).
		Select("verification_status").
		Where("user_id = ?", userID).
		Scan(&currentStatus)

	targetStatus := string(domain.VerificationStatusPending)
	if currentStatus == string(domain.VerificationStatusVerified) {
		targetStatus = string(domain.VerificationStatusVerified)
	}

	updates := map[string]interface{}{
		"verification_status":       targetStatus,
		"id_card_type":              req.IdCardType,
		"id_card_photo_url":         req.IdCardPhotoUrl,
		"id_card_expiry":            req.IdCardExpiry,
		"identity_number_encrypted": encryptedIdentity,
		"identity_number_hash":      identityHash,
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

	err = r.db.Model(&domain.UserInformation{}).
		Where("user_id = ?", userID).
		Updates(updates).Error
	if err != nil {
		return err
	}

	// Create admin notification if pending verification
	if targetStatus == string(domain.VerificationStatusPending) {
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
	}

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

const MaxDevicesPerUser = 3

func (r *verificationRepository) RegisterFCMToken(userID uuid.UUID, req *domain.RegisterFCMTokenRequest) error {
	now := time.Now()

	// 1. ตรวจสอบว่า device_id นี้ของ user_id นี้มีอยู่แล้วหรือไม่
	var existing domain.UserFCMToken
	err := r.db.Where("user_id = ? AND device_id = ?", userID, req.DeviceID).First(&existing).Error
	if err == nil {
		// เครื่องเดิม -> อัปเดต Token และ updated_date (เป็น Active ล่าสุด)
		return r.db.Model(&domain.UserFCMToken{}).
			Where("user_id = ? AND device_id = ?", userID, req.DeviceID).
			Updates(map[string]interface{}{
				"token":        req.Token,
				"updated_date": now,
			}).Error
	}

	// 2. ถ้าเป็นเครื่องใหม่ -> ตรวจสอบจำนวนเครื่องปัจจุบันของ User คนนี้
	var totalDevices int64
	r.db.Model(&domain.UserFCMToken{}).Where("user_id = ?", userID).Count(&totalDevices)

	// หากครบหรือเกินเพดาน MaxDevicesPerUser (3 เครื่อง) -> ดำเนินการ LRU Eviction: ลบเครื่องที่ updated_date เก่าที่สุดออก 1 เครื่อง
	if totalDevices >= MaxDevicesPerUser {
		var oldestDevice domain.UserFCMToken
		if err := r.db.Where("user_id = ?", userID).
			Order("updated_date ASC, created_date ASC").
			First(&oldestDevice).Error; err == nil {
			_ = r.db.Where("user_id = ? AND device_id = ?", userID, oldestDevice.DeviceID).
				Delete(&domain.UserFCMToken{}).Error
		}
	}

	// 3. บันทึกเครื่องใหม่เข้าไป
	newToken := domain.UserFCMToken{
		UserID:      userID,
		DeviceID:    req.DeviceID,
		Token:       req.Token,
		CreatedDate: now,
		UpdatedDate: now,
	}
	return r.db.Create(&newToken).Error
}

func (r *verificationRepository) UnregisterFCMToken(userID uuid.UUID, deviceID string) error {
	return r.db.Where("user_id = ? AND device_id = ?", userID, deviceID).
		Delete(&domain.UserFCMToken{}).Error
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

func (r *verificationRepository) UpdateAddress(userID uuid.UUID, req *domain.UpdateAddressRequest) error {
	var count int64
	r.db.Model(&domain.UserInformation{}).Where("user_id = ?", userID).Count(&count)
	if count == 0 {
		info := domain.UserInformation{
			UserId:        userID,
			HouseNumber:   req.HouseNumber,
			VillageNumber: req.VillageNumber,
			Alley:         req.Alley,
			Road:          req.Road,
			Subdistrict:   req.Subdistrict,
			District:      req.District,
			Province:      req.Province,
			PostalCode:    req.PostalCode,
			CreatedDate:   time.Now(),
			UpdatedDate:   time.Now(),
		}
		return r.db.Create(&info).Error
	}

	updates := map[string]interface{}{
		"house_number":   req.HouseNumber,
		"village_number": req.VillageNumber,
		"alley":          req.Alley,
		"road":           req.Road,
		"subdistrict":    req.Subdistrict,
		"district":       req.District,
		"province":       req.Province,
		"postal_code":    req.PostalCode,
		"updated_date":   time.Now(),
	}
	return r.db.Model(&domain.UserInformation{}).Where("user_id = ?", userID).Updates(updates).Error
}

