package usecase_test

import (
	"testing"

	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

type mockComplaintRepo struct {
	domain.ComplaintRepository
	created *domain.Complaint
	updated *domain.Complaint
	stored  map[string]*domain.Complaint
}

func (m *mockComplaintRepo) Create(complaint *domain.Complaint) error {
	m.created = complaint
	if m.stored == nil {
		m.stored = make(map[string]*domain.Complaint)
	}
	copyVal := *complaint
	m.stored[complaint.ID] = &copyVal
	return nil
}
func (m *mockComplaintRepo) GetByUserID(userID string, status string, search string, page int, limit int) ([]domain.Complaint, error) {
	return nil, nil
}
func (m *mockComplaintRepo) GetByID(id string, userID string) (*domain.Complaint, error) {
	if m.stored != nil {
		if c, ok := m.stored[id]; ok {
			copyVal := *c
			return &copyVal, nil
		}
	}
	return nil, nil
}
func (m *mockComplaintRepo) Update(complaint *domain.Complaint) error {
	m.updated = complaint
	if m.stored == nil {
		m.stored = make(map[string]*domain.Complaint)
	}
	copyVal := *complaint
	m.stored[complaint.ID] = &copyVal
	return nil
}
func (m *mockComplaintRepo) UpdateStatus(id string, status string) error     { return nil }
func (m *mockComplaintRepo) Delete(id string) error                          { return nil }
func (m *mockComplaintRepo) GetFirstUserID() (string, error)                 { return "", nil }
func (m *mockComplaintRepo) CreateActivity(activity *domain.ComplaintActivity) error {
	return nil
}
func (m *mockComplaintRepo) CreateRatingHistory(history *domain.ComplaintRatingHistory) error {
	return nil
}
func (m *mockComplaintRepo) GetCompleterInfo(complaintID string) (*string, *string, error) {
	return nil, nil, nil
}
func (m *mockComplaintRepo) GetComplaintMode() (string, error) {
	return "central", nil
}

func TestAddComplaint_MinimumTwoImagesForPending(t *testing.T) {
	repo := &mockComplaintRepo{}
	uc := usecase.NewComplaintUseCase(repo)

	// 1. Pending complaint with < 2 images must fail validation
	err := uc.AddComplaint(&domain.Complaint{
		UserId:       "user-1",
		ModuleTypeId: "type-1",
		Description:  "test issue",
		Status:       domain.ComplaintStatusPending,
	}, []string{"https://example.com/1.jpg"})
	assert.ErrorIs(t, err, usecase.ErrMinimumComplaintImages)
	assert.Nil(t, repo.created)

	// 2. Draft complaint can be saved with < 2 images
	err = uc.AddComplaint(&domain.Complaint{
		UserId:       "user-1",
		ModuleTypeId: "type-1",
		Description:  "draft issue",
		Status:       domain.ComplaintStatusDraft,
	}, []string{"https://example.com/1.jpg"})
	assert.NoError(t, err)
	assert.NotNil(t, repo.created)

	// 3. Pending complaint with >= 2 images succeeds (with no DB notifications in unit test)
	repo.created = nil
	err = uc.AddComplaint(&domain.Complaint{
		UserId:       "not-a-uuid",
		ModuleTypeId: "type-1",
		Description:  "valid issue",
		Status:       domain.ComplaintStatusPending,
	}, []string{"https://example.com/1.jpg", "https://example.com/2.jpg"})
	assert.NoError(t, err)
	assert.NotNil(t, repo.created)
	assert.Len(t, repo.created.Images, 2)
}

func TestComplaintNotifications_DraftVsPending(t *testing.T) {
	userUUID := uuid.New().String()
	userNotifCount := 0
	deptNotifCount := 0

	repo := &mockComplaintRepo{}
	uc := usecase.NewComplaintUseCaseWithNotifiers(
		repo,
		func(userID uuid.UUID, title, body, refID, refStatus, notifType string) {
			userNotifCount++
		},
		func(departmentID string, role string, title, body, refID, refStatus string) {
			deptNotifCount++
		},
	)

	// 1. Creating a draft complaint must NOT notify staff or user
	draft := &domain.Complaint{
		UserId:       userUUID,
		ModuleTypeId: "type-1",
		Description:  "draft complaint",
		Status:       domain.ComplaintStatusDraft,
	}
	err := uc.AddComplaint(draft, []string{"https://example.com/1.jpg"})
	assert.NoError(t, err)
	assert.Equal(t, 0, userNotifCount)
	assert.Equal(t, 0, deptNotifCount)

	// 2. Updating a draft while staying in draft must NOT notify
	err = uc.UpdateComplaint(&domain.Complaint{
		ID:           draft.ID,
		UserId:       userUUID,
		ModuleTypeId: "type-1",
		Description:  "updated draft",
		Status:       domain.ComplaintStatusDraft,
	}, []string{"https://example.com/1.jpg"})
	assert.NoError(t, err)
	assert.Equal(t, 0, userNotifCount)
	assert.Equal(t, 0, deptNotifCount)

	// 3. Submitting draft -> pending with < 2 images must fail and NOT notify
	err = uc.UpdateComplaint(&domain.Complaint{
		ID:           draft.ID,
		UserId:       userUUID,
		ModuleTypeId: "type-1",
		Description:  "submit draft too few images",
		Status:       domain.ComplaintStatusPending,
	}, []string{"https://example.com/1.jpg"})
	assert.ErrorIs(t, err, usecase.ErrMinimumComplaintImages)
	assert.Equal(t, 0, userNotifCount)
	assert.Equal(t, 0, deptNotifCount)

	// 4. Submitting draft -> pending with >= 2 images MUST notify user and department once
	err = uc.UpdateComplaint(&domain.Complaint{
		ID:           draft.ID,
		UserId:       userUUID,
		ModuleTypeId: "type-1",
		Description:  "submitted complaint",
		Status:       domain.ComplaintStatusPending,
	}, []string{"https://example.com/1.jpg", "https://example.com/2.jpg"})
	assert.NoError(t, err)
	assert.Equal(t, 1, userNotifCount)
	assert.Equal(t, 1, deptNotifCount)

	// 5. Editing an already-pending complaint must NOT resend notifications
	err = uc.UpdateComplaint(&domain.Complaint{
		ID:           draft.ID,
		UserId:       userUUID,
		ModuleTypeId: "type-1",
		Description:  "edited pending complaint",
		Status:       domain.ComplaintStatusPending,
	}, []string{"https://example.com/1.jpg", "https://example.com/2.jpg"})
	assert.NoError(t, err)
	assert.Equal(t, 1, userNotifCount)
	assert.Equal(t, 1, deptNotifCount)

	// 6. Creating a pending complaint directly MUST notify user and department
	err = uc.AddComplaint(&domain.Complaint{
		UserId:       userUUID,
		ModuleTypeId: "type-1",
		Description:  "direct pending complaint",
		Status:       domain.ComplaintStatusPending,
	}, []string{"https://example.com/1.jpg", "https://example.com/2.jpg"})
	assert.NoError(t, err)
	assert.Equal(t, 2, userNotifCount)
	assert.Equal(t, 2, deptNotifCount)
}
