package repository_test

import (
	"testing"

	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/repository"

	"github.com/stretchr/testify/assert"
)

func strPtr(s string) *string { return &s }

func TestResolveSocialUnlinkCleanup(t *testing.T) {
	googleURL := "https://lh3.googleusercontent.com/a/default-t"
	googleEmail := "testphett@gmail.com"
	manualUploadURL := "/uploads/profiles/custom.jpg"
	hash := "manual-email-hash"

	t.Run("clears Google avatar and synced email when unlinked", func(t *testing.T) {
		user := &domain.AppUser{
			ImageProfileUrl: strPtr(googleURL),
			Email:           strPtr(googleEmail),
			Information: &domain.UserInformation{
				Email:   strPtr(googleEmail),
				LogoUrl: googleURL,
			},
		}
		target := &domain.UserOauthAccount{
			Provider:  "Google",
			Email:     googleEmail,
			AvatarUrl: googleURL,
		}
		got := repository.ResolveSocialUnlinkCleanup(user, target, nil)
		assert.True(t, got.ClearImageProfileURL)
		assert.True(t, got.ClearInfoLogoURL)
		assert.True(t, got.ClearUserEmail)
		assert.True(t, got.ClearInfoEmail)
	})

	t.Run("preserves manually uploaded avatar and manually entered email", func(t *testing.T) {
		user := &domain.AppUser{
			ImageProfileUrl: strPtr(manualUploadURL),
			Email:           strPtr(googleEmail),
			EmailHash:       &hash,
			Information: &domain.UserInformation{
				Email:   strPtr(googleEmail),
				LogoUrl: manualUploadURL,
			},
		}
		target := &domain.UserOauthAccount{
			Provider:  "Google",
			Email:     googleEmail,
			AvatarUrl: googleURL,
		}
		got := repository.ResolveSocialUnlinkCleanup(user, target, nil)
		assert.False(t, got.ClearImageProfileURL)
		assert.False(t, got.ClearInfoLogoURL)
		assert.False(t, got.ClearUserEmail)
		assert.False(t, got.ClearInfoEmail)
	})

	t.Run("preserves avatar/email still shared by another linked social account", func(t *testing.T) {
		user := &domain.AppUser{
			ImageProfileUrl: strPtr(googleURL),
			Email:           strPtr(googleEmail),
			Information: &domain.UserInformation{
				Email: strPtr(googleEmail),
			},
		}
		target := &domain.UserOauthAccount{
			Provider:  "Google",
			Email:     googleEmail,
			AvatarUrl: googleURL,
		}
		remaining := []domain.UserOauthAccount{
			{Provider: "Apple", Email: googleEmail},
		}
		got := repository.ResolveSocialUnlinkCleanup(user, target, remaining)
		assert.True(t, got.ClearImageProfileURL)
		assert.False(t, got.ClearUserEmail)
		assert.False(t, got.ClearInfoEmail)
	})
}
