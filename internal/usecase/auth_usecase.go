package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/idtoken"
)

type otpData struct {
	Code      string
	Ref       string
	ExpiresAt time.Time
}

type authUseCase struct {
	repo     domain.AuthRepository
	config   *config.Config
	otpStore map[string]otpData
	otpMu    sync.RWMutex
}

func NewAuthUseCase(repo domain.AuthRepository, cfg *config.Config) domain.AuthUseCase {
	return &authUseCase{
		repo:     repo,
		config:   cfg,
		otpStore: make(map[string]otpData),
	}
}

func (u *authUseCase) LoginWithGoogle(idToken string) (*domain.AuthResponse, error) {

	payload, err := idtoken.Validate(context.Background(), idToken, u.config.GoogleClientID)
	if err != nil {
		return nil, errors.New("invalid google token")
	}

	email := payload.Claims["email"].(string)
	googleID := payload.Subject
	name := payload.Claims["name"].(string)
	picture := payload.Claims["picture"].(string)

	user, err := u.repo.GetByProviderID("google", googleID)
	if err != nil {

		user, err = u.repo.GetByEmail(email)
		if err != nil {

			user = &domain.AppUser{
				ID:              uuid.New(),
				PhoneNumber:     "",
				Provider:        stringPtr("google"),
				ProviderId:      &googleID,
				ImageProfileUrl: &picture,
				IsConsent:       true,
				CreatedBy:       "system",
				CreatedDate:     time.Now(),
				UpdatedBy:       "system",
				UpdatedDate:     time.Now(),
				Information: &domain.UserInformation{
					Name:      name,
					Email:     &email,
					Status:    "active",
					IsConsent: true,
					CreatedBy: "system",
				},
			}
			err = u.repo.Create(user)
			if err != nil {
				return nil, err
			}
		} else {

			user.Provider = stringPtr("google")
			user.ProviderId = &googleID
			u.repo.Update(user)
		}
	}

	accessToken, err := u.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := u.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}

func (u *authUseCase) LoginWithFacebook(accessToken string) (*domain.AuthResponse, error) {
	return nil, errors.New("facebook login not implemented yet")
}

func (u *authUseCase) RefreshToken(refreshToken string) (*domain.AuthResponse, error) {
	return nil, errors.New("refresh token not implemented yet")
}

func (u *authUseCase) generateAccessToken(user *domain.AppUser) (string, error) {
	email := ""
	if user.Information != nil && user.Information.Email != nil {
		email = *user.Information.Email
	}

	claims := jwt.MapClaims{
		"user_id": user.ID.String(),
		"email":   email,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(u.config.JWTSecret))
}

func (u *authUseCase) generateRefreshToken(user *domain.AppUser) (string, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID.String(),
		"exp":     time.Now().Add(time.Hour * 24 * 30).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(u.config.JWTSecret))
}

func stringPtr(s string) *string {
	return &s
}

func (u *authUseCase) RequestOTP(phoneNumber string) (*domain.OTPRequestResponse, error) {
	// เจนรหัส OTP 6 หลัก
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return nil, fmt.Errorf("failed to generate random otp: %w", err)
	}
	otpCode := fmt.Sprintf("%06d", n.Int64()+100000)

	// เจน Reference Code 4 ตัวอักษรพิมพ์ใหญ่
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	refBytes := make([]byte, 4)
	for i := 0; i < 4; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return nil, fmt.Errorf("failed to generate ref code: %w", err)
		}
		refBytes[i] = letters[idx.Int64()]
	}
	ref := string(refBytes)

	// บันทึกใส่ Memory Store
	u.otpMu.Lock()
	u.otpStore[phoneNumber] = otpData{
		Code:      otpCode,
		Ref:       ref,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	u.otpMu.Unlock()

	// พิมพ์รหัสออกหน้าจอ console เพื่อการทดสอบใน local dev
	log.Printf("📱 [OTP Debug] Phone: %s -> OTP: %s, Ref: %s (Expires in 5m)", phoneNumber, otpCode, ref)

	return &domain.OTPRequestResponse{
		Success: true,
		Ref:     ref,
		OTP:     otpCode, // แนบไปให้หน้าบ้านหยิบใช้งานได้ทันทีในโหมด dev
	}, nil
}

