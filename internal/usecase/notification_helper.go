package usecase

import (
	"log"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/database"
	"time"

	"github.com/google/uuid"
)

func SendNotificationToUser(userID uuid.UUID, title, body, refID, refStatus, notifType string) {
	db := database.DB
	if db == nil {
		log.Println("[Notif-Helper-Mobile] database.DB is nil")
		return
	}

	var moduleID uuid.UUID
	switch notifType {
	case "complaint", "complaint_assignee":
		moduleID = uuid.MustParse("5b630777-4de8-42f7-926a-2335879e0f6d")
	case "tax":
		moduleID = uuid.MustParse("eadaf67c-db3e-4557-82cd-8bf721c3e2e7")
	default:
		if parsed, err := uuid.Parse(notifType); err == nil {
			moduleID = parsed
		} else {
			moduleID = uuid.MustParse("5b630777-4de8-42f7-926a-2335879e0f6d")
		}
	}

	newNotif := domain.ModuleNotification{
		ID:              uuid.New(),
		ModuleID:        moduleID,
		UserID:          &userID,
		ReferenceID:     refID,
		ReferenceTitle:  title,
		ReferenceBody:   body,
		ReferenceStatus: refStatus,
		Type:            "user",
		Status:          "published",
		State:           "unread",
		IsRead:          false,
		CreatedBy:       "mobile_usecase",
		CreatedDate:     time.Now(),
		UpdatedDate:     time.Now(),
	}

	if err := db.Create(&newNotif).Error; err != nil {
		log.Printf("[Notif-Helper-Mobile] Failed to save user notification: %v", err)
	}
}

func SendNotificationToDepartment(departmentID string, role string, title, body, refID, refStatus string) {
	db := database.DB
	if db == nil {
		log.Println("[Notif-Helper-Mobile] database.DB is nil")
		return
	}

	var deptUUID *uuid.UUID
	if departmentID != "" {
		if parsed, err := uuid.Parse(departmentID); err == nil {
			deptUUID = &parsed
		}
	}

	var roleStr *string
	if role != "" {
		roleStr = &role
	}

	newNotif := domain.ModuleNotification{
		ID:              uuid.New(),
		ModuleID:        uuid.MustParse("d01b2ce5-34a9-498b-bba0-b1b8360f1ea9"),
		DepartmentID:    deptUUID,
		Role:            roleStr,
		ReferenceID:     refID,
		ReferenceTitle:  title,
		ReferenceBody:   body,
		ReferenceStatus: refStatus,
		Type:            "admin",
		Status:          "published",
		State:           "unread",
		IsRead:          false,
		CreatedBy:       "mobile_usecase",
		CreatedDate:     time.Now(),
		UpdatedDate:     time.Now(),
	}

	if err := db.Create(&newNotif).Error; err != nil {
		log.Printf("[Notif-Helper-Mobile] Failed to save admin notification: %v", err)
	}
}
