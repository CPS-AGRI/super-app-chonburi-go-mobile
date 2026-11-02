package usecase

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/infrastructure"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	ErrThaIDRequired       = errors.New("THAID_VERIFICATION_REQUIRED")
	ErrInvalidTicket       = errors.New("INVALID_OR_EXPIRED_TICKET")
	ErrUnauthorizedPartner = errors.New("UNAUTHORIZED_PARTNER")
	ErrRateLimitExceeded   = errors.New("RATE_LIMIT_EXCEEDED")
	ErrInvalidCallbackURL  = errors.New("INVALID_CALLBACK_URL")
	ErrUserNotFound        = errors.New("USER_NOT_FOUND_IN_SYSTEM")

	// Pre-compiled atomic Lua script for single-use ticket invalidation fallback (Redis < 6.2)
	getDelLuaScript = redis.NewScript(`
		local val = redis.call("GET", KEYS[1])
		if val then
			redis.call("DEL", KEYS[1])
		end
		return val
	`)
)

// IsValidTicket validates ticket format (TKT_ prefix + 64 lowercase hex chars) with zero heap allocations.
func IsValidTicket(t string) bool {
	const prefix = "TKT_"
	const expectedLen = 4 + 64
	if len(t) != expectedLen {
		return false
	}
	if t[0] != 'T' || t[1] != 'K' || t[2] != 'T' || t[3] != '_' {
		return false
	}
	for i := 4; i < expectedLen; i++ {
		c := t[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

type cctvUseCase struct {
	repo           domain.MobileCCTVRepository
	authRepo       domain.AuthRepository
	cfg            *config.Config
	redisClient    *infrastructure.RedisClient
	analyticsUC    domain.AnalyticsUseCase
	supportsGetDel atomic.Bool
}

// NewCCTVUseCase creates a new MobileCCTVUseCase with unified ticket handshake capabilities.
func NewCCTVUseCase(
	repo domain.MobileCCTVRepository,
	authRepo domain.AuthRepository,
	cfg *config.Config,
	redisClient *infrastructure.RedisClient,
	analyticsUC ...domain.AnalyticsUseCase,
) domain.MobileCCTVUseCase {
	var auc domain.AnalyticsUseCase
	if len(analyticsUC) > 0 {
		auc = analyticsUC[0]
	}
	uc := &cctvUseCase{
		repo:        repo,
		authRepo:    authRepo,
		cfg:         cfg,
		redisClient: redisClient,
		analyticsUC: auc,
	}
	uc.supportsGetDel.Store(true)
	return uc
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

// getVerifiedUser strictly verifies that the user exists in our database and checks ThaID verification.
func (u *cctvUseCase) getVerifiedUser(userID uuid.UUID) (*domain.AppUser, bool, error) {
	if u.authRepo == nil {
		return nil, false, errors.New("auth repository not available")
	}
	user, err := u.authRepo.GetByID(userID)
	if err != nil || user == nil {
		return nil, false, ErrUserNotFound
	}

	// 1. Check UserInformation VerificationStatus
	isThaIDVerified := false
	if user.Information != nil && strings.EqualFold(user.Information.VerificationStatus, "verified") {
		isThaIDVerified = true
	}

	// 2. Check OauthAccounts for ThaID provider
	if !isThaIDVerified {
		for _, acc := range user.OauthAccounts {
			if strings.EqualFold(acc.Provider, "thaiid") {
				isThaIDVerified = true
				break
			}
		}
	}

	return user, isThaIDVerified, nil
}

// CreateHandshakeTicket generates a single-use ephemeral ticket (TTL 60s) for Channel 1.
// Strict Gatekeeper: Must exist in our database and be verified with ThaID for download action.
func (u *cctvUseCase) CreateHandshakeTicket(userID uuid.UUID, input domain.CreateHandshakeTicketInput, meta ...domain.RequestMetadata) (*domain.CreateHandshakeTicketResponse, error) {
	if userID == uuid.Nil {
		return nil, errors.New("unauthorized user")
	}

	// Strict check: User MUST exist in our system
	user, isThaIDVerified, err := u.getVerifiedUser(userID)
	if err != nil {
		return nil, err
	}

	// Gatekeeper: If action is download, MUST be ThaID verified
	if strings.EqualFold(input.Action, "download") && !isThaIDVerified {
		return nil, ErrThaIDRequired
	}

	displayName := ""
	if user.Information != nil {
		displayName = strings.TrimSpace(user.Information.Name + " " + user.Information.LastName)
	}
	if displayName == "" && len(user.OauthAccounts) > 0 {
		for _, acc := range user.OauthAccounts {
			if strings.TrimSpace(acc.DisplayName) != "" {
				displayName = strings.TrimSpace(acc.DisplayName)
				break
			}
		}
	}

	// Generate high-entropy ticket ID (256 bits of entropy = 32 bytes = 64 hex characters)
	ticketBytes := make([]byte, 32)
	if _, err := rand.Read(ticketBytes); err != nil {
		return nil, fmt.Errorf("failed to generate ticket entropy: %w", err)
	}
	ticketID := "TKT_" + hex.EncodeToString(ticketBytes)

	ttl := 60 * time.Second
	if u.cfg != nil && u.cfg.CCTVTicketTTLSeconds > 0 {
		ttl = time.Duration(u.cfg.CCTVTicketTTLSeconds) * time.Second
	}

	now := time.Now().UTC()
	ticket := domain.CCTVHandshakeTicket{
		TicketID:        ticketID,
		UserReferenceID: "usr_" + userID.String(), // Pseudonymous reference ID, NEVER citizen ID
		DisplayName:     displayName,
		AuthChannel:     "thaid",
		IsThaIDVerified: isThaIDVerified,
		CCTVID:          input.CCTVID,
		Action:          input.Action,
		StartTime:       input.StartTime,
		EndTime:         input.EndTime,
		IssuedAt:        now,
		ExpiresAt:       now.Add(ttl),
	}

	// Store in Redis with configurable TTL
	if u.redisClient != nil {
		ticketKey := "cctv:ticket:" + ticketID
		if err := u.redisClient.SetJSON(context.Background(), ticketKey, ticket, ttl); err != nil {
			log.Printf("⚠️ [CCTV Redis Error] Failed to store ticket in Redis: %v", err)
			return nil, errors.New("ticket service temporarily unavailable")
		}
	} else {
		return nil, errors.New("distributed cache not available")
	}

	// 1. Record CCTV Request History in DB (module_cctv_requests)
	if strings.EqualFold(input.Action, "download") && u.repo != nil {
		var cctvUUID uuid.UUID
		if parsed, parseErr := uuid.Parse(input.CCTVID); parseErr == nil {
			cctvUUID = parsed
		}
		startTime := input.StartTime
		if startTime == "" {
			startTime = "00:00"
		}
		endTime := input.EndTime
		if endTime == "" {
			endTime = "23:59"
		}
		cctvReq := &domain.CCTVRequest{
			ID:              uuid.New(),
			UserID:          userID,
			CCTVID:          cctvUUID,
			IncidentDate:    now,
			StartTime:       startTime,
			EndTime:         endTime,
			Reason:          "ขอดูกล้องย้อนหลังผ่านระบบ Handshake",
			EvidenceFileURL: "HANDSHAKE_TICKET:" + ticketID,
			Status:          "PENDING",
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		go func(r *domain.CCTVRequest) {
			if createErr := u.repo.CreateRequest(r); createErr != nil {
				log.Printf("⚠️ [CCTV Request DB Log] Failed to record CCTV request history: %v", createErr)
			}
		}(cctvReq)
	}

	// 2. Extract Client Metadata for KPI and Audit Logging
	var ip, userAgent, platform string
	if len(meta) > 0 {
		ip = meta[0].IP
		userAgent = meta[0].UserAgent
		platform = meta[0].Platform
	}
	if platform == "" {
		platform = "mobile"
	}

	// 3. KPI Log: Track module usage in module_usage_logs
	if u.analyticsUC != nil {
		go func(uid uuid.UUID, clientIP, uAgent, plt string) {
			usageDTO := &domain.RecordModuleUsageDTO{
				ModuleCode: "cctv",
				Action:     "HANDSHAKE_TICKET_REQUEST",
				Platform:   plt,
			}
			if trackErr := u.analyticsUC.TrackModuleUsage(usageDTO, &uid, clientIP, uAgent); trackErr != nil {
				log.Printf("⚠️ [CCTV KPI Log] Failed to track module usage: %v", trackErr)
			}
		}(userID, ip, userAgent, platform)
	}

	// 4. Structured Security Audit Log
	log.Printf("[AUDIT] CCTV_HANDSHAKE_ISSUED: ticket=%s user_id=%s cctv_id=%s action=%s is_thaid_verified=%t ip=%s",
		ticketID, userID.String(), input.CCTVID, input.Action, isThaIDVerified, ip)

	portalURL := ""
	if u.cfg != nil {
		portalURL = u.cfg.CCTVPartnerPortalURL
	}
	if portalURL == "" {
		return nil, errors.New("CCTV_PARTNER_PORTAL_URL is not configured")
	}

	separator := "?"
	if strings.Contains(portalURL, "?") {
		separator = "&"
	}
	redirectURL := fmt.Sprintf("%s%sticket=%s", portalURL, separator, ticketID)

	return &domain.CreateHandshakeTicketResponse{
		Ticket:           ticketID,
		RedirectURL:      redirectURL,
		ExpiresInSeconds: int(ttl.Seconds()),
	}, nil
}

// VerifyHandshakeTicket validates and atomically invalidates a ticket (Channel 1 & Channel 2).
// OWASP Top 10 Hardened: Adaptive Native GETDEL/Lua Invalidation, Constant-time Compare, Zero-Alloc Validator.
func (u *cctvUseCase) VerifyHandshakeTicket(partnerHeaderKey string, input domain.VerifyTicketInput) (*domain.VerifyTicketResponse, error) {
	// 1. Authenticate Partner (Timing-attack resilient comparison for credentials and API keys)
	isAuthorized := false
	if u.cfg != nil {
		cidMatch := subtle.ConstantTimeCompare([]byte(input.ClientID), []byte(u.cfg.PH6ClientID)) == 1 && u.cfg.PH6ClientID != ""
		csecMatch := subtle.ConstantTimeCompare([]byte(input.ClientSecret), []byte(u.cfg.PH6ClientSecret)) == 1 && u.cfg.PH6ClientSecret != ""
		ph6Valid := cidMatch && csecMatch

		partnerKeyMatch := subtle.ConstantTimeCompare([]byte(input.ClientSecret), []byte(u.cfg.CCTVPartnerAPIKey)) == 1 && u.cfg.CCTVPartnerAPIKey != ""
		headerKeyMatch := subtle.ConstantTimeCompare([]byte(partnerHeaderKey), []byte(u.cfg.CCTVPartnerAPIKey)) == 1 && u.cfg.CCTVPartnerAPIKey != ""

		if (input.ClientID != "" && input.ClientSecret != "" && (ph6Valid || partnerKeyMatch)) || (partnerHeaderKey != "" && headerKeyMatch) {
			isAuthorized = true
		}
	}

	if !isAuthorized {
		return nil, ErrUnauthorizedPartner
	}

	// 2. Strict Input Validation (Zero-alloc byte comparison, OWASP A03 Injection Defense)
	ticket := strings.TrimSpace(input.Ticket)
	if !IsValidTicket(ticket) {
		return nil, ErrInvalidTicket
	}

	if u.redisClient == nil || u.redisClient.Raw() == nil {
		return nil, errors.New("cache service unavailable")
	}

	ctx := context.Background()
	ticketKey := "cctv:ticket:" + ticket

	// 3. Atomic Single-Use Invalidation (Adaptive GETDEL with Lua Script fallback)
	var ticketJSON string
	if u.supportsGetDel.Load() {
		res, err := u.redisClient.Raw().GetDel(ctx, ticketKey).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return nil, ErrInvalidTicket
			}
			if strings.Contains(strings.ToLower(err.Error()), "unknown command") {
				u.supportsGetDel.Store(false)
				luaRes, luaErr := getDelLuaScript.Run(ctx, u.redisClient.Raw(), []string{ticketKey}).Result()
				if luaErr != nil || luaRes == nil {
					return nil, ErrInvalidTicket
				}
				ticketJSON, _ = luaRes.(string)
			} else {
				return nil, ErrInvalidTicket
			}
		} else {
			ticketJSON = res
		}
	} else {
		luaRes, luaErr := getDelLuaScript.Run(ctx, u.redisClient.Raw(), []string{ticketKey}).Result()
		if luaErr != nil || luaRes == nil {
			return nil, ErrInvalidTicket
		}
		ticketJSON, _ = luaRes.(string)
	}

	if ticketJSON == "" {
		return nil, ErrInvalidTicket
	}

	var storedTicket domain.CCTVHandshakeTicket
	if err := json.Unmarshal([]byte(ticketJSON), &storedTicket); err != nil {
		return nil, ErrInvalidTicket
	}

	// 4. Verify TTL Expiration
	if time.Now().UTC().After(storedTicket.ExpiresAt) {
		return nil, ErrInvalidTicket
	}

	// 5. Data Minimization: Return assertion and pseudonymous user ref ONLY (No citizen ID)
	// The decision on footage download request approval resides entirely on the partner system.
	// Structured Security Audit Log:
	log.Printf("[AUDIT] CCTV_HANDSHAKE_VERIFIED: ticket=%s user_ref=%s is_thaid_verified=%t cctv_id=%s",
		ticket, storedTicket.UserReferenceID, storedTicket.IsThaIDVerified, storedTicket.CCTVID)

	// Output fields strictly match Ph6 Specification: success, user_identity, name
	return &domain.VerifyTicketResponse{
		Success:         true,
		UserIdentity:    storedTicket.UserReferenceID, // Required by Ph6: Unique ID of user
		Name:            storedTicket.DisplayName,     // Required by Ph6: Display Name
		UserReferenceID: storedTicket.UserReferenceID,
		UserStatus:      "active",
		AuthChannel:     storedTicket.AuthChannel,
		IsThaIDVerified: storedTicket.IsThaIDVerified,
		IssuedAt:        storedTicket.IssuedAt.Format(time.RFC3339),
		ExpiresAt:       storedTicket.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// ValidateCallbackURL validates that callback_url belongs to the whitelist (OWASP A01/A10 SSRF & Open Redirect).
func (u *cctvUseCase) ValidateCallbackURL(callbackURL string) bool {
	if callbackURL == "" {
		return false
	}

	parsed, err := url.Parse(callbackURL)
	if err != nil || !parsed.IsAbs() {
		return false
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return false
	}

	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return false
	}

	// SSRF Defense: Identify localhost and private/loopback/link-local IP addresses
	isPrivateHost := false
	if hostname == "localhost" {
		isPrivateHost = true
	} else if ip := net.ParseIP(hostname); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			isPrivateHost = true
		}
	} else {
		privatePrefixes := []string{"127.", "10.", "192.168.", "169.254."}
		for _, prefix := range privatePrefixes {
			if strings.HasPrefix(hostname, prefix) {
				isPrivateHost = true
				break
			}
		}
	}

	if u.cfg == nil {
		return false
	}

	cleanCallbackPath := path.Clean(parsed.Path)
	for _, allowed := range u.cfg.CCTVAllowedCallbacks {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		allowedParsed, err := url.Parse(allowed)
		if err != nil || !allowedParsed.IsAbs() {
			continue
		}
		if strings.EqualFold(parsed.Scheme, allowedParsed.Scheme) &&
			strings.EqualFold(parsed.Host, allowedParsed.Host) &&
			cleanCallbackPath == path.Clean(allowedParsed.Path) {
			// If it's a private host/localhost, it is only allowed if explicitly configured in the allowed whitelist
			if isPrivateHost {
				if strings.EqualFold(allowedParsed.Hostname(), hostname) {
					return true
				}
				return false
			}
			return true
		}
	}

	return false
}

// CreateWebHandshakeTicket generates a ticket after ThaID web login and returns redirect target.
func (u *cctvUseCase) CreateWebHandshakeTicket(userID uuid.UUID, callbackURL, partnerID string) (string, error) {
	if !u.ValidateCallbackURL(callbackURL) {
		return "", ErrInvalidCallbackURL
	}

	// Strict check: User MUST exist in our database and be ThaID verified
	user, isThaIDVerified, err := u.getVerifiedUser(userID)
	if err != nil {
		return "", err
	}
	if !isThaIDVerified {
		return "", ErrThaIDRequired
	}

	displayName := ""
	if user.Information != nil {
		displayName = strings.TrimSpace(user.Information.Name + " " + user.Information.LastName)
	}
	if displayName == "" && len(user.OauthAccounts) > 0 {
		for _, acc := range user.OauthAccounts {
			if strings.TrimSpace(acc.DisplayName) != "" {
				displayName = strings.TrimSpace(acc.DisplayName)
				break
			}
		}
	}

	ticketBytes := make([]byte, 32)
	if _, err := rand.Read(ticketBytes); err != nil {
		return "", fmt.Errorf("failed to generate ticket entropy: %w", err)
	}
	ticketID := "TKT_" + hex.EncodeToString(ticketBytes)

	ttl := 60 * time.Second
	if u.cfg != nil && u.cfg.CCTVTicketTTLSeconds > 0 {
		ttl = time.Duration(u.cfg.CCTVTicketTTLSeconds) * time.Second
	}

	now := time.Now().UTC()
	ticket := domain.CCTVHandshakeTicket{
		TicketID:        ticketID,
		UserReferenceID: "usr_" + userID.String(),
		DisplayName:     displayName,
		AuthChannel:     "thaid",
		IsThaIDVerified: true,
		Action:          "download",
		IssuedAt:        now,
		ExpiresAt:       now.Add(ttl),
	}

	if u.redisClient != nil {
		ticketKey := "cctv:ticket:" + ticketID
		if err := u.redisClient.SetJSON(context.Background(), ticketKey, ticket, ttl); err != nil {
			return "", err
		}
	}

	separator := "?"
	if strings.Contains(callbackURL, "?") {
		separator = "&"
	}
	return fmt.Sprintf("%s%sticket=%s", callbackURL, separator, ticketID), nil
}

func (u *cctvUseCase) RecordViewLog(input domain.CreateCCTVLogInput, userID *uuid.UUID, clientIP, userAgent string) error {
	if input.DurationSeconds < 3 {
		return nil // Ignore noise (< 3s)
	}

	var cctvUUID uuid.UUID
	var err error
	if input.CCTVID != "" {
		cctvUUID, err = uuid.Parse(input.CCTVID)
	}

	if err != nil || cctvUUID == uuid.Nil {
		if cam, findErr := u.repo.FindCameraByIDOrName(input.CCTVID); findErr == nil && cam != nil {
			cctvUUID = cam.ID
		}
	}

	if cctvUUID == uuid.Nil {
		return errors.New("cctv device not found")
	}

	now := time.Now()
	if input.StartedAt.IsZero() {
		input.StartedAt = now.Add(-time.Duration(input.DurationSeconds) * time.Second)
	}
	if input.EndedAt == nil {
		end := now
		input.EndedAt = &end
	}

	if input.DurationSeconds > 28800 {
		input.DurationSeconds = 28800
	}

	device := input.DeviceType
	if device == "" {
		device = "Mobile App"
	}

	policyType := input.PolicyType
	if policyType == "" {
		policyType = "UserPolicy"
	}

	viewLog := &domain.CCTVViewLog{
		ID:              uuid.New(),
		CCTVID:          cctvUUID,
		UserID:          userID,
		PolicyType:      policyType,
		DeviceType:      device,
		StartedAt:       input.StartedAt,
		EndedAt:         input.EndedAt,
		DurationSeconds: input.DurationSeconds,
		IPAddress:       clientIP,
		UserAgent:       userAgent,
		CreatedAt:       time.Now(),
	}

	return u.repo.CreateViewLog(viewLog)
}
