package main

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/database"

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

	log.Printf("✨ Seeding completed! First User ID for testing: %s\n", firstUserID)
}

func hashValue(v string) string {
	h := sha256.New()
	h.Write([]byte(v))
	return hex.EncodeToString(h.Sum(nil))
}

func stringPtr(s string) *string {
	return &s
}
