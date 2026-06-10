package repository

import (
	"sort"
	"strings"
	"time"

	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type notificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) domain.NotificationRepository {
	return &notificationRepository{db: db}
}

func (r *notificationRepository) GetNotifications(userID uuid.UUID) ([]domain.NotificationResponseItem, error) {

	type txRow struct {
		ID             uuid.UUID
		ReferenceTitle string
		ReferenceBody  string
		CreatedDate    time.Time
		UpdatedDate    time.Time
		ModuleName     string
		Read           bool
		ReferenceID    string
	}
	var txItems []txRow
	err := r.db.Raw(`
		SELECT n.id, n.reference_title, n.reference_body, n.created_date, n.updated_date, COALESCE(m.name_th, '') as module_name,
		       (un.module_notification_id IS NOT NULL) as read, n.reference_id
		FROM module_notifications n
		LEFT JOIN modules m ON m.id = n.module_id
		LEFT JOIN module_user_notifications un ON un.module_notification_id = n.id AND un.user_id = ?
		WHERE (n.user_id = ? OR (n.user_id IS NULL AND n.type = 'user'))
	`, userID, userID).Scan(&txItems).Error
	if err != nil {
		return nil, err
	}

	type prRow struct {
		ID               uuid.UUID
		Title            string
		Description      string
		CreatedDate      time.Time
		UpdatedDate      time.Time
		ModuleName       string
		Read             bool
		PublicRelationID *uuid.UUID
	}
	var prItems []prRow
	err = r.db.Raw(`
		SELECT p.id, p.title, COALESCE(p.description, '') as description, p.created_date, p.updated_date, COALESCE(m.name_th, '') as module_name,
		       (un.module_notification_id IS NOT NULL) as read, p.public_relation_id
		FROM module_public_relation_notifications p
		LEFT JOIN modules m ON m.id = p.module_id
		LEFT JOIN module_user_notifications un ON un.module_notification_id = p.id AND un.user_id = ?
		WHERE p.status = 'active' AND (p.send_date IS NULL OR p.send_date <= NOW())
	`, userID).Scan(&prItems).Error
	if err != nil {
		return nil, err
	}

	var results []domain.NotificationResponseItem

	for _, item := range txItems {

		notifType := "general"
		nameLower := strings.ToLower(item.ModuleName)
		if strings.Contains(nameLower, "ร้องทุกข์") || strings.Contains(nameLower, "complaint") {
			notifType = "complaint"
		} else if strings.Contains(nameLower, "ภาษี") || strings.Contains(nameLower, "tax") {
			notifType = "tax"
		} else if strings.Contains(nameLower, "น้ำท่วม") || strings.Contains(nameLower, "flood") || strings.Contains(nameLower, "ภัย") {
			notifType = "flood"
		}

		results = append(results, domain.NotificationResponseItem{
			ID:          item.ID.String(),
			Title:       item.ReferenceTitle,
			Description: item.ReferenceBody,
			Date:        item.CreatedDate.Format(time.RFC3339),
			UpdatedDate: item.UpdatedDate.Format(time.RFC3339),
			Type:        notifType,
			Read:        item.Read,
			ReferenceID: item.ReferenceID,
		})
	}

	for _, item := range prItems {
		var refID string
		if item.PublicRelationID != nil {
			refID = item.PublicRelationID.String()
		}
		results = append(results, domain.NotificationResponseItem{
			ID:          item.ID.String(),
			Title:       item.Title,
			Description: item.Description,
			Date:        item.CreatedDate.Format(time.RFC3339),
			UpdatedDate: item.UpdatedDate.Format(time.RFC3339),
			Type:        "news",
			Read:        item.Read,
			ReferenceID: refID,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		ti, errI := time.Parse(time.RFC3339, results[i].UpdatedDate)
		tj, errJ := time.Parse(time.RFC3339, results[j].UpdatedDate)
		if errI == nil && errJ == nil {
			return ti.After(tj)
		}
		return results[i].UpdatedDate > results[j].UpdatedDate
	})

	return results, nil
}

func (r *notificationRepository) MarkAsRead(userID uuid.UUID, notificationID uuid.UUID) error {

	return r.db.Exec(`
		INSERT INTO module_user_notifications (module_notification_id, user_id, created_date)
		VALUES (?, ?, NOW())
		ON CONFLICT (module_notification_id, user_id) DO NOTHING
	`, notificationID, userID).Error
}

func (r *notificationRepository) MarkAllAsRead(userID uuid.UUID) error {

	return r.db.Exec(`
		INSERT INTO module_user_notifications (module_notification_id, user_id, created_date)
		SELECT id, ?, NOW() FROM (
			SELECT id FROM module_notifications WHERE (user_id = ? OR (user_id IS NULL AND type = 'user'))
			UNION
			SELECT id FROM module_public_relation_notifications WHERE status = 'active' AND (send_date IS NULL OR send_date <= NOW())
		) AS all_notifs
		ON CONFLICT (module_notification_id, user_id) DO NOTHING
	`, userID, userID).Error
}
