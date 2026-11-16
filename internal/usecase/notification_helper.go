package usecase

import (
	"log"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/database"
	"time"

	"github.com/google/uuid"
)

var (
	mobileModuleIDComplaint       = uuid.MustParse("5b630777-4de8-42f7-926a-2335879e0f6d")
	mobileModuleIDComplaintCenter = uuid.MustParse("d01b2ce5-34a9-498b-bba0-b1b8360f1ea9")
	mobileModuleIDTaxPayment      = uuid.MustParse("eadaf67c-db3e-4557-82cd-8bf721c3e2e7")
	mobileModuleIDPublicRelations = uuid.MustParse("d04d6d51-b35d-43df-be91-31178986aae3")
)

func resolveMobileNotificationModuleID(notifType string, fallback uuid.UUID) uuid.UUID {
	switch notifType {
	case "complaint", "complaint_assignee":
		return mobileModuleIDComplaint
	case "complaint_center":
		return mobileModuleIDComplaintCenter
	case "tax":
		return mobileModuleIDTaxPayment
	case "pr", "news", "advertise":
		return mobileModuleIDPublicRelations
	default:
		if parsed, err := uuid.Parse(notifType); err == nil {
			return parsed
		}
		return fallback
	}
}

func SendNotificationToUser(userID uuid.UUID, title, body, refID, refStatus, notifType string) {
	db := database.DB
	if db == nil {
		log.Println("[Notif-Helper-Mobile] database.DB is nil")
		return
	}

	moduleID := resolveMobileNotificationModuleID(notifType, mobileModuleIDComplaint)

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
	SendNotificationToDepartmentWithModule("complaint_center", departmentID, role, title, body, refID, refStatus)
}

func SendNotificationToDepartmentWithModule(notifType, departmentID, role, title, body, refID, refStatus string) {
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
		ModuleID:        resolveMobileNotificationModuleID(notifType, mobileModuleIDComplaintCenter),
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

func SendNotificationToAdmin(adminID uuid.UUID, title, body, refID, refStatus, notifType string) {
	db := database.DB
	if db == nil {
		log.Println("[Notif-Helper-Mobile] database.DB is nil")
		return
	}

	newNotif := domain.ModuleNotification{
		ID:              uuid.New(),
		ModuleID:        resolveMobileNotificationModuleID(notifType, mobileModuleIDComplaint),
		UserAdminID:     &adminID,
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
		log.Printf("[Notif-Helper-Mobile] Failed to save admin direct notification: %v", err)
	}
}
