package repository

import (
	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type cctvRepository struct {
	db *gorm.DB
}

// NewCCTVRepository creates a new MobileCCTVRepository.
func NewCCTVRepository(db *gorm.DB) domain.MobileCCTVRepository {
	return &cctvRepository{db: db}
}

func (r *cctvRepository) GetCameras(query domain.MobileCCTVQuery) ([]domain.CCTV, error) {
	var cameras []domain.CCTV
	// Bounding Box search using coordinates index.
	// Latitude BETWEEN lat_min AND lat_max AND Longitude BETWEEN lng_min AND lng_max.
	// AccessLevel is restricted to PUBLIC cameras only.
	err := r.db.Where("access_level = ? AND latitude BETWEEN ? AND ? AND longitude BETWEEN ? AND ?",
		"PUBLIC", query.LatMin, query.LatMax, query.LngMin, query.LngMax).
		Find(&cameras).Error
	if err != nil {
		return nil, err
	}
	return cameras, nil
}

func (r *cctvRepository) CreateRequest(req *domain.CCTVRequest) error {
	return r.db.Create(req).Error
}

func (r *cctvRepository) GetRequestsByUserID(userID uuid.UUID) ([]domain.CCTVRequest, error) {
	var requests []domain.CCTVRequest
	// Preload CCTV camera relation to prevent N+1 query loops.
	// Filter by UserID and sort by CreatedAt DESC.
	err := r.db.Preload("CCTV").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&requests).Error
	if err != nil {
		return nil, err
	}
	return requests, nil
}
