package usecase

import (
	"errors"
	"fmt"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/safe"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	newsViewCache       sync.Map // key -> time.Time
	newsViewCooldown    = 30 * time.Minute
	newsViewCleanupOnce sync.Once
)

type publicRelationMobileUseCase struct {
	repo        domain.PublicRelationMobileRepository
	notifyDept  func(notifType, departmentID, role, title, body, refID, refStatus string)
	notifyAdmin func(adminID uuid.UUID, title, body, refID, refStatus, notifType string)
}

func NewPublicRelationMobileUseCase(repo domain.PublicRelationMobileRepository) domain.PublicRelationMobileUseCase {
	newsViewCleanupOnce.Do(func() {
		safe.Go(func() {
			ticker := time.NewTicker(10 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				now := time.Now()
				newsViewCache.Range(func(key, value interface{}) bool {
					if lastTime, ok := value.(time.Time); ok {
						if now.Sub(lastTime) >= newsViewCooldown {
							newsViewCache.Delete(key)
						}
					} else {
						newsViewCache.Delete(key)
					}
					return true
				})
			}
		})
	})
	return &publicRelationMobileUseCase{
		repo:        repo,
		notifyDept:  SendNotificationToDepartmentWithModule,
		notifyAdmin: SendNotificationToAdmin,
	}
}

func (u *publicRelationMobileUseCase) GetNewsFeed(moduleId string, page int, limit int, userId string) ([]domain.PublicRelation, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	return u.repo.GetPaginated(moduleId, page, limit, userId)
}

func (u *publicRelationMobileUseCase) GetNewsDetail(moduleId string, id string, userId string) (*domain.PublicRelation, bool, error) {
	// Idempotent GET - Side effects removed to prevent inflated/duplicate counts
	pr, err := u.repo.GetByID(moduleId, id)
	if err != nil {
		return nil, false, err
	}
	if pr == nil {
		return nil, false, errors.New("news not found")
	}

	liked := false
	if userId != "" {
		like, err := u.repo.GetLike(id, userId)
		if err == nil && like != nil {
			liked = true
		}
	}

	return pr, liked, nil
}

func (u *publicRelationMobileUseCase) RecordNewsView(prId string, userId string, sessionId string) error {
	if _, err := uuid.Parse(prId); err != nil {
		return fmt.Errorf("invalid news id: %w", err)
	}
	identifier := userId
	if identifier == "" {
		identifier = sessionId
	}
	if identifier == "" {
		identifier = "anon"
	}

	cacheKey := prId + ":" + identifier
	now := time.Now()

	if lastTimeVal, ok := newsViewCache.Load(cacheKey); ok {
		if lastTime, ok := lastTimeVal.(time.Time); ok {
			if now.Sub(lastTime) < newsViewCooldown {
				// Cooldown active, duplicate skipped (0 DB writes)
				return nil
			}
		}
	}

	newsViewCache.Store(cacheKey, now)
	return u.repo.IncrementVisitorCount(prId)
}

func (u *publicRelationMobileUseCase) ToggleLike(prId string, userId string) (bool, error) {
	if _, err := uuid.Parse(prId); err != nil {
		return false, fmt.Errorf("invalid news id: %w", err)
	}
	if _, err := uuid.Parse(userId); err != nil {
		return false, fmt.Errorf("invalid user id: %w", err)
	}
	return u.repo.ToggleLike(prId, userId)
}

func (u *publicRelationMobileUseCase) AddComment(moduleId string, prId string, userId string, commentText string) (*domain.PublicRelationComment, error) {
	parsedPrID, err := uuid.Parse(prId)
	if err != nil {
		return nil, fmt.Errorf("invalid news id: %w", err)
	}
	parsedUserID, err := uuid.Parse(userId)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}

	comment := &domain.PublicRelationComment{
		ID:                     uuid.New(),
		ModulePublicRelationId: parsedPrID,
		UserId:                 parsedUserID,
		Comment:                commentText,
		Status:                 "active",
		CreatedDate:            time.Now(),
		UpdatedDate:            time.Now(),
		CreatedBy:              "mobile_user",
		UpdatedBy:              "mobile_user",
	}

	if err := u.repo.CreateComment(comment); err != nil {
		return nil, err
	}

	return comment, nil
}

