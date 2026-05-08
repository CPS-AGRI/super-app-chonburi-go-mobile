package usecase

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
	
	// Default status if not set
	if complaint.Status == "" {
		complaint.Status = domain.ComplaintStatusSubmitted
	}

	// Map Images
	for i, url := range imageURLs {
		complaint.Images = append(complaint.Images, domain.ComplaintImage{
			ID:                uuid.New().String(),
			ModuleComplaintId: complaint.ID,
			Url:               url,
			Sequence:          i + 1,
			CreatedDate:       time.Now(),
		})
	}

	return u.repo.Create(complaint)
}

func (u *complaintUseCase) GetMyComplaints(userID string, status string, search string) ([]domain.Complaint, error) {
	return u.repo.GetByUserID(userID, status, search)
}

func (u *complaintUseCase) GetDetail(id string, userID string) (*domain.Complaint, error) {
	return u.repo.GetByID(id, userID)
}

func (u *complaintUseCase) CancelComplaint(id string, userID string) error {
	// 1. Check ownership
	complaint, err := u.repo.GetByID(id, userID)
	if err != nil {
		return err
	}

	// 2. Only allow canceling if not completed/already canceled
	if complaint.Status == domain.ComplaintStatusCompleted || complaint.Status == domain.ComplaintStatusCanceled {
		return fmt.Errorf("cannot cancel complaint in status: %s", complaint.Status)
	}

	return u.repo.UpdateStatus(id, domain.ComplaintStatusCanceled)
}

func generateDocumentID() string {
	now := time.Now()
	rand.Seed(time.Now().UnixNano())
	randomPart := rand.Intn(9999)
	return fmt.Sprintf("CP-%s-%04d", now.Format("20060102"), randomPart)
}
