package http_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"super-app-chonburi-go-mobile/config"
	deliveryhttp "super-app-chonburi-go-mobile/internal/delivery/http"
	"super-app-chonburi-go-mobile/internal/delivery/http/middleware"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/usecase"
	"super-app-chonburi-go-mobile/pkg/jwtutil"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

type stubWaterLevelUseCase struct {
	err error
}

func (s *stubWaterLevelUseCase) GetStations(ctx context.Context) ([]domain.WaterStation, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []domain.WaterStation{{Serial: "CANAL-01", Name: "Canal 1", HasForecast: true}}, nil
}
func (s *stubWaterLevelUseCase) GetDeviceDetail(ctx context.Context, serial string) (*domain.WaterDeviceDetail, error) {
	return &domain.WaterDeviceDetail{}, s.err
}
func (s *stubWaterLevelUseCase) GetDeviceHistory(ctx context.Context, serial string, timeframe int) ([]domain.WaterHistoryPoint, error) {
	return nil, s.err
}
func (s *stubWaterLevelUseCase) GetDeviceForecast(ctx context.Context, serial string, days int, date string) (*domain.WaterForecastResult, error) {
	return &domain.WaterForecastResult{Success: true}, s.err
}
func (s *stubWaterLevelUseCase) GetSnapshots(ctx context.Context, serial, date string) ([]domain.WaterSnapshotLog, error) {
	return nil, s.err
}
func (s *stubWaterLevelUseCase) InitStream(ctx context.Context, serial string) (*domain.WaterStreamResult, error) {
	if serial != "CANAL-01" {
		return nil, usecase.ErrInvalidWaterStationSerial
	}
	return &domain.WaterStreamResult{StreamURL: "https://example.com/live"}, nil
}
func (s *stubWaterLevelUseCase) TerminateStream(ctx context.Context, serial string) error {
	if serial != "CANAL-01" {
		return usecase.ErrInvalidWaterStationSerial
	}
	return nil
}
func (s *stubWaterLevelUseCase) GetActiveAlerts(ctx context.Context) ([]domain.WaterAlert, error) {
	return nil, s.err
}
func (s *stubWaterLevelUseCase) RelayWebRTCOffer(ctx context.Context, serial, sdpOffer string) (string, error) {
	if serial != "CANAL-01" {
		return "", usecase.ErrInvalidWaterStationSerial
	}
	return "v=0", nil
}

type stubPublicRelationUseCase struct {
	lastHideUserID   string
	hideErr          error
	lastReportUserID string
	reportErr        error
}

func (s *stubPublicRelationUseCase) GetNewsFeed(moduleId string, page int, limit int, userId string) ([]domain.PublicRelation, error) {
	return nil, nil
}
func (s *stubPublicRelationUseCase) GetNewsDetail(moduleId string, id string, userId string) (*domain.PublicRelation, bool, error) {
	return nil, false, nil
}
func (s *stubPublicRelationUseCase) ToggleLike(prId string, userId string) (bool, error) {
	return false, nil
}
func (s *stubPublicRelationUseCase) AddComment(moduleId string, prId string, userId string, commentText string) (*domain.PublicRelationComment, error) {
	return nil, nil
}
func (s *stubPublicRelationUseCase) GetComments(prId string) ([]domain.PublicRelationComment, error) {
	return nil, nil
}
func (s *stubPublicRelationUseCase) DeleteComment(commentId string, userId string) error {
	return nil
}
func (s *stubPublicRelationUseCase) ReportComment(commentId string, reporterUserID string) error {
	s.lastReportUserID = reporterUserID
	return s.reportErr
}
func (s *stubPublicRelationUseCase) HideComment(commentId string, userId string) error {
	s.lastHideUserID = userId
	return s.hideErr
}
func (s *stubPublicRelationUseCase) GetWelcomeScreen() (*domain.MunicipalityWelcomeScreen, error) {
	return nil, nil
}
func (s *stubPublicRelationUseCase) RecordNewsView(prId string, userId string, sessionId string) error {
	return nil
}

