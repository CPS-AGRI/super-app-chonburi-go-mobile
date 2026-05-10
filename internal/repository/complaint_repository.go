package repository

import (
	"super-app-chonburi-go-mobile/internal/domain"
	"gorm.io/gorm"
)

type complaintRepository struct {
	db *gorm.DB
}

func NewComplaintRepository(db *gorm.DB) domain.ComplaintRepository {
	return &complaintRepository{db: db}
}

func (r *complaintRepository) Create(complaint *domain.Complaint) error {
	return r.db.Create(complaint).Error
}

func (r *complaintRepository) GetByUserID(userID string, status string, search string, page int, limit int) ([]domain.Complaint, error) {
	var complaints []domain.Complaint
	query := r.db.Preload("Images").Preload("ModuleType").Preload("Activities.Images").Where("user_id = ?", userID)

	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}

	if search != "" {
		query = query.Where("description ILIKE ? OR document_id ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	err := query.Order("module_complaints.created_date DESC").Limit(limit).Offset(offset).Find(&complaints).Error
	return complaints, err
}

func (r *complaintRepository) GetByID(id string, userID string) (*domain.Complaint, error) {
	var complaint domain.Complaint
	err := r.db.Preload("Images").Preload("ModuleType").Preload("Activities.Images").Where("id = ? AND user_id = ?", id, userID).First(&complaint).Error
	if err != nil {
		return nil, err
	}
	return &complaint, nil
}

func (r *complaintRepository) Update(complaint *domain.Complaint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Update main complaint fields explicitly
		updateData := map[string]interface{}{
			"module_type_id": complaint.ModuleTypeId,
			"description":    complaint.Description,
			"latitude":       complaint.Latitude,
			"longitude":      complaint.Longitude,
			"status":         complaint.Status,
			"updated_date":   complaint.UpdatedDate,
			"updated_by":     complaint.UpdatedBy,
		}

		if err := tx.Table("module_complaints").Where("id = ?", complaint.ID).Updates(updateData).Error; err != nil {
			return err
		}

		// 2. Sync images: Delete old ones and insert new ones
		if err := tx.Exec("DELETE FROM module_complaint_images WHERE module_complaint_id = ?", complaint.ID).Error; err != nil {
			return err
		}

		if len(complaint.Images) > 0 {
			if err := tx.Create(&complaint.Images).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *complaintRepository) UpdateStatus(id string, status string) error {
	return r.db.Model(&domain.Complaint{}).Where("id = ?", id).Update("status", status).Error
}

func (r *complaintRepository) Delete(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Delete activity images first (deepest level)
		err := tx.Exec(`
			DELETE FROM module_complaint_activity_images 
			WHERE module_complaint_activity_id IN (
				SELECT id FROM module_complaint_activities WHERE module_complaint_id = ?
			)`, id).Error
		if err != nil {
			return err
		}

		// 2. Delete activities
		if err := tx.Exec("DELETE FROM module_complaint_activities WHERE module_complaint_id = ?", id).Error; err != nil {
			return err
		}

		// 3. Delete complaint images
		if err := tx.Exec("DELETE FROM module_complaint_images WHERE module_complaint_id = ?", id).Error; err != nil {
			return err
		}

		// 4. Delete the complaint itself (top level)
		if err := tx.Exec("DELETE FROM module_complaints WHERE id = ?", id).Error; err != nil {
			return err
		}

		return nil
	})
}

func (r *complaintRepository) GetFirstUserID() (string, error) {
	var userID string
	err := r.db.Table("users").Select("id").Limit(1).Scan(&userID).Error
	return userID, err
}
