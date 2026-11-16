package usecase

import (
	"testing"

	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubPRRepo struct {
	comment        *domain.PublicRelationComment
	reportChanged  bool
	reportCalls    int
	lastReporterID string
	newsTitle      string
	newsAdminID    uuid.UUID
	newsDeptID     string
}

func (s *stubPRRepo) GetPaginated(moduleId string, page int, limit int, userId string) ([]domain.PublicRelation, error) {
	return nil, nil
}
func (s *stubPRRepo) GetByID(moduleId string, id string) (*domain.PublicRelation, error) {
	return nil, nil
}
func (s *stubPRRepo) IncrementVisitorCount(prId string) error { return nil }
func (s *stubPRRepo) GetLike(prId string, userId string) (*domain.PublicRelationLike, error) {
	return nil, nil
}
func (s *stubPRRepo) ToggleLike(prId string, userId string) (bool, error) { return false, nil }
func (s *stubPRRepo) CreateComment(comment *domain.PublicRelationComment) error {
	return nil
}
func (s *stubPRRepo) GetComments(prId string) ([]domain.PublicRelationComment, error) {
	return nil, nil
}
func (s *stubPRRepo) GetCommentByID(commentId string) (*domain.PublicRelationComment, error) {
	return s.comment, nil
}
func (s *stubPRRepo) GetNewsModerationTarget(prId string) (string, uuid.UUID, string, error) {
	return s.newsTitle, s.newsAdminID, s.newsDeptID, nil
}
func (s *stubPRRepo) DeleteComment(commentId string, userId string) error { return nil }
func (s *stubPRRepo) ReportComment(commentId string, reporterUserID string) (bool, error) {
	s.reportCalls++
	s.lastReporterID = reporterUserID
	return s.reportChanged, nil
}
func (s *stubPRRepo) HideComment(commentId string, userId string) error { return nil }
func (s *stubPRRepo) GetActiveWelcomeScreen() (*domain.MunicipalityWelcomeScreen, error) {
	return nil, nil
}

func TestMobileReportComment_ValidationAndNotifications(t *testing.T) {
	ownerID := uuid.New()
	reporterID := uuid.New()
	newsID := uuid.New()
	commentID := uuid.New()
	deptID := uuid.New().String()

	t.Run("rejects invalid ids", func(t *testing.T) {
		uc := NewPublicRelationMobileUseCase(&stubPRRepo{})
		err := uc.ReportComment("bad-uuid", reporterID.String())
		assert.ErrorIs(t, err, domain.ErrInvalidCommentInput)
	})

	t.Run("returns not found when missing", func(t *testing.T) {
		uc := NewPublicRelationMobileUseCase(&stubPRRepo{comment: nil})
		err := uc.ReportComment(commentID.String(), reporterID.String())
		assert.ErrorIs(t, err, domain.ErrCommentNotFound)
	})

	t.Run("forbids reporting own comment", func(t *testing.T) {
		uc := NewPublicRelationMobileUseCase(&stubPRRepo{
			comment: &domain.PublicRelationComment{ID: commentID, UserId: ownerID, Status: "active"},
		})
		err := uc.ReportComment(commentID.String(), ownerID.String())
		assert.ErrorIs(t, err, domain.ErrCannotReportOwnComment)
	})

	t.Run("does not revert hidden or duplicate report", func(t *testing.T) {
		for _, status := range []string{"hidden", "reported"} {
			repo := &stubPRRepo{
				comment: &domain.PublicRelationComment{ID: commentID, UserId: ownerID, Status: status},
			}
			notified := 0
			uc := &publicRelationMobileUseCase{
				repo:       repo,
				notifyDept: func(_, _, _, _, _, _, _ string) { notified++ },
			}
			err := uc.ReportComment(commentID.String(), reporterID.String())
			require.NoError(t, err)
			assert.Equal(t, 0, repo.reportCalls)
			assert.Equal(t, 0, notified)
		}
	})

	t.Run("reports active comment and notifies PR department", func(t *testing.T) {
		repo := &stubPRRepo{
			comment: &domain.PublicRelationComment{
				ID:                     commentID,
				ModulePublicRelationId: newsID,
				UserId:                 ownerID,
				Comment:                "ข้อความไม่เหมาะสม",
				Status:                 "active",
			},
			reportChanged: true,
			newsTitle:     "ข่าวทดสอบ",
			newsDeptID:    deptID,
		}
		var notifTypes []string
		var notifRoles []string
		uc := &publicRelationMobileUseCase{
			repo: repo,
			notifyDept: func(notifType, departmentID, role, title, body, refID, refStatus string) {
				notifTypes = append(notifTypes, notifType)
				notifRoles = append(notifRoles, role)
				assert.Equal(t, deptID, departmentID)
				assert.Equal(t, newsID.String(), refID)
				assert.Equal(t, "reported", refStatus)
			},
		}
		err := uc.ReportComment(commentID.String(), reporterID.String())
		require.NoError(t, err)
		assert.Equal(t, 1, repo.reportCalls)
		assert.Equal(t, reporterID.String(), repo.lastReporterID)
		assert.Equal(t, []string{"pr", "pr"}, notifTypes)
		assert.Equal(t, []string{"Managers", "Employees"}, notifRoles)
	})
}

