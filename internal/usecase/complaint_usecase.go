package usecase

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

	if complaint.Status != domain.ComplaintStatusDraft {
		complaint.Status = domain.ComplaintStatusPending
	}

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

	err := u.repo.Create(complaint)
	if err != nil {
		return err
	}

	if parsedUserUUID, err := uuid.Parse(complaint.UserId); err == nil {
		SendNotificationToUser(
			parsedUserUUID,
			"ยื่นคำร้องร้องเรียนสำเร็จ",
			fmt.Sprintf("เราได้รับเรื่องร้องเรียนเลขที่ %s เรียบร้อยแล้ว (สถานะ: รอดำเนินการ)", complaint.DocumentId),
			complaint.ID,
			"pending",
			"complaint",
		)
	}

	deptID := ""
	if complaint.DepartmentId != nil {
		deptID = *complaint.DepartmentId
	}
	SendNotificationToDepartment(
		deptID,
		"",
		"มีเรื่องร้องเรียนใหม่ส่งเข้ามา",
		fmt.Sprintf("เรื่องร้องเรียนเลขที่ %s เรื่อง: %s รอนุมัติรับเรื่องและส่งต่อทำงาน", complaint.DocumentId, complaint.Description),
		complaint.ID,
		"pending",
	)

	return nil
}

func (u *complaintUseCase) UpdateComplaint(complaint *domain.Complaint, imageURLs []string) error {

	existing, err := u.repo.GetByID(complaint.ID, complaint.UserId)
	if err != nil {
		return fmt.Errorf("complaint not found or unauthorized")
	}

	existing.Description = complaint.Description
	existing.ModuleTypeId = complaint.ModuleTypeId
	existing.Latitude = complaint.Latitude
	existing.Longitude = complaint.Longitude
	existing.Status = complaint.Status
	existing.UpdatedDate = time.Now()
	existing.UpdatedBy = complaint.UserId

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

	if complaint.Status == domain.ComplaintStatusCompleted {

		var lastCompletedAt time.Time
		for _, act := range complaint.Activities {
			if act.Status == domain.ComplaintStatusCompleted {
				if act.CreatedDate.After(lastCompletedAt) {
					lastCompletedAt = act.CreatedDate
				}
			}
		}

		for _, act := range complaint.Activities {
			if act.Status == domain.ActivityStatusUserRating {

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

	assigneeID, departmentID, _ := u.repo.GetCompleterInfo(id)

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

func (u *complaintUseCase) DisputeComplaint(id string, userID string, reason string, images []string) error {
	complaint, err := u.repo.GetByID(id, userID)
	if err != nil {
		return fmt.Errorf("complaint not found")
	}

	if complaint.Status != domain.ComplaintStatusCompleted {
		return fmt.Errorf("can only dispute a completed complaint")
	}

	assigneeID, departmentID, _ := u.repo.GetCompleterInfo(id)

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

	mode, modeErr := u.repo.GetComplaintMode()
	if modeErr == nil && mode == "central" {
		complaint.DepartmentId = nil
	}

	complaint.AssigneeId = nil
	complaint.Status = domain.ComplaintStatusPending
	complaint.UpdatedDate = time.Now()
	complaint.IsDisputed = true

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

	for i, imgUrl := range images {
		activity.Images = append(activity.Images, domain.ComplaintActivityImage{
			ID:                        uuid.New().String(),
			ModuleComplaintActivityId: activity.ID,
			Url:                       imgUrl,
			Sequence:                  i + 1,
			CreatedBy:                 userID,
			UpdatedBy:                 userID,
			CreatedDate:               time.Now(),
			UpdatedDate:               time.Now(),
		})
	}

	if err := u.repo.CreateActivity(activity); err != nil {
		return err
	}

	err = u.repo.Update(complaint)
	if err != nil {
		return err
	}

	deptID := ""
	if complaint.DepartmentId != nil {
		deptID = *complaint.DepartmentId
	}
	SendNotificationToDepartment(
		deptID,
		"",
		"คำร้องเรียนได้รับข้อพิพาทใหม่",
		fmt.Sprintf("ผู้ยื่นเรื่องคำร้องเรียนเลขที่ %s ได้ส่งยื่นอุทธรณ์/ข้อพิพาท: %s", complaint.DocumentId, reason),
		complaint.ID,
		"disputed",
	)

	return nil
}

func generateDocumentID() string {
	now := time.Now()
	rand.Seed(time.Now().UnixNano())
	randomPart := rand.Intn(9999)
	return fmt.Sprintf("CP-%s-%04d", now.Format("20060102"), randomPart)
}
