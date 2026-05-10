package usecase

import (
	"context"
	"errors"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/api/idtoken"
)

type authUseCase struct {
	repo   domain.AuthRepository
	config *config.Config
}

func NewAuthUseCase(repo domain.AuthRepository, cfg *config.Config) domain.AuthUseCase {
	return &authUseCase{
		repo:   repo,
		config: cfg,
	}
}

func (u *authUseCase) LoginWithGoogle(idToken string) (*domain.AuthResponse, error) {
	// 1. Verify Google ID Token
	payload, err := idtoken.Validate(context.Background(), idToken, u.config.GoogleClientID)
	if err != nil {
		return nil, errors.New("invalid google token")
	}

	email := payload.Claims["email"].(string)
	googleID := payload.Subject
	name := payload.Claims["name"].(string)
	picture := payload.Claims["picture"].(string)

	// 2. Check if user exists
	user, err := u.repo.GetByProviderID("google", googleID)
	if err != nil {
		// Try by email if provider ID not found
		user, err = u.repo.GetByEmail(email)
		if err != nil {
			// 3. Create new user if not exists
			user = &domain.AppUser{
				ID:              uuid.New(),
				PhoneNumber:     "", // OAuth users might not have phone initially
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
			// Update provider info for existing email user
			user.Provider = stringPtr("google")
			user.ProviderId = &googleID
			u.repo.Update(user)
		}
	}

	// 4. Generate Tokens
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
		"exp":     time.Now().Add(time.Hour * 24).Unix(), // 24 hours
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(u.config.JWTSecret))
}

func (u *authUseCase) generateRefreshToken(user *domain.AppUser) (string, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID.String(),
		"exp":     time.Now().Add(time.Hour * 24 * 30).Unix(), // 30 days
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(u.config.JWTSecret))
}

func stringPtr(s string) *string {
	return &s
}
