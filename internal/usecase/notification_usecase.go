package usecase

import (
	"super-app-chonburi-go-mobile/internal/domain"

	"github.com/google/uuid"
)

type notificationUseCase struct {
	repo domain.NotificationRepository
}

func NewNotificationUseCase(repo domain.NotificationRepository) domain.NotificationUseCase {
	return &notificationUseCase{repo: repo}
}

func (u *notificationUseCase) GetNotifications(userID string) ([]domain.NotificationResponseItem, error) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	return u.repo.GetNotifications(userUUID)
}

func (u *notificationUseCase) MarkAsRead(userID string, notificationID string) error {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return err
	}
	notifUUID, err := uuid.Parse(notificationID)
	if err != nil {
		return err
	}
	return u.repo.MarkAsRead(userUUID, notifUUID)
}

func (u *notificationUseCase) MarkAllAsRead(userID string) error {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return err
	}
	return u.repo.MarkAllAsRead(userUUID)
}
