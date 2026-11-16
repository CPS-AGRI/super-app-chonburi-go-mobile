package usecase

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"super-app-chonburi-go-mobile/internal/domain"
	"time"

	"github.com/google/uuid"
)

var ErrMinimumComplaintImages = errors.New("กรุณาแนบรูปภาพประกอบอย่างน้อย 2 รูป")

type complaintUseCase struct {
	repo       domain.ComplaintRepository
	notifyUser func(userID uuid.UUID, title, body, refID, refStatus, notifType string)
	notifyDept func(departmentID string, role string, title, body, refID, refStatus string)
}

func NewComplaintUseCase(repo domain.ComplaintRepository) domain.ComplaintUseCase {
	return &complaintUseCase{
		repo:       repo,
		notifyUser: SendNotificationToUser,
		notifyDept: SendNotificationToDepartment,
	}
}

func NewComplaintUseCaseWithNotifiers(
	repo domain.ComplaintRepository,
	notifyUser func(userID uuid.UUID, title, body, refID, refStatus, notifType string),
	notifyDept func(departmentID string, role string, title, body, refID, refStatus string),
) domain.ComplaintUseCase {
	return &complaintUseCase{
		repo:       repo,
		notifyUser: notifyUser,
		notifyDept: notifyDept,
	}
}

func (u *complaintUseCase) sendPendingComplaintNotifications(complaint *domain.Complaint) {
	if complaint == nil || complaint.Status == domain.ComplaintStatusDraft {
		return
	}

	if u.notifyUser != nil {
		if parsedUserUUID, err := uuid.Parse(complaint.UserId); err == nil {
			u.notifyUser(
				parsedUserUUID,
				"ยื่นคำร้องร้องเรียนสำเร็จ",
				fmt.Sprintf("เราได้รับเรื่องร้องเรียนเลขที่ %s เรียบร้อยแล้ว (สถานะ: รอดำเนินการ)", complaint.DocumentId),
				complaint.ID,
				"pending",
				"complaint",
			)
		}
	}

	if u.notifyDept != nil {
		deptID := ""
		if complaint.DepartmentId != nil {
			deptID = *complaint.DepartmentId
		}
		u.notifyDept(
			deptID,
			"Managers",
			"มีเรื่องร้องเรียนใหม่ส่งเข้ามา",
			fmt.Sprintf("เรื่องร้องเรียนเลขที่ %s เรื่อง: %s รอนุมัติรับเรื่องและส่งต่อทำงาน", complaint.DocumentId, complaint.Description),
			complaint.ID,
			"pending",
		)
	}
}

func (u *complaintUseCase) AddComplaint(complaint *domain.Complaint, imageURLs []string) error {
	complaint.ID = uuid.New().String()
	complaint.DocumentId = generateDocumentID()
	complaint.CreatedDate = time.Now()
	complaint.UpdatedDate = time.Now()

	if complaint.Status != domain.ComplaintStatusDraft {
		complaint.Status = domain.ComplaintStatusPending
		if len(imageURLs) < 2 {
			return ErrMinimumComplaintImages
		}
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

	if complaint.Status != domain.ComplaintStatusDraft {
		u.sendPendingComplaintNotifications(complaint)
	}

	return nil
}

func (u *complaintUseCase) UpdateComplaint(complaint *domain.Complaint, imageURLs []string) error {

	existing, err := u.repo.GetByID(complaint.ID, complaint.UserId)
	if err != nil {
		return fmt.Errorf("complaint not found or unauthorized")
	}

	if existing.IsDisputed {
		return fmt.Errorf("เรื่องร้องเรียนนี้มีการดำเนินการและตีกลับแก้ไขแล้ว ไม่สามารถแก้ไขได้")
	}

	for _, act := range existing.Activities {
		if act.Status == domain.ComplaintStatusInProgress || act.Status == domain.ComplaintStatusCompleted || act.Status == domain.ActivityStatusDisputeRequest {
			return fmt.Errorf("เรื่องร้องเรียนนี้มีประวัติการดำเนินการแล้ว ไม่สามารถแก้ไขได้")
		}
	}

	if existing.Status != domain.ComplaintStatusDraft && existing.Status != domain.ComplaintStatusPending {
		return fmt.Errorf("สามารถแก้ไขได้เฉพาะเรื่องร้องเรียนที่อยู่ในสถานะแบบร่าง หรือรอดำเนินการเท่านั้น")
	}

	wasDraft := existing.Status == domain.ComplaintStatusDraft

	existing.Description = complaint.Description
	existing.ModuleTypeId = complaint.ModuleTypeId
	existing.Latitude = complaint.Latitude
	existing.Longitude = complaint.Longitude
	if complaint.Status == domain.ComplaintStatusDraft || complaint.Status == domain.ComplaintStatusPending {
		existing.Status = complaint.Status
	} else if existing.Status != domain.ComplaintStatusDraft {
		existing.Status = domain.ComplaintStatusPending
	}
	if existing.Status == domain.ComplaintStatusPending && len(imageURLs) < 2 {
		return ErrMinimumComplaintImages
	}
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

	if err := u.repo.Update(existing); err != nil {
		return err
	}

	// Only notify when a draft is officially submitted (draft -> pending); never notify for drafts.
	if wasDraft && existing.Status == domain.ComplaintStatusPending {
		u.sendPendingComplaintNotifications(existing)
	}

	return nil
}

func (u *complaintUseCase) GetMyComplaints(userID string, status string, search string, page int, limit int) ([]domain.Complaint, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
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

	existing, err := u.repo.GetByID(id, userID)
	if err != nil {
		return fmt.Errorf("complaint not found or unauthorized")
	}

	if existing.IsDisputed {
		return fmt.Errorf("เรื่องร้องเรียนนี้มีการดำเนินการและตีกลับแก้ไขแล้ว ไม่สามารถยกเลิกหรือลบได้")
	}

	for _, act := range existing.Activities {
		if act.Status == domain.ComplaintStatusInProgress || act.Status == domain.ComplaintStatusCompleted || act.Status == domain.ActivityStatusDisputeRequest {
			return fmt.Errorf("เรื่องร้องเรียนนี้มีประวัติการดำเนินการแล้ว ไม่สามารถยกเลิกหรือลบได้")
		}
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
		"Managers",
		"คำร้องเรียนได้รับข้อพิพาทใหม่",
		fmt.Sprintf("ผู้ยื่นเรื่องคำร้องเรียนเลขที่ %s ได้ส่งยื่นอุทธรณ์/ข้อพิพาท: %s", complaint.DocumentId, reason),
		complaint.ID,
		"disputed",
	)

	return nil
}

func generateDocumentID() string {
	now := time.Now()
	nBig, err := rand.Int(rand.Reader, big.NewInt(1000000))
	var randomPart int64
	if err == nil && nBig != nil {
		randomPart = nBig.Int64()
	} else {
		randomPart = now.UnixNano() % 1000000
	}
	return fmt.Sprintf("CP-%s-%06d", now.Format("20060102"), randomPart)
}
