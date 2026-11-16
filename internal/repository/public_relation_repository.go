package repository

import (
	"errors"
	"fmt"
	"super-app-chonburi-go-mobile/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type publicRelationMobileRepository struct {
	db *gorm.DB
}

func NewPublicRelationMobileRepository(db *gorm.DB) domain.PublicRelationMobileRepository {
	return &publicRelationMobileRepository{db: db}
}

func (r *publicRelationMobileRepository) GetPaginated(moduleId string, page int, limit int, userId string) ([]domain.PublicRelation, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	var prs []domain.PublicRelation
	offset := (page - 1) * limit

	err := r.db.Preload("Images").
		Where("module_id = ? AND status = ? AND start_date::date <= (CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Bangkok')::date AND end_date::date >= (CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Bangkok')::date", moduleId, "Published").
		Offset(offset).
		Limit(limit).
		Order("created_date DESC").
		Find(&prs).Error

	if err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return prs, nil
	}

	prIDs := make([]uuid.UUID, len(prs))
	for i := range prs {
		prIDs[i] = prs[i].ID
	}

	type countRow struct {
		PRID  uuid.UUID `gorm:"column:module_public_relation_id"`
		Count int64     `gorm:"column:cnt"`
	}

	// 1. Batch fetch LikesCount
	var likeCounts []countRow
	r.db.Model(&domain.PublicRelationLike{}).
		Select("module_public_relation_id, COUNT(*) AS cnt").
		Where("module_public_relation_id IN ?", prIDs).
		Group("module_public_relation_id").
		Scan(&likeCounts)
	likeMap := make(map[uuid.UUID]int64, len(likeCounts))
	for _, row := range likeCounts {
		likeMap[row.PRID] = row.Count
	}

	// 2. Batch fetch CommentsCount
	var commentCounts []countRow
	r.db.Model(&domain.PublicRelationComment{}).
		Select("module_public_relation_id, COUNT(*) AS cnt").
		Where("module_public_relation_id IN ? AND status != ?", prIDs, "hidden").
		Group("module_public_relation_id").
		Scan(&commentCounts)
	commentMap := make(map[uuid.UUID]int64, len(commentCounts))
	for _, row := range commentCounts {
		commentMap[row.PRID] = row.Count
	}

	// 3. Batch fetch VisitorCounts
	var vcs []domain.PublicRelationVisitorCount
	r.db.Where("module_public_relation_id IN ?", prIDs).Find(&vcs)
	viewMap := make(map[uuid.UUID]int, len(vcs))
	for _, vc := range vcs {
		viewMap[vc.ModulePublicRelationId] = vc.Count
	}

	// 4. Batch fetch User Likes
	likedMap := make(map[uuid.UUID]bool)
	if userId != "" {
		if _, parseErr := uuid.Parse(userId); parseErr == nil {
			var userLikes []domain.PublicRelationLike
			r.db.Select("module_public_relation_id").
				Where("module_public_relation_id IN ? AND user_id = ?", prIDs, userId).
				Find(&userLikes)
			for _, ul := range userLikes {
				likedMap[ul.ModulePublicRelationId] = true
			}
		}
	}

	for i := range prs {
		id := prs[i].ID
		prs[i].LikesCount = likeMap[id]
		prs[i].CommentsCount = commentMap[id]
		prs[i].ViewCount = viewMap[id]
		prs[i].Liked = likedMap[id]
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
	parsedPRID, err := uuid.Parse(prId)
	if err != nil {
		return fmt.Errorf("invalid public relation id: %w", err)
	}
	now := time.Now()
	vc := domain.PublicRelationVisitorCount{
		ModulePublicRelationId: parsedPRID,
		Count:                  1,
		CreatedDate:            now,
		UpdatedDate:            now,
		CreatedBy:              "mobile_user",
		UpdatedBy:              "mobile_user",
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "module_public_relation_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"count":        gorm.Expr("module_public_relation_visitor_count.count + 1"),
			"updated_date": now,
		}),
	}).Create(&vc).Error
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
	parsedPRID, err := uuid.Parse(prId)
	if err != nil {
		return false, fmt.Errorf("invalid public relation id: %w", err)
	}
	parsedUserID, err := uuid.Parse(userId)
	if err != nil {
		return false, fmt.Errorf("invalid user id: %w", err)
	}

	var like domain.PublicRelationLike
	err = r.db.Where("module_public_relation_id = ? AND user_id = ?", parsedPRID, parsedUserID).First(&like).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			newLike := domain.PublicRelationLike{
				ModulePublicRelationId: parsedPRID,
				UserId:                 parsedUserID,
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
	if err != nil {
		return nil, err
	}
	for i := range comments {
		if comments[i].User != nil {
			var safeInfo *domain.UserInformation
			if comments[i].User.Information != nil {
				safeInfo = &domain.UserInformation{
					UserId:   comments[i].User.Information.UserId,
					Prefix:   comments[i].User.Information.Prefix,
					Name:     comments[i].User.Information.Name,
					LastName: comments[i].User.Information.LastName,
					LogoUrl:  comments[i].User.Information.LogoUrl,
				}
			}
			comments[i].User = &domain.AppUser{
				ID:              comments[i].User.ID,
				ImageProfileUrl: comments[i].User.ImageProfileUrl,
				Information:     safeInfo,
			}
		}
		if comments[i].Information != nil {
			comments[i].Information = &domain.UserInformation{
				UserId:   comments[i].Information.UserId,
				Prefix:   comments[i].Information.Prefix,
				Name:     comments[i].Information.Name,
				LastName: comments[i].Information.LastName,
				LogoUrl:  comments[i].Information.LogoUrl,
			}
		}
	}
	return comments, nil
}

func (r *publicRelationMobileRepository) GetCommentByID(commentId string) (*domain.PublicRelationComment, error) {
	var comment domain.PublicRelationComment
	err := r.db.Where("id = ?", commentId).First(&comment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &comment, nil
}

func (r *publicRelationMobileRepository) GetNewsModerationTarget(prId string) (string, uuid.UUID, string, error) {
	type newsTarget struct {
		Title        string    `gorm:"column:title"`
		AdminUserID  uuid.UUID `gorm:"column:admin_user_id"`
		DepartmentID string    `gorm:"column:department_id"`
	}
	var res newsTarget
	err := r.db.Table("module_public_relations pr").
		Select("pr.title, pr.admin_user_id, COALESCE((SELECT ad.department_id::text FROM admin_departments ad WHERE ad.admin_id = pr.admin_user_id LIMIT 1), '') as department_id").
		Where("pr.id = ?", prId).
		Scan(&res).Error
	return res.Title, res.AdminUserID, res.DepartmentID, err
}

func (r *publicRelationMobileRepository) DeleteComment(commentId string, userId string) error {
	return r.db.Where("id = ? AND user_id = ?", commentId, userId).Delete(&domain.PublicRelationComment{}).Error
}

func (r *publicRelationMobileRepository) ReportComment(commentId string, reporterUserID string) (bool, error) {
	res := r.db.Model(&domain.PublicRelationComment{}).
		Where("id = ? AND status = ?", commentId, "active").
		Updates(map[string]interface{}{
			"status":       "reported",
			"updated_by":   reporterUserID,
			"updated_date": time.Now(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *publicRelationMobileRepository) HideComment(commentId string, userId string) error {
	res := r.db.Model(&domain.PublicRelationComment{}).
		Where("id = ? AND user_id = ?", commentId, userId).
		Update("status", "hidden")
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("comment not found or forbidden")
	}
	return nil
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
