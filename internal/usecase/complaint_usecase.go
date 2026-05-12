package usecase // Updated for DeleteComplaint synchronization

import (
	"fmt"
	"math/rand"
	"super-app-chonburi-go-mobile/internal/domain"
	"time"

	"github.com/google/uuid"
)

type complaintUseCase struct {
	repo domain.ComplaintRepository
}

func NewComplaintUseCase(repo domain.ComplaintRepository) domain.ComplaintUseCase {
	return &complaintUseCase{repo: repo}
}

func (u *complaintUseCase) AddComplaint(complaint *domain.Complaint, imageURLs []string) error {
	complaint.ID = uuid.New().String()
	complaint.DocumentId = generateDocumentID()
	complaint.CreatedDate = time.Now()
	complaint.UpdatedDate = time.Now()
	
	// Default to pending if not draft
	if complaint.Status != domain.ComplaintStatusDraft {
		complaint.Status = domain.ComplaintStatusPending
	}

	// Map Images
	for i, url := range imageURLs {
		complaint.Images = append(complaint.Images, domain.ComplaintImage{
			ID:                uuid.New().String(),
			ModuleComplaintId: complaint.ID,
			Url:               url,
			Sequence:          i + 1,
			CreatedDate:       time.Now(),
			UpdatedDate:       time.Now(),
			CreatedBy:         complaint.UserId,
			UpdatedBy:         complaint.UserId,
		})
	}

	return u.repo.Create(complaint)
}

func (u *complaintUseCase) UpdateComplaint(complaint *domain.Complaint, imageURLs []string) error {
	// 1. Check if exists and ownership
	existing, err := u.repo.GetByID(complaint.ID, complaint.UserId)
	if err != nil {
		return fmt.Errorf("complaint not found or unauthorized")
	}

	// 2. Update fields
	existing.Description = complaint.Description
	existing.ModuleTypeId = complaint.ModuleTypeId
	existing.Latitude = complaint.Latitude
	existing.Longitude = complaint.Longitude
	existing.Status = complaint.Status
	existing.UpdatedDate = time.Now()
	existing.UpdatedBy = complaint.UserId

	// 3. Map new images
	existing.Images = []domain.ComplaintImage{}
	for i, url := range imageURLs {
		existing.Images = append(existing.Images, domain.ComplaintImage{
			ID:                uuid.New().String(),
			ModuleComplaintId: existing.ID,
			Url:               url,
			Sequence:          i + 1,
			CreatedDate:       time.Now(),
			UpdatedDate:       time.Now(),
			CreatedBy:         complaint.UserId,
			UpdatedBy:         complaint.UserId,
		})
	}

	return u.repo.Update(existing)
}

func (u *complaintUseCase) GetMyComplaints(userID string, status string, search string, page int, limit int) ([]domain.Complaint, error) {
	return u.repo.GetByUserID(userID, status, search, page, limit)
}

func (u *complaintUseCase) GetDetail(id string, userID string) (*domain.Complaint, error) {
	return u.repo.GetByID(id, userID)
}

func (u *complaintUseCase) DeleteComplaint(id string, userID string) error {
	// 1. Check ownership
	_, err := u.repo.GetByID(id, userID)
	if err != nil {
		return fmt.Errorf("complaint not found or unauthorized")
	}

	return u.repo.Delete(id)
}

func (u *complaintUseCase) GetFirstUserID() (string, error) {
	return u.repo.GetFirstUserID()
}

func generateDocumentID() string {
	now := time.Now()
	rand.Seed(time.Now().UnixNano())
	randomPart := rand.Intn(9999)
	return fmt.Sprintf("CP-%s-%04d", now.Format("20060102"), randomPart)
}
