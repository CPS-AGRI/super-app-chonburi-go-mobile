package repository

import (
	"errors"
	"super-app-chonburi-go-mobile/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type publicRelationMobileRepository struct {
	db *gorm.DB
}

func NewPublicRelationMobileRepository(db *gorm.DB) domain.PublicRelationMobileRepository {
	return &publicRelationMobileRepository{db: db}
}

func (r *publicRelationMobileRepository) GetPaginated(moduleId string, page int, limit int, userId string) ([]domain.PublicRelation, error) {
	var prs []domain.PublicRelation
	offset := (page - 1) * limit
	now := time.Now()

	err := r.db.Preload("Images").
		Where("module_id = ? AND status = ? AND start_date <= ? AND end_date >= ?", moduleId, "Published", now, now).
		Offset(offset).
		Limit(limit).
		Order("created_date DESC").
		Find(&prs).Error

	if err != nil {
		return nil, err
	}

	// Compute Likes, Comments count, and Liked status for each news item
	for i := range prs {
		r.db.Model(&domain.PublicRelationLike{}).Where("module_public_relation_id = ?", prs[i].ID).Count(&prs[i].LikesCount)
		r.db.Model(&domain.PublicRelationComment{}).Where("module_public_relation_id = ? AND status != ?", prs[i].ID, "hidden").Count(&prs[i].CommentsCount)
		
		var vc domain.PublicRelationVisitorCount
		if r.db.Where("module_public_relation_id = ?", prs[i].ID).First(&vc).Error == nil {
			prs[i].ViewCount = vc.Count
		}

		if userId != "" {
			var count int64
			r.db.Model(&domain.PublicRelationLike{}).Where("module_public_relation_id = ? AND user_id = ?", prs[i].ID, userId).Count(&count)
			prs[i].Liked = count > 0
		}
	}

	return prs, nil
}

func (r *publicRelationMobileRepository) GetByID(moduleId string, id string) (*domain.PublicRelation, error) {
	var pr domain.PublicRelation
	err := r.db.Preload("Images").
		Where("module_id = ? AND id = ?", moduleId, id).
		First(&pr).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	r.db.Model(&domain.PublicRelationLike{}).Where("module_public_relation_id = ?", pr.ID).Count(&pr.LikesCount)
	r.db.Model(&domain.PublicRelationComment{}).Where("module_public_relation_id = ? AND status != ?", pr.ID, "hidden").Count(&pr.CommentsCount)

	var vc domain.PublicRelationVisitorCount
	if r.db.Where("module_public_relation_id = ?", pr.ID).First(&vc).Error == nil {
		pr.ViewCount = vc.Count
	}

	return &pr, nil
}

func (r *publicRelationMobileRepository) IncrementVisitorCount(prId string) error {
	return r.db.Model(&domain.PublicRelationVisitorCount{}).
		Where("module_public_relation_id = ?", prId).
		UpdateColumn("count", gorm.Expr("count + 1")).Error
}

func (r *publicRelationMobileRepository) GetLike(prId string, userId string) (*domain.PublicRelationLike, error) {
	var like domain.PublicRelationLike
	err := r.db.Where("module_public_relation_id = ? AND user_id = ?", prId, userId).First(&like).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &like, nil
}

func (r *publicRelationMobileRepository) ToggleLike(prId string, userId string) (bool, error) {
	var like domain.PublicRelationLike
	err := r.db.Where("module_public_relation_id = ? AND user_id = ?", prId, userId).First(&like).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Like it
			newLike := domain.PublicRelationLike{
				ModulePublicRelationId: uuid.MustParse(prId),
				UserId:                 uuid.MustParse(userId),
				CreatedDate:            time.Now(),
				UpdatedDate:            time.Now(),
				CreatedBy:              "mobile_user",
				UpdatedBy:              "mobile_user",
			}
			if err := r.db.Create(&newLike).Error; err != nil {
				return false, err
			}
			return true, nil
		}
		return false, err
	}

	// Unlike it
	if err := r.db.Delete(&like).Error; err != nil {
		return false, err
	}
	return false, nil
}

func (r *publicRelationMobileRepository) CreateComment(comment *domain.PublicRelationComment) error {
	return r.db.Create(comment).Error
}

func (r *publicRelationMobileRepository) GetComments(prId string) ([]domain.PublicRelationComment, error) {
	var comments []domain.PublicRelationComment
	err := r.db.Preload("User.Information").
		Where("module_public_relation_id = ? AND status != ?", prId, "hidden").
		Order("created_date DESC").
		Find(&comments).Error
	return comments, err
}

func (r *publicRelationMobileRepository) DeleteComment(commentId string, userId string) error {
	return r.db.Where("id = ? AND user_id = ?", commentId, userId).Delete(&domain.PublicRelationComment{}).Error
}

func (r *publicRelationMobileRepository) ReportComment(commentId string) error {
	return r.db.Model(&domain.PublicRelationComment{}).Where("id = ?", commentId).Update("status", "reported").Error
}

func (r *publicRelationMobileRepository) HideComment(commentId string) error {
	return r.db.Model(&domain.PublicRelationComment{}).Where("id = ?", commentId).Update("status", "hidden").Error
}

func (r *publicRelationMobileRepository) GetActiveWelcomeScreen() (*domain.MunicipalityWelcomeScreen, error) {
	var screen domain.MunicipalityWelcomeScreen
	err := r.db.Where("is_active = ?", true).Order("created_date DESC").First(&screen).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &screen, nil
}
