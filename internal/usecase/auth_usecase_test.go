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
	"golang.org/x/crypto/bcrypt"
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

func (m *MockAuthRepository) GetByOAuthAccountID(oauthID uuid.UUID) (*domain.AppUser, error) {
	args := m.Called(oauthID)
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

func (m *MockAuthRepository) SoftDeleteUser(userID uuid.UUID) error {
	args := m.Called(userID)
	return args.Error(0)
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

func (m *MockAuthRepository) DeleteOauthAccount(id uuid.UUID) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockAuthRepository) GetSocialLinks(userID uuid.UUID) (*domain.SocialAccountsResponse, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SocialAccountsResponse), args.Error(1)
}

func (m *MockAuthRepository) UnlinkSocial(userID uuid.UUID, provider string) error {
	args := m.Called(userID, provider)
	return args.Error(0)
}

func (m *MockAuthRepository) LinkSocialAccount(userID uuid.UUID, account *domain.UserOauthAccount) error {
	args := m.Called(userID, account)
	return args.Error(0)
}

func (m *MockAuthRepository) UpdateProfileImage(userID uuid.UUID, imageURL string) error {
	args := m.Called(userID, imageURL)
	return args.Error(0)
}

func (m *MockAuthRepository) GetExistingSocialProfile(userID uuid.UUID, provider string) (*domain.UserOauthAccount, error) {
	args := m.Called(userID, provider)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.UserOauthAccount), args.Error(1)
}

func (m *MockAuthRepository) BindOverrideSocialAccount(userID uuid.UUID, provider string, newOAuthID uuid.UUID) error {
	args := m.Called(userID, provider, newOAuthID)
	return args.Error(0)
}

func (m *MockAuthRepository) MigrateThaiIDAccount(oldUserID, newUserID uuid.UUID, oauthAcc *domain.UserOauthAccount, updateInfo *domain.UserInformation) error {
	args := m.Called(oldUserID, newUserID, oauthAcc, updateInfo)
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

func TestRequestOTP_ProductionHidesOTP(t *testing.T) {
	cfg := &config.Config{
		AppEnv:    "production",
		JWTSecret: "test-secret-key-12345",
	}
	mockRepo := new(MockAuthRepository)
	mockSMS := new(MockSMSService)
	authUC := usecase.NewAuthUseCase(mockRepo, cfg, mockSMS, nil)

	phone := "0812345678"
	mockSMS.On("SendOTP", mock.Anything, phone, mock.Anything, mock.Anything).Return(nil)

	res, err := authUC.RequestOTP(phone)

	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.True(t, res.Success)
	assert.NotEmpty(t, res.Ref)
	assert.Empty(t, res.OTP)
	mockSMS.AssertExpectations(t)
}

func TestRequestOTP_FailsWhenSMSGatewayFails(t *testing.T) {
	authUC, _, mockSMS, _ := setupTestUseCase()
	phone := "0812345678"

	mockSMS.On("SendOTP", mock.Anything, phone, mock.Anything, mock.Anything).Return(assert.AnError)

	res, err := authUC.RequestOTP(phone)

	assert.Error(t, err)
	assert.Nil(t, res)
	mockSMS.AssertExpectations(t)
}

func TestBindPhone_RejectsEmptyOTP(t *testing.T) {
	authUC, _, _, _ := setupTestUseCase()
	res, err := authUC.BindPhone("google", uuid.New().String(), "0812345678", "", "", "")
	assert.Error(t, err)
	assert.Nil(t, res)
}

func TestRefreshToken_RejectsAccessTokenType(t *testing.T) {
	authUC, _, _, cfg := setupTestUseCase()
	userID := uuid.New()

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":    userID.String(),
		"token_type": "access",
		"exp":        time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, _ := accessToken.SignedString([]byte(cfg.JWTSecret))

	res, err := authUC.RefreshToken(tokenStr)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "access token cannot be used as refresh token")
}

