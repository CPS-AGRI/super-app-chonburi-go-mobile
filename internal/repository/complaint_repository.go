package repository

import (
	"time"

	"gorm.io/gorm"
	"super-app-chonburi-go-mobile/internal/domain"
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
		if status == "pending" {
			query = query.Where("status IN ?", []string{"pending", "received", "rejected"})
		} else {
			query = query.Where("status = ?", status)
		}
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
		updateData := struct {
			ModuleTypeId string    `gorm:"column:module_type_id"`
			Description  string    `gorm:"column:description"`
			Latitude     float64   `gorm:"column:latitude"`
			Longitude    float64   `gorm:"column:longitude"`
			Status       string    `gorm:"column:status"`
			UpdatedDate  time.Time `gorm:"column:updated_date"`
			UpdatedBy    string    `gorm:"column:updated_by"`
			AssigneeId   *string   `gorm:"column:assignee_id"`
			DepartmentId *string   `gorm:"column:department_id"`
			IsDisputed   bool      `gorm:"column:is_disputed"`
		}{
			ModuleTypeId: complaint.ModuleTypeId,
			Description:  complaint.Description,
			Latitude:     complaint.Latitude,
			Longitude:    complaint.Longitude,
			Status:       complaint.Status,
			UpdatedDate:  complaint.UpdatedDate,
			UpdatedBy:    complaint.UpdatedBy,
			AssigneeId:   complaint.AssigneeId,
			DepartmentId: complaint.DepartmentId,
			IsDisputed:   complaint.IsDisputed,
		}

		if err := tx.Table("module_complaints").Where("id = ?", complaint.ID).Updates(updateData).Error; err != nil {
			return err
		}

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

		err := tx.Exec(`
			DELETE FROM module_complaint_activity_images 
			WHERE module_complaint_activity_id IN (
				SELECT id FROM module_complaint_activities WHERE module_complaint_id = ?
			)`, id).Error
		if err != nil {
			return err
		}

		if err := tx.Exec("DELETE FROM module_complaint_activities WHERE module_complaint_id = ?", id).Error; err != nil {
			return err
		}

		if err := tx.Exec("DELETE FROM module_complaint_images WHERE module_complaint_id = ?", id).Error; err != nil {
			return err
		}

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

func (r *complaintRepository) CreateActivity(activity *domain.ComplaintActivity) error {
	return r.db.Create(activity).Error
}

func (r *complaintRepository) CreateRatingHistory(history *domain.ComplaintRatingHistory) error {
	return r.db.Create(history).Error
}

func (r *complaintRepository) GetCompleterInfo(complaintID string) (*string, *string, error) {

	var complaint domain.Complaint
	errComp := r.db.Select("id, assignee_id, department_id").First(&complaint, "id = ?", complaintID).Error

	var completedBy string
	err := r.db.Table("module_complaint_activities").
		Where("module_complaint_id = ? AND status = ?", complaintID, "completed").
		Order("created_date DESC").
		Limit(1).
		Pluck("created_by", &completedBy).
		Error

	var assigneeIDPtr *string
	if err == nil && completedBy != "" {
		assigneeIDPtr = &completedBy
	} else if errComp == nil {
		assigneeIDPtr = complaint.AssigneeId
	}

	var deptIDPtr *string
	if errComp == nil && complaint.DepartmentId != nil {

		deptIDPtr = complaint.DepartmentId
	} else if assigneeIDPtr != nil {

		var deptID string
		errDept := r.db.Table("admin_departments").
			Where("admin_id = ?", *assigneeIDPtr).
			Limit(1).
			Pluck("department_id", &deptID).
			Error
		if errDept == nil && deptID != "" {
			deptIDPtr = &deptID
		}
	}

	return assigneeIDPtr, deptIDPtr, nil
}

func (r *complaintRepository) GetComplaintMode() (string, error) {
	var mode string
	err := r.db.Table("municipalities").Select("complaint_mode").Limit(1).Scan(&mode).Error
	if err != nil {
		return "", err
	}
	return mode, nil
}
