package usecase_test

import (
	"errors"
	"testing"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockMobileCCTVRepository implements domain.MobileCCTVRepository for testing
type MockMobileCCTVRepository struct {
	mock.Mock
}

func (m *MockMobileCCTVRepository) GetCameras(query domain.MobileCCTVQuery) ([]domain.CCTV, error) {
	args := m.Called(query)
	return args.Get(0).([]domain.CCTV), args.Error(1)
}

func (m *MockMobileCCTVRepository) CreateRequest(req *domain.CCTVRequest) error {
	args := m.Called(req)
	return args.Error(0)
}

func (m *MockMobileCCTVRepository) GetRequestsByUserID(userID uuid.UUID) ([]domain.CCTVRequest, error) {
	args := m.Called(userID)
	return args.Get(0).([]domain.CCTVRequest), args.Error(1)
}

func (m *MockMobileCCTVRepository) CreateViewLog(log *domain.CCTVViewLog) error {
	args := m.Called(log)
	return args.Error(0)
}

func (m *MockMobileCCTVRepository) FindCameraByIDOrName(idOrName string) (*domain.CCTV, error) {
	args := m.Called(idOrName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.CCTV), args.Error(1)
}

func TestCCTVUseCase_CreateHandshakeTicket_ThaIDGatekeeper(t *testing.T) {
	mockCCTVRepo := new(MockMobileCCTVRepository)
	mockAuthRepo := new(MockAuthRepository)

	cfg := &config.Config{
		CCTVPartnerAPIKey:    "test_partner_api_key_12345",
		CCTVPartnerPortalURL: "https://cctv-partner.com/portal",
		CCTVAllowedCallbacks: []string{"https://cctv-partner.com/portal"},
	}

	uc := usecase.NewCCTVUseCase(mockCCTVRepo, mockAuthRepo, cfg, nil)

	nonExistentUserID := uuid.New()
	unverifiedUserID := uuid.New()
	verifiedUserID := uuid.New()

	// 0. Non-existent user in system -> MUST be rejected with ErrUserNotFound
	mockAuthRepo.On("GetByID", nonExistentUserID).Return(nil, errors.New("record not found"))
	inputDownload := domain.CreateHandshakeTicketInput{
		CCTVID: uuid.New().String(),
		Action: "download",
	}
	resNonExistent, errNonExistent := uc.CreateHandshakeTicket(nonExistentUserID, inputDownload)
	assert.Nil(t, resNonExistent)
	assert.True(t, errors.Is(errNonExistent, usecase.ErrUserNotFound), "User not existing in our system must return ErrUserNotFound")

	// 1. User without ThaID verification
	mockAuthRepo.On("GetByID", unverifiedUserID).Return(&domain.AppUser{
		ID: unverifiedUserID,
		Information: &domain.UserInformation{
			VerificationStatus: "unverified",
		},
		OauthAccounts: []domain.UserOauthAccount{},
	}, nil)

	// Case A: Unverified user requesting "download" -> MUST be blocked by Gatekeeper
	res, err := uc.CreateHandshakeTicket(unverifiedUserID, inputDownload)
	assert.Nil(t, res)
	assert.True(t, errors.Is(err, usecase.ErrThaIDRequired), "Unverified user requesting download must return ErrThaIDRequired")

	// 2. User with ThaID verification
	mockAuthRepo.On("GetByID", verifiedUserID).Return(&domain.AppUser{
		ID: verifiedUserID,
		Information: &domain.UserInformation{
			VerificationStatus: "verified",
		},
		OauthAccounts: []domain.UserOauthAccount{
			{Provider: "thaiid"},
		},
	}, nil)

	// Case B: Verified user requesting download (Redis nil fallback test)
	resVerified, errVerified := uc.CreateHandshakeTicket(verifiedUserID, inputDownload)
	// Even if Redis is nil, it correctly passed the ThaID check and reached cache error
	assert.Nil(t, resVerified)
	assert.Contains(t, errVerified.Error(), "distributed cache not available")
}

func TestCCTVUseCase_ValidateCallbackURL_SSRFAndWhitelist(t *testing.T) {
	mockCCTVRepo := new(MockMobileCCTVRepository)
	mockAuthRepo := new(MockAuthRepository)

	cfg := &config.Config{
		CCTVPartnerAPIKey: "test_key",
		CCTVAllowedCallbacks: []string{
			"https://cctv-partner.com/portal",
			"https://cctv.chonburi.go.th/portal",
		},
	}

	uc := usecase.NewCCTVUseCase(mockCCTVRepo, mockAuthRepo, cfg, nil)

	// Valid whitelisted URL
	assert.True(t, uc.ValidateCallbackURL("https://cctv-partner.com/portal"))
	assert.True(t, uc.ValidateCallbackURL("https://cctv-partner.com/portal?ref=123"))
	assert.True(t, uc.ValidateCallbackURL("https://cctv.chonburi.go.th/portal"))

	// Unauthorized / Open Redirect URLs (OWASP A01 & A10)
	assert.False(t, uc.ValidateCallbackURL("https://evil-attacker.com/steal-ticket"))
	assert.False(t, uc.ValidateCallbackURL("http://localhost/internal-admin"))
	assert.False(t, uc.ValidateCallbackURL("javascript:alert(1)"))
	assert.False(t, uc.ValidateCallbackURL(""))

	// SSRF Defense: Private IP ranges and loopback
	assert.False(t, uc.ValidateCallbackURL("http://127.0.0.1/admin"))
	assert.False(t, uc.ValidateCallbackURL("http://10.0.0.1/secret"))
	assert.False(t, uc.ValidateCallbackURL("http://192.168.1.1/internal"))
	assert.False(t, uc.ValidateCallbackURL("http://169.254.169.254/metadata"))

	// Path Traversal & Subdomain Confusion Defense
	assert.False(t, uc.ValidateCallbackURL("https://cctv-partner.com/portal/../admin"))
	assert.False(t, uc.ValidateCallbackURL("https://cctv-partner.com/portal/%2e%2e/admin"))
	assert.False(t, uc.ValidateCallbackURL("https://cctv-partner.com/portal.evil.com/steal"))
}

func TestCCTVUseCase_VerifyTicket_InputValidation(t *testing.T) {
	mockCCTVRepo := new(MockMobileCCTVRepository)
	mockAuthRepo := new(MockAuthRepository)

	cfg := &config.Config{
		CCTVPartnerAPIKey: "valid_partner_key_prod",
		PH6ClientID:       "ph6_client_test",
		PH6ClientSecret:   "ph6_secret_test",
	}

	uc := usecase.NewCCTVUseCase(mockCCTVRepo, mockAuthRepo, cfg, nil)

	validSampleTicket := "TKT_7f8a9b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a"

	// 1. Wrong partner API key & credentials -> Unauthorized
	_, err := uc.VerifyHandshakeTicket("wrong_key", domain.VerifyTicketInput{
		Ticket: validSampleTicket,
	})
	assert.True(t, errors.Is(err, usecase.ErrUnauthorizedPartner))

	_, errCreds := uc.VerifyHandshakeTicket("", domain.VerifyTicketInput{
		Ticket:       validSampleTicket,
		ClientID:     "wrong_id",
		ClientSecret: "wrong_secret",
	})
	assert.True(t, errors.Is(errCreds, usecase.ErrUnauthorizedPartner))

	// 2. Malformed ticket (OWASP Injection attempt) -> Invalid Ticket
	maliciousTickets := []string{
		"TKT_123' OR 1=1 --",
		"<script>alert(1)</script>",
		"TKT_short",
		"invalid_prefix_1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
	}
	for _, malTicket := range maliciousTickets {
		// Test via Header API Key
		_, errHeader := uc.VerifyHandshakeTicket("valid_partner_key_prod", domain.VerifyTicketInput{
			Ticket: malTicket,
		})
		assert.True(t, errors.Is(errHeader, usecase.ErrInvalidTicket), "Malformed ticket must be rejected immediately")

		// Test via Ph6 Client Credentials
		_, errPh6 := uc.VerifyHandshakeTicket("", domain.VerifyTicketInput{
			Ticket:       malTicket,
			ClientID:     "ph6_client_test",
			ClientSecret: "ph6_secret_test",
		})
		assert.True(t, errors.Is(errPh6, usecase.ErrInvalidTicket), "Malformed ticket with Ph6 credentials must be rejected immediately")
	}

	// 3. Zero-allocation validator test for valid 256-bit entropy tickets
	assert.True(t, usecase.IsValidTicket(validSampleTicket))
	assert.False(t, usecase.IsValidTicket("TKT_invalid"))
	assert.False(t, usecase.IsValidTicket(""))
}
