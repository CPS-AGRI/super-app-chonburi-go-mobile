package usecase_test

import (
	"context"
	"testing"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/usecase"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockAuthRepository implements domain.AuthRepository for testing
type MockAuthRepository struct {
	mock.Mock
}

func (m *MockAuthRepository) GetByPhoneNumber(phone string) (*domain.AppUser, error) {
	args := m.Called(phone)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AppUser), args.Error(1)
}

func (m *MockAuthRepository) GetByEmail(email string) (*domain.AppUser, error) {
	args := m.Called(email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AppUser), args.Error(1)
}

func (m *MockAuthRepository) GetByProviderID(provider, providerID string) (*domain.AppUser, error) {
	args := m.Called(provider, providerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AppUser), args.Error(1)
}

func (m *MockAuthRepository) GetByID(id uuid.UUID) (*domain.AppUser, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AppUser), args.Error(1)
}

func (m *MockAuthRepository) Create(user *domain.AppUser) error {
	args := m.Called(user)
	return args.Error(0)
}

func (m *MockAuthRepository) Update(user *domain.AppUser) error {
	args := m.Called(user)
	return args.Error(0)
}

func (m *MockAuthRepository) CreateOauthAccount(acc *domain.UserOauthAccount) error {
	args := m.Called(acc)
	return args.Error(0)
}

func (m *MockAuthRepository) UpdateOauthAccount(acc *domain.UserOauthAccount) error {
	args := m.Called(acc)
	return args.Error(0)
}

func (m *MockAuthRepository) Delete(user *domain.AppUser) error {
	args := m.Called(user)
	return args.Error(0)
}

// MockSMSService implements infrastructure.SMSService for testing
type MockSMSService struct {
	mock.Mock
}

func (m *MockSMSService) SendOTP(ctx context.Context, phoneNumber, otpCode, ref string) error {
	args := m.Called(ctx, phoneNumber, otpCode, ref)
	return args.Error(0)
}

func setupTestUseCase() (domain.AuthUseCase, *MockAuthRepository, *MockSMSService, *config.Config) {
	cfg := &config.Config{
		JWTSecret: "test-secret-key-12345",
	}
	mockRepo := new(MockAuthRepository)
	mockSMS := new(MockSMSService)
	authUC := usecase.NewAuthUseCase(mockRepo, cfg, mockSMS, nil)
	return authUC, mockRepo, mockSMS, cfg
}

func TestRequestOTP_Success(t *testing.T) {
	authUC, _, mockSMS, _ := setupTestUseCase()
	phone := "0812345678"

	mockSMS.On("SendOTP", mock.Anything, phone, mock.Anything, mock.Anything).Return(nil)

	res, err := authUC.RequestOTP(phone)

	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.True(t, res.Success)
	assert.NotEmpty(t, res.Ref)
	assert.Len(t, res.OTP, 6)
	mockSMS.AssertExpectations(t)
}

func TestVerifyOTP_Success_UnregisteredUser(t *testing.T) {
	authUC, mockRepo, mockSMS, _ := setupTestUseCase()
	phone := "0898765432"

	mockSMS.On("SendOTP", mock.Anything, phone, mock.Anything, mock.Anything).Return(nil)

	reqRes, err := authUC.RequestOTP(phone)
	assert.NoError(t, err)

	// User does not exist in DB
	mockRepo.On("GetByPhoneNumber", phone).Return(nil, assert.AnError)

	verifyRes, err := authUC.VerifyOTP(phone, reqRes.OTP, reqRes.Ref)

	assert.NoError(t, err)
	assert.NotNil(t, verifyRes)
	assert.True(t, verifyRes.Success)
	assert.False(t, verifyRes.IsRegistered)
	assert.NotEmpty(t, verifyRes.TempToken)
}

func TestRegister_Success_FullPayload(t *testing.T) {
	authUC, mockRepo, _, cfg := setupTestUseCase()
	phone := "0811112222"

	// Create valid temp token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"phone_number": phone,
		"exp":          time.Now().Add(15 * time.Minute).Unix(),
		"purpose":      "registration",
	})
	tempTokenStr, _ := token.SignedString([]byte(cfg.JWTSecret))

	// User not yet registered
	mockRepo.On("GetByPhoneNumber", phone).Return(nil, assert.AnError)
	mockRepo.On("Create", mock.Anything).Return(nil)

	req := domain.RegisterRequest{
		TempToken:   tempTokenStr,
		Pin:         "123456",
		Prefix:      "นาย",
		FirstName:   "สมชาย",
		LastName:    "ใจดี",
		Email:       "somchai@example.com",
		Birthday:    "1995-05-15",
		IDCardHash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		LaserIDHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Province:    "ชลบุรี",
	}

	authRes, err := authUC.Register(req)

	assert.NoError(t, err)
	assert.NotNil(t, authRes)
	assert.NotEmpty(t, authRes.AccessToken)
	assert.NotEmpty(t, authRes.RefreshToken)
	assert.Equal(t, phone, authRes.User.PhoneNumber)
	mockRepo.AssertExpectations(t)
}

func TestRegister_InvalidSHA256Format(t *testing.T) {
	authUC, mockRepo, _, cfg := setupTestUseCase()
	phone := "0811113333"

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"phone_number": phone,
		"exp":          time.Now().Add(15 * time.Minute).Unix(),
		"purpose":      "registration",
	})
	tempTokenStr, _ := token.SignedString([]byte(cfg.JWTSecret))

	mockRepo.On("GetByPhoneNumber", phone).Return(nil, assert.AnError)

	req := domain.RegisterRequest{
		TempToken:  tempTokenStr,
		Pin:        "123456",
		IDCardHash: "invalid-short-hash", // Bad format
	}

	authRes, err := authUC.Register(req)

	assert.Error(t, err)
	assert.Nil(t, authRes)
	assert.Contains(t, err.Error(), "invalid id_card_hash format")
}
