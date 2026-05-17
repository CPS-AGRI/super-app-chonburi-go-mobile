package usecase // Updated for DeleteComplaint synchronization

import (
	"encoding/json"
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
	complaint, err := u.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}

	// Calculate if rated this round
	if complaint.Status == domain.ComplaintStatusCompleted {
		// 1. Find the latest 'completed' activity timestamp
		var lastCompletedAt time.Time
		for _, act := range complaint.Activities {
			if act.Status == domain.ComplaintStatusCompleted {
				if act.CreatedDate.After(lastCompletedAt) {
					lastCompletedAt = act.CreatedDate
				}
			}
		}

		// 2. Look for 'user_rating' activity after the last 'completed'
		for _, act := range complaint.Activities {
			if act.Status == domain.ActivityStatusUserRating {
				// If it's after the last completion (or there was no completion activity recorded)
				if act.CreatedDate.After(lastCompletedAt) {
					var rating domain.ComplaintRating
					if err := json.Unmarshal([]byte(act.Description), &rating); err == nil {
						complaint.CurrentRating = &rating
						complaint.HasRatedThisRound = true
						break
					}
				}
			}
		}
	}

	return complaint, nil
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

func (u *complaintUseCase) RateComplaint(id string, userID string, rating int, comment string) error {
	complaint, err := u.repo.GetByID(id, userID)
	if err != nil {
		return fmt.Errorf("complaint not found")
	}

	if complaint.Status != domain.ComplaintStatusCompleted {
		return fmt.Errorf("can only rate a completed complaint")
	}

	// 1. Get completer info for assignee_id and department_id
	assigneeID, departmentID, _ := u.repo.GetCompleterInfo(id)

	// 2. Create history record
	history := &domain.ComplaintRatingHistory{
		ID:                uuid.New().String(),
		ModuleComplaintId: id,
		AssigneeId:        assigneeID,
		DepartmentId:      departmentID,
		RatingScore:       &rating,
		IsDisputed:        false,
		CreatedDate:       time.Now(),
		UpdatedDate:       time.Now(),
		CreatedBy:         userID,
		UpdatedBy:         userID,
	}
	if err := u.repo.CreateRatingHistory(history); err != nil {
		return err
	}

	// 3. Reset active dispute flag on the main complaint
	complaint.IsDisputed = false
	if err := u.repo.Update(complaint); err != nil {
		return err
	}

	descBytes, _ := json.Marshal(domain.ComplaintRating{Rating: rating, Comment: comment})

	activity := &domain.ComplaintActivity{
		ID:                uuid.New().String(),
		ModuleComplaintId: id,
		Description:       string(descBytes),
		Status:            domain.ActivityStatusUserRating,
		CreatedBy:         userID,
		UpdatedBy:         userID,
		CreatedDate:       time.Now(),
		UpdatedDate:       time.Now(),
	}

	return u.repo.CreateActivity(activity)
}

func (u *complaintUseCase) DisputeComplaint(id string, userID string, reason string) error {
	complaint, err := u.repo.GetByID(id, userID)
	if err != nil {
		return fmt.Errorf("complaint not found")
	}

	if complaint.Status != domain.ComplaintStatusCompleted {
		return fmt.Errorf("can only dispute a completed complaint")
	}

	// 1. Get completer info for assignee_id and department_id (the person who failed to complete the job)
	assigneeID, departmentID, _ := u.repo.GetCompleterInfo(id)

	// 2. Create history record
	history := &domain.ComplaintRatingHistory{
		ID:                uuid.New().String(),
		ModuleComplaintId: id,
		AssigneeId:        assigneeID,
		DepartmentId:      departmentID,
		RatingScore:       nil,
		IsDisputed:        true,
		CreatedDate:       time.Now(),
		UpdatedDate:       time.Now(),
		CreatedBy:         userID,
		UpdatedBy:         userID,
	}
	if err := u.repo.CreateRatingHistory(history); err != nil {
		return err
	}

	activity := &domain.ComplaintActivity{
		ID:                uuid.New().String(),
		ModuleComplaintId: id,
		Description:       reason,
		Status:            domain.ActivityStatusDisputeRequest,
		CreatedBy:         userID,
		UpdatedBy:         userID,
		CreatedDate:       time.Now(),
		UpdatedDate:       time.Now(),
	}

	if err := u.repo.CreateActivity(activity); err != nil {
		return err
	}

	// Reset assignee and set status back to pending
	complaint.AssigneeId = nil
	complaint.Status = domain.ComplaintStatusPending
	complaint.UpdatedDate = time.Now()
	complaint.UpdatedBy = userID
	complaint.IsDisputed = true // Mark as disputed directly in db to flag current active dispute state

	return u.repo.Update(complaint)
}

func generateDocumentID() string {
	now := time.Now()
	rand.Seed(time.Now().UnixNano())
	randomPart := rand.Intn(9999)
	return fmt.Sprintf("CP-%s-%04d", now.Format("20060102"), randomPart)
}

