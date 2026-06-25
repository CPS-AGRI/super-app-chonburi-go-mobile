package usecase

import (
	"errors"
	"time"

	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/google/uuid"
)

type cctvUseCase struct {
	repo domain.MobileCCTVRepository
}

// NewCCTVUseCase creates a new MobileCCTVUseCase.
func NewCCTVUseCase(repo domain.MobileCCTVRepository) domain.MobileCCTVUseCase {
	return &cctvUseCase{repo: repo}
}

func (u *cctvUseCase) GetCameras(query domain.MobileCCTVQuery) ([]domain.CCTV, error) {
	if query.LatMin > query.LatMax {
		query.LatMin, query.LatMax = query.LatMax, query.LatMin
	}
	if query.LngMin > query.LngMax {
		query.LngMin, query.LngMax = query.LngMax, query.LngMin
	}
	return u.repo.GetCameras(query)
}

func (u *cctvUseCase) CreateRequest(req *domain.CCTVRequest) error {
	if req.CCTVID == uuid.Nil {
		return errors.New("camera id is required")
	}
	if req.UserID == uuid.Nil {
		return errors.New("unauthorized user")
	}
	if req.EvidenceFileURL == "" {
		return errors.New("evidence file url is required")
	}
	if req.IncidentDate.IsZero() {
		return errors.New("incident date is required")
	}
	if req.StartTime == "" || req.EndTime == "" {
		return errors.New("start and end time range are required")
	}

	req.ID = uuid.New()
	req.Status = "PENDING"
	req.CreatedAt = time.Now()
	req.UpdatedAt = time.Now()

	return u.repo.CreateRequest(req)
}

func (u *cctvUseCase) GetMyRequests(userID uuid.UUID) ([]domain.CCTVRequest, error) {
	if userID == uuid.Nil {
		return nil, errors.New("invalid user id")
	}
	return u.repo.GetRequestsByUserID(userID)
}
