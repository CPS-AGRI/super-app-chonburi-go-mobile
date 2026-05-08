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
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(complaint).Error; err != nil {
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

func (r *complaintRepository) GetByUserID(userID string, status string, search string) ([]domain.Complaint, error) {
	var complaints []domain.Complaint
	query := r.db.Preload("Images").Where("user_id = ?", userID)

	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}

	if search != "" {
		query = query.Where("title ILIKE ? OR document_id ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	err := query.Order("created_date DESC").Find(&complaints).Error
	return complaints, err
}

func (r *complaintRepository) GetByID(id string, userID string) (*domain.Complaint, error) {
	var complaint domain.Complaint
	err := r.db.Preload("Images").Where("id = ? AND user_id = ?", id, userID).First(&complaint).Error
	if err != nil {
		return nil, err
	}
	return &complaint, nil
}

func (r *complaintRepository) UpdateStatus(id string, status string) error {
	return r.db.Model(&domain.Complaint{}).Where("id = ?", id).Update("status", status).Error
}
