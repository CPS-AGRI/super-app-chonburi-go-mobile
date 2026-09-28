package usecase

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"super-app-chonburi-go-mobile/internal/domain"
)

type analyticsUseCase struct {
	repo          domain.AnalyticsRepository
	cooldownCache sync.Map // key: string -> lastSeen time.Time
}

func NewAnalyticsUseCase(repo domain.AnalyticsRepository) domain.AnalyticsUseCase {
	uc := &analyticsUseCase{
		repo: repo,
	}

	// Background routine to evict expired cooldown entries every 10 minutes
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		for range ticker.C {
			now := time.Now()
			uc.cooldownCache.Range(func(key, val interface{}) bool {
				if lastSeen, ok := val.(time.Time); ok {
					if now.Sub(lastSeen) > 30*time.Minute {
						uc.cooldownCache.Delete(key)
					}
				}
				return true
			})
		}
	}()

	return uc
}

func (u *analyticsUseCase) TrackModuleUsage(req *domain.RecordModuleUsageDTO, userID *uuid.UUID, ip, userAgent string) error {
	if req == nil || strings.TrimSpace(req.ModuleCode) == "" {
		return nil
	}

	cleanModuleCode := strings.ToLower(strings.TrimSpace(req.ModuleCode))

	// 1. Identify actor (User UUID or Device ID or IP)
	actorKey := ip
	if req.DeviceID != "" {
		actorKey = req.DeviceID
	}
	if userID != nil {
		actorKey = userID.String()
	}

	// 2. Server-side cooldown (1 minute per actor + module) to eliminate network retries or bots
	cacheKey := actorKey + ":" + cleanModuleCode
	now := time.Now().UTC()
	if lastSeenVal, exists := u.cooldownCache.Load(cacheKey); exists {
		if lastSeen, ok := lastSeenVal.(time.Time); ok {
			if now.Sub(lastSeen) < 1*time.Minute {
				// Deduped / within cooldown: successfully absorbed
				return nil
			}
		}
	}
	u.cooldownCache.Store(cacheKey, now)

	// 3. Construct ISO-standard immutable usage log
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if platform == "" {
		platform = "mobile"
	}

	action := strings.ToUpper(strings.TrimSpace(req.Action))
	if action == "" {
		action = "VIEW"
	}

	usageLog := &domain.ModuleUsageLog{
		ID:         uuid.New(),
		UserID:     userID,
		ModuleCode: cleanModuleCode,
		Action:     action,
		Platform:   platform,
		DeviceID:   req.DeviceID,
		IPAddress:  ip,
		UserAgent:  userAgent,
		CreatedAt:  now,
	}

	// 4. Asynchronously persist to DB without blocking HTTP client
	go func(record *domain.ModuleUsageLog) {
		if err := u.repo.RecordUsage(record); err != nil {
			log.Printf("⚠️ [Analytics] Failed to record module usage: %v", err)
		}
	}(usageLog)

	return nil
}
