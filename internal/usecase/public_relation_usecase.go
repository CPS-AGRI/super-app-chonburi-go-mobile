package usecase

import (
	"errors"
	"super-app-chonburi-go-mobile/internal/domain"
	"time"

	"github.com/google/uuid"
)

type publicRelationMobileUseCase struct {
	repo domain.PublicRelationMobileRepository
}

func NewPublicRelationMobileUseCase(repo domain.PublicRelationMobileRepository) domain.PublicRelationMobileUseCase {
	return &publicRelationMobileUseCase{repo: repo}
}

func (u *publicRelationMobileUseCase) GetNewsFeed(moduleId string, page int, limit int, userId string) ([]domain.PublicRelation, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	return u.repo.GetPaginated(moduleId, page, limit, userId)
}

func (u *publicRelationMobileUseCase) GetNewsDetail(moduleId string, id string, userId string) (*domain.PublicRelation, bool, error) {
	// Increment visitor count asynchronously (best effort)
	go func() {
		_ = u.repo.IncrementVisitorCount(id)
	}()

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

func (u *publicRelationMobileUseCase) ToggleLike(prId string, userId string) (bool, error) {
	return u.repo.ToggleLike(prId, userId)
}

func (u *publicRelationMobileUseCase) AddComment(moduleId string, prId string, userId string, commentText string) (*domain.PublicRelationComment, error) {
	comment := &domain.PublicRelationComment{
		ID:                     uuid.New(),
		ModulePublicRelationId: uuid.MustParse(prId),
		UserId:                 uuid.MustParse(userId),
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

func (u *publicRelationMobileUseCase) GetComments(prId string) ([]domain.PublicRelationComment, error) {
	return u.repo.GetComments(prId)
}

func (u *publicRelationMobileUseCase) DeleteComment(commentId string, userId string) error {
	return u.repo.DeleteComment(commentId, userId)
}

func (u *publicRelationMobileUseCase) ReportComment(commentId string) error {
	return u.repo.ReportComment(commentId)
}

func (u *publicRelationMobileUseCase) HideComment(commentId string) error {
	return u.repo.HideComment(commentId)
}

func (u *publicRelationMobileUseCase) GetWelcomeScreen() (*domain.MunicipalityWelcomeScreen, error) {
	return u.repo.GetActiveWelcomeScreen()
}