func TestDeleteAccount_CallsSoftDeleteUser(t *testing.T) {
	authUC, mockRepo, _, _ := setupTestUseCase()
	userID := uuid.New()

	mockRepo.On("SoftDeleteUser", userID).Return(nil)

	err := authUC.DeleteAccount(userID)
	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestVerifyOTP_RegisteredUser_ReturnsTempTokenAndResetPin(t *testing.T) {
	authUC, mockRepo, mockSMS, _ := setupTestUseCase()
	phone := "0812349999"
	userID := uuid.New()
	oldPinHash, _ := bcrypt.GenerateFromPassword([]byte("111111"), bcrypt.DefaultCost)

	existingUser := &domain.AppUser{
		ID:          userID,
		PhoneNumber: phone,
		PinHash:     string(oldPinHash),
	}

	mockSMS.On("SendOTP", mock.Anything, phone, mock.Anything, mock.Anything).Return(nil)
	reqRes, err := authUC.RequestOTP(phone)
	assert.NoError(t, err)

	mockRepo.On("GetByPhoneNumber", phone).Return(existingUser, nil)
	mockRepo.On("GetByID", userID).Return(existingUser, nil)
	mockRepo.On("Update", mock.Anything).Return(nil)

	verifyRes, err := authUC.VerifyOTP(phone, reqRes.OTP, reqRes.Ref)
	assert.NoError(t, err)
	assert.NotNil(t, verifyRes)
	assert.True(t, verifyRes.IsRegistered)
	assert.NotEmpty(t, verifyRes.TempToken)

	// Now reset PIN using the temp_token returned by VerifyOTP
	resetRes, err := authUC.ResetPin(verifyRes.TempToken, phone, "654321")
	assert.NoError(t, err)
	assert.NotNil(t, resetRes)
	assert.NotEmpty(t, resetRes.AccessToken)
	assert.NotEmpty(t, resetRes.RefreshToken)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(existingUser.PinHash), []byte("654321")))
}

func TestChangePin_Success(t *testing.T) {
	authUC, mockRepo, _, _ := setupTestUseCase()
	userID := uuid.New()
	oldPinHash, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)

	existingUser := &domain.AppUser{
		ID:          userID,
		PhoneNumber: "0812348888",
		PinHash:     string(oldPinHash),
	}

	mockRepo.On("GetByID", userID).Return(existingUser, nil)
	mockRepo.On("Update", mock.Anything).Return(nil)

	// Wrong old PIN fails
	err := authUC.ChangePin(userID, "000000", "654321")
	assert.Error(t, err)

	// Correct old PIN succeeds
	err = authUC.ChangePin(userID, "123456", "654321")
	assert.NoError(t, err)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(existingUser.PinHash), []byte("654321")))
}

func TestVerifyOTP_ThenBindPhone_DoubleConsumptionFixed(t *testing.T) {
	authUC, mockRepo, mockSMS, _ := setupTestUseCase()
	phone := "0812347777"
	oauthAccID := uuid.New()
	tempUserID := uuid.New()

	tempUser := &domain.AppUser{
		ID:          tempUserID,
		PhoneNumber: "",
		OauthAccounts: []domain.UserOauthAccount{
			{
				ID:         oauthAccID,
				UserId:     tempUserID,
				Provider:   "google",
				ProviderId: "google-sub-123",
				Email:      "user@example.com",
			},
		},
		Information: &domain.UserInformation{
			UserId: tempUserID,
			Name:   "Test",
		},
	}

	mockSMS.On("SendOTP", mock.Anything, phone, mock.Anything, mock.Anything).Return(nil)
	reqRes, err := authUC.RequestOTP(phone)
	assert.NoError(t, err)

	// Step 1: Mobile app calls VerifyOTP first (consumes OTP, sets verified marker)
	mockRepo.On("GetByPhoneNumber", phone).Return(nil, assert.AnError)
	verifyRes, err := authUC.VerifyOTP(phone, reqRes.OTP, reqRes.Ref)
	assert.NoError(t, err)
	assert.True(t, verifyRes.Success)

	// Step 2: Mobile app calls BindPhone next (even with empty OTP or consumed OTP)
	mockRepo.On("GetByOAuthAccountID", oauthAccID).Return(tempUser, nil)
	mockRepo.On("GetByID", tempUserID).Return(tempUser, nil)
	mockRepo.On("Update", mock.Anything).Return(nil)

	bindRes, err := authUC.BindPhone("google", oauthAccID.String(), phone, "", "", "123456")
	assert.NoError(t, err)
	assert.NotNil(t, bindRes)
	assert.NotEmpty(t, bindRes.AccessToken)
	assert.Equal(t, phone, bindRes.User.PhoneNumber)
}