func signTestToken(secret, userID string) string {
	claims := jwtutil.MobileClaims{
		UserID:    userID,
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	return tok
}

func TestWaterLevelStreamRequiresAuthAndSanitizesErrors(t *testing.T) {
	cfg := &config.Config{JWTSecret: "test-secret-key-1234"}
	app := fiber.New()
	deliveryhttp.NewWaterLevelHandler(app, &stubWaterLevelUseCase{}, cfg)

	// Unauthenticated GET works
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/mobile/water-level/devices", nil))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Unauthenticated stream endpoints return 401
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/mobile/water-level/devices/CANAL-01/stream"},
		{http.MethodPost, "/api/v1/mobile/water-level/devices/CANAL-01/webrtc-offer"},
		{http.MethodDelete, "/api/v1/mobile/water-level/devices/CANAL-01/stream"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("v=0"))
		r, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, r.StatusCode)
	}

	// Authenticated valid serial passes
	token := signTestToken(cfg.JWTSecret, "user-123")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/water-level/devices/CANAL-01/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, r.StatusCode)

	// Authenticated unknown serial returns 404
	reqUnknown := httptest.NewRequest(http.MethodDelete, "/api/v1/mobile/water-level/devices/BOGUS-99/stream", nil)
	reqUnknown.Header.Set("Authorization", "Bearer "+token)
	rUnknown, err := app.Test(reqUnknown)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rUnknown.StatusCode)

	// Internal errors are sanitized
	errApp := fiber.New()
	deliveryhttp.NewWaterLevelHandler(errApp, &stubWaterLevelUseCase{err: errors.New("dial tcp 10.1.2.19:8081: connection refused")}, cfg)
	errResp, err := errApp.Test(httptest.NewRequest(http.MethodGet, "/api/v1/mobile/water-level/devices", nil))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, errResp.StatusCode)
	bodyBytes, _ := io.ReadAll(errResp.Body)
	assert.NotContains(t, string(bodyBytes), "10.1.2.19")
}

func TestHideCommentEnforcesAuthenticatedUserAndForbidden(t *testing.T) {
	cfg := &config.Config{JWTSecret: "test-secret-key-1234"}
	uc := &stubPublicRelationUseCase{hideErr: errors.New("comment not found or forbidden")}
	app := fiber.New()
	deliveryhttp.NewPublicRelationMobileHandler(app, uc, cfg)

	token := signTestToken(cfg.JWTSecret, "user-abc")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/public-relations/comments/comment-1/hide", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, "user-abc", uc.lastHideUserID)
}

func TestReportCommentEnforcesAuthAndStatusMapping(t *testing.T) {
	cfg := &config.Config{JWTSecret: "test-secret-key-1234"}
	token := signTestToken(cfg.JWTSecret, "reporter-1")

	t.Run("requires auth", func(t *testing.T) {
		app := fiber.New()
		deliveryhttp.NewPublicRelationMobileHandler(app, &stubPublicRelationUseCase{}, cfg)
		resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/api/v1/mobile/public-relations/comments/c-1/report", nil))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("maps not found to 404", func(t *testing.T) {
		uc := &stubPublicRelationUseCase{reportErr: domain.ErrCommentNotFound}
		app := fiber.New()
		deliveryhttp.NewPublicRelationMobileHandler(app, uc, cfg)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/public-relations/comments/c-1/report", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.Equal(t, "reporter-1", uc.lastReportUserID)
	})

	t.Run("maps own comment report to 400", func(t *testing.T) {
		uc := &stubPublicRelationUseCase{reportErr: domain.ErrCannotReportOwnComment}
		app := fiber.New()
		deliveryhttp.NewPublicRelationMobileHandler(app, uc, cfg)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/public-relations/comments/c-1/report", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestAPIRateLimiterSeparatesAuthenticatedAndAnonymousKeys(t *testing.T) {
	cfg := &config.Config{JWTSecret: "test-secret-key-1234"}
	app := fiber.New()
	app.Use(middleware.NewAPIRateLimiter(cfg, 2, 3, time.Minute))
	app.Get("/ping", func(c fiber.Ctx) error { return c.SendString("ok") })

	// User A hits limit after 2 requests
	tokA := signTestToken(cfg.JWTSecret, "user-A")
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.Header.Set("Authorization", "Bearer "+tokA)
		r, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, r.StatusCode)
	}
	reqBlocked := httptest.NewRequest(http.MethodGet, "/ping", nil)
	reqBlocked.Header.Set("Authorization", "Bearer "+tokA)
	rBlocked, err := app.Test(reqBlocked)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, rBlocked.StatusCode)

	// User B on same IP still succeeds independently
	tokB := signTestToken(cfg.JWTSecret, "user-B")
	reqB := httptest.NewRequest(http.MethodGet, "/ping", nil)
	reqB.Header.Set("Authorization", "Bearer "+tokB)
	rB, err := app.Test(reqB)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rB.StatusCode)
}