func (u *authUseCase) VerifyOTP(phoneNumber, otp, ref string) (*domain.OTPVerifyResponse, error) {
	u.otpMu.Lock()
	stored, exists := u.otpStore[phoneNumber]
	if exists {
		// ลบ OTP ทิ้งทันทีเมื่อนำมาตรวจสอบ เพื่อป้องกัน replay attacks
		delete(u.otpStore, phoneNumber)
	}
	u.otpMu.Unlock()

	if !exists {
		return nil, errors.New("OTP verification code not found or expired")
	}

	if time.Now().After(stored.ExpiresAt) {
		return nil, errors.New("OTP code has expired")
	}

	if stored.Code != otp || stored.Ref != ref {
		return nil, errors.New("invalid OTP code or reference")
	}

	// ตรวจสอบสถานะการมีอยู่ของผู้ใช้งานด้วยเบอร์โทรศัพท์
	_, err := u.repo.GetByPhoneNumber(phoneNumber)
	if err != nil {
		// ไม่มีผู้ใช้ในระบบ -> คืนค่า false เพื่อให้ข้ามไปตั้ง PIN และลงทะเบียน
		// ทำการออก temp_token (JWT) เพื่อป้องกันการสวมสิทธิ์ในหน้า register
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"phone_number": phoneNumber,
			"exp":          time.Now().Add(15 * time.Minute).Unix(),
			"purpose":      "registration",
		})
		tempToken, err := token.SignedString([]byte(u.config.JWTSecret))
		if err != nil {
			return nil, err
		}

		return &domain.OTPVerifyResponse{
			Success:      true,
			IsRegistered: false,
			TempToken:    tempToken,
		}, nil
	}

	// มีผู้ใช้ในระบบแล้ว -> ส่งผลลัพธ์กลับเพื่อให้ไปหน้ากรอก PIN
	return &domain.OTPVerifyResponse{
		Success:      true,
		IsRegistered: true,
	}, nil
}

func (u *authUseCase) Register(pin, tempToken string) (*domain.AuthResponse, error) {
	// Parse tempToken และทำการตรวจสอบ
	parsedToken, err := jwt.Parse(tempToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(u.config.JWTSecret), nil
	})

	if err != nil || !parsedToken.Valid {
		return nil, errors.New("invalid or expired temporary registration token")
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	// ตรวจสอบ purpose
	if purpose, ok := claims["purpose"].(string); !ok || purpose != "registration" {
		return nil, errors.New("invalid token purpose")
	}

	phoneNumber, ok := claims["phone_number"].(string)
	if !ok || phoneNumber == "" {
		return nil, errors.New("phone number not found in token")
	}

	// เช็คซ้ำอีกครั้งว่าเบอร์มีอยู่ในระบบแล้วหรือไม่
	if _, err := u.repo.GetByPhoneNumber(phoneNumber); err == nil {
		return nil, errors.New("phone number is already registered")
	}

	// เข้ารหัส PIN
	hashedPin, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash pin: %w", err)
	}

	userID := uuid.New()
	user := &domain.AppUser{
		ID:              userID,
		PhoneNumber:     phoneNumber,
		PhoneNumberHash: hashValue(phoneNumber),
		PinHash:         string(hashedPin),
		IsConsent:       true,
		CreatedBy:       "registration_flow",
		CreatedDate:     time.Now(),
		UpdatedBy:       "registration_flow",
		UpdatedDate:     time.Now(),
		Information: &domain.UserInformation{
			UserId:             userID,
			Name:               "ผู้ใช้ชลบุรีพลัส", // Default name
			LastName:           fmt.Sprintf("เบอร์ %s", phoneNumber[len(phoneNumber)-4:]),
			Phone:              phoneNumber,
			Status:             "active",
			VerificationStatus: "unverified",
			IsConsent:          true,
			CreatedBy:          "registration_flow",
			CreatedDate:        time.Now(),
			UpdatedDate:        time.Now(),
		},
	}

	// สร้างผู้ใช้ในฐานข้อมูล
	if err := u.repo.Create(user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	accessToken, err := u.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := u.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}

func (u *authUseCase) LoginWithPin(phoneNumber, pin string) (*domain.AuthResponse, error) {
	user, err := u.repo.GetByPhoneNumber(phoneNumber)
	if err != nil {
		return nil, errors.New("invalid phone number or PIN")
	}

	// ตรวจสอบ PIN
	err = bcrypt.CompareHashAndPassword([]byte(user.PinHash), []byte(pin))
	if err != nil {
		return nil, errors.New("invalid phone number or PIN")
	}

	accessToken, err := u.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := u.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}

func hashValue(v string) string {
	h := sha256.New()
	h.Write([]byte(v))
	return hex.EncodeToString(h.Sum(nil))
}
