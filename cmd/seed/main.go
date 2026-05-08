package main

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/database"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// 1. Load config
	cfg := config.LoadConfig()

	// 2. Connect to Database
	database.ConnectDB(cfg)

	// --- SEED USERS ---
	type SeedUser struct {
		User    domain.AppUser
		Profile domain.UserInformation
	}

	seedUsers := []SeedUser{
		{
			User: domain.AppUser{
				PhoneNumber: "0812345671",
				IsConsent:   true,
				CreatedBy:   "seed_data",
			},
			Profile: domain.UserInformation{
				Name:           "สมชาย",
				LastName:       "สายชล",
				Email:          stringPtr("somchai@example.com"),
				Phone:          "0812345671",
				IdentityNumberEncrypted: "ENC_1100112233441",
				LaserIdEncrypted:        "ENC_ME0123456781",
				Status:         "active",
				IsConsent:      true,
			},
		},
	}

	hashedPin, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	pinHash := string(hashedPin)

	var firstUserID string

	for _, data := range seedUsers {
		var existingUser domain.AppUser
		if err := database.DB.Where("phone_number = ?", data.User.PhoneNumber).First(&existingUser).Error; err == nil {
			firstUserID = existingUser.ID.String()
			continue
		}

		data.User.ID = uuid.New()
		data.User.PinHash = pinHash
		data.User.PhoneNumberHash = hashValue(data.User.PhoneNumber)
		
		tx := database.DB.Begin()
		tx.Create(&data.User)
		data.Profile.UserId = data.User.ID
		tx.Create(&data.Profile)
		tx.Commit()
		
		firstUserID = data.User.ID.String()
		log.Printf("✅ Seeded User: %s", data.Profile.Name)
	}

	// --- SEED COMPLAINTS ---
	if firstUserID != "" {
		complaints := []domain.Complaint{
			{
				ID:           uuid.New().String(),
				UserId:       firstUserID,
				ModuleTypeId: uuid.New().String(),
				DocumentId:   "CP-20240508-0001",
				Title:        "ถนนเป็นหลุมบ่อ",
				Description:  "ถนนเส้นหน้าหมู่บ้านชลบุรีพลัสเป็นหลุมขนาดใหญ่ เสี่ยงเกิดอุบัติเหตุ",
				Status:       domain.ComplaintStatusUpdated,
				CreatedDate:  time.Now().Add(-24 * time.Hour),
				UpdatedDate:  time.Now(),
				CreatedBy:    firstUserID,
				UpdatedBy:    firstUserID,
			},
			{
				ID:           uuid.New().String(),
				UserId:       firstUserID,
				ModuleTypeId: uuid.New().String(),
				DocumentId:   "CP-20240508-0002",
				Title:        "ไฟถนนดับ",
				Description:  "ไฟกิ่งซอย 5 ดับทั้งซอย มืดมากครับ",
				Status:       domain.ComplaintStatusInProgress,
				CreatedDate:  time.Now().Add(-48 * time.Hour),
				UpdatedDate:  time.Now().Add(-12 * time.Hour),
				CreatedBy:    firstUserID,
				UpdatedBy:    firstUserID,
			},
			{
				ID:           uuid.New().String(),
				UserId:       firstUserID,
				ModuleTypeId: uuid.New().String(),
				DocumentId:   "CP-20240508-0003",
				Title:        "ขยะตกค้าง",
				Description:  "ไม่ได้มาเก็บขยะมา 3 วันแล้วครับ ส่งกลิ่นเหม็น",
				Status:       domain.ComplaintStatusSubmitted,
				CreatedDate:  time.Now().Add(-2 * time.Hour),
				UpdatedDate:  time.Now().Add(-2 * time.Hour),
				CreatedBy:    firstUserID,
				UpdatedBy:    firstUserID,
			},
		}

		for _, c := range complaints {
			var existing domain.Complaint
			if err := database.DB.Where("document_id = ?", c.DocumentId).First(&existing).Error; err != nil {
				database.DB.Create(&c)
				log.Printf("✅ Seeded Complaint: %s", c.DocumentId)
			}
		}
	}

	log.Println("✨ Seeding completed!")
}

func hashValue(v string) string {
	h := sha256.New()
	h.Write([]byte(v))
	return hex.EncodeToString(h.Sum(nil))
}

func stringPtr(s string) *string {
	return &s
}