func sanitizeCommentPII(c *domain.PublicRelationComment) {
	if c == nil {
		return
	}
	if c.User != nil {
		var safeInfo *domain.UserInformation
		if c.User.Information != nil {
			safeInfo = &domain.UserInformation{
				UserId:   c.User.Information.UserId,
				Prefix:   c.User.Information.Prefix,
				Name:     c.User.Information.Name,
				LastName: c.User.Information.LastName,
				LogoUrl:  c.User.Information.LogoUrl,
			}
		}
		c.User = &domain.AppUser{
			ID:              c.User.ID,
			ImageProfileUrl: c.User.ImageProfileUrl,
			Information:     safeInfo,
		}
	}
	if c.Information != nil {
		c.Information = &domain.UserInformation{
			UserId:   c.Information.UserId,
			Prefix:   c.Information.Prefix,
			Name:     c.Information.Name,
			LastName: c.Information.LastName,
			LogoUrl:  c.Information.LogoUrl,
		}
	}
}

func (u *publicRelationMobileUseCase) GetComments(prId string) ([]domain.PublicRelationComment, error) {
	if _, err := uuid.Parse(prId); err != nil {
		return nil, fmt.Errorf("invalid news id: %w", err)
	}
	comments, err := u.repo.GetComments(prId)
	if err != nil {
		return nil, err
	}
	for i := range comments {
		sanitizeCommentPII(&comments[i])
	}
	return comments, nil
}

func (u *publicRelationMobileUseCase) DeleteComment(commentId string, userId string) error {
	if _, err := uuid.Parse(commentId); err != nil {
		return fmt.Errorf("invalid comment id: %w", err)
	}
	if _, err := uuid.Parse(userId); err != nil {
		return fmt.Errorf("invalid user id: %w", err)
	}
	return u.repo.DeleteComment(commentId, userId)
}

func (u *publicRelationMobileUseCase) ReportComment(commentId string, reporterUserID string) error {
	if _, err := uuid.Parse(commentId); err != nil {
		return fmt.Errorf("%w: invalid comment id", domain.ErrInvalidCommentInput)
	}
	if _, err := uuid.Parse(reporterUserID); err != nil {
		return fmt.Errorf("%w: invalid user id", domain.ErrInvalidCommentInput)
	}

	comment, err := u.repo.GetCommentByID(commentId)
	if err != nil {
		return err
	}
	if comment == nil {
		return domain.ErrCommentNotFound
	}
	if comment.UserId.String() == reporterUserID {
		return domain.ErrCannotReportOwnComment
	}
	// Do not revert already-hidden comments back to reported or emit duplicate alerts.
	if comment.Status != "active" {
		return nil
	}

	changed, err := u.repo.ReportComment(commentId, reporterUserID)
	if err != nil {
		return err
	}
	if changed {
		u.notifyPRReport(comment)
	}
	return nil
}

func (u *publicRelationMobileUseCase) notifyPRReport(comment *domain.PublicRelationComment) {
	if comment == nil {
		return
	}
	newsTitle, adminID, deptID, _ := u.repo.GetNewsModerationTarget(comment.ModulePublicRelationId.String())
	if newsTitle == "" {
		newsTitle = "ข่าวประชาสัมพันธ์"
	}
	body := fmt.Sprintf("มีรายงานความคิดเห็นไม่เหมาะสมในข่าว \"%s\": %s", newsTitle, comment.Comment)
	refID := comment.ModulePublicRelationId.String()

	if deptID != "" && u.notifyDept != nil {
		u.notifyDept("pr", deptID, "Managers", "มีรายงานความคิดเห็นไม่เหมาะสม", body, refID, "reported")
		u.notifyDept("pr", deptID, "Employees", "มีรายงานความคิดเห็นไม่เหมาะสม", body, refID, "reported")
	} else if adminID != uuid.Nil && u.notifyAdmin != nil {
		u.notifyAdmin(adminID, "มีรายงานความคิดเห็นไม่เหมาะสม", body, refID, "reported", "pr")
	} else if u.notifyDept != nil {
		u.notifyDept("pr", "", "Managers", "มีรายงานความคิดเห็นไม่เหมาะสม", body, refID, "reported")
	}
}

func (u *publicRelationMobileUseCase) HideComment(commentId string, userId string) error {
	if _, err := uuid.Parse(commentId); err != nil {
		return fmt.Errorf("invalid comment id: %w", err)
	}
	if _, err := uuid.Parse(userId); err != nil {
		return fmt.Errorf("invalid user id: %w", err)
	}
	return u.repo.HideComment(commentId, userId)
}

func (u *publicRelationMobileUseCase) GetWelcomeScreen() (*domain.MunicipalityWelcomeScreen, error) {
	return u.repo.GetActiveWelcomeScreen()
}
