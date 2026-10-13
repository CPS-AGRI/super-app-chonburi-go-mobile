package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/infrastructure"

	"github.com/google/uuid"
)

var (
	ErrThaIDRequired       = errors.New("THAID_VERIFICATION_REQUIRED")
	ErrInvalidTicket       = errors.New("INVALID_OR_EXPIRED_TICKET")
	ErrUnauthorizedPartner = errors.New("UNAUTHORIZED_PARTNER")
	ErrRateLimitExceeded   = errors.New("RATE_LIMIT_EXCEEDED")
	ErrInvalidCallbackURL  = errors.New("INVALID_CALLBACK_URL")
	ErrUserNotFound        = errors.New("USER_NOT_FOUND_IN_SYSTEM")
	ticketFormatRegex      = regexp.MustCompile(`^TKT_[a-f0-9]{64}$`)
)

type cctvUseCase struct {
	repo        domain.MobileCCTVRepository
	authRepo    domain.AuthRepository
	cfg         *config.Config
	redisClient *infrastructure.RedisClient
}

// NewCCTVUseCase creates a new MobileCCTVUseCase with unified ticket handshake capabilities.
func NewCCTVUseCase(
	repo domain.MobileCCTVRepository,
	authRepo domain.AuthRepository,
	cfg *config.Config,
	redisClient *infrastructure.RedisClient,
) domain.MobileCCTVUseCase {
	return &cctvUseCase{
		repo:        repo,
		authRepo:    authRepo,
		cfg:         cfg,
		redisClient: redisClient,
	}
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
func (u *cctvUseCase) CreateHandshakeTicket(userID uuid.UUID, input domain.CreateHandshakeTicketInput) (*domain.CreateHandshakeTicketResponse, error) {
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
// OWASP Top 10 Hardened: Single-use Lua atomic get+del, Strict Regex, Zero Sensitive PII.
func (u *cctvUseCase) VerifyHandshakeTicket(partnerHeaderKey string, input domain.VerifyTicketInput) (*domain.VerifyTicketResponse, error) {
	// 1. Authenticate Partner (support both Ph6 client_id/client_secret in body and X-Partner-API-Key in header)
	isAuthorized := false
	if input.ClientID != "" && input.ClientSecret != "" {
		if (u.cfg != nil && u.cfg.PH6ClientID != "" && input.ClientID == u.cfg.PH6ClientID && input.ClientSecret == u.cfg.PH6ClientSecret) ||
			(u.cfg != nil && u.cfg.CCTVPartnerAPIKey != "" && input.ClientSecret == u.cfg.CCTVPartnerAPIKey) {
			isAuthorized = true
		}
	} else if partnerHeaderKey != "" && u.cfg != nil && u.cfg.CCTVPartnerAPIKey != "" && partnerHeaderKey == u.cfg.CCTVPartnerAPIKey {
		isAuthorized = true
	}

	if !isAuthorized {
		return nil, ErrUnauthorizedPartner
	}

	// 2. Strict Input Validation (OWASP A03 Injection Defense)
	ticket := strings.TrimSpace(input.Ticket)
	if !ticketFormatRegex.MatchString(ticket) {
		return nil, ErrInvalidTicket
	}

	if u.redisClient == nil || u.redisClient.Raw() == nil {
		return nil, errors.New("cache service unavailable")
	}

	ctx := context.Background()
	ticketKey := "cctv:ticket:" + ticket

	// 3. Atomic Single-Use Invalidation (OWASP A04 Anti-Replay Attack)
	luaScript := `
		local val = redis.call("GET", KEYS[1])
		if val then
			redis.call("DEL", KEYS[1])
		end
		return val
	`
	res, err := u.redisClient.Raw().Eval(ctx, luaScript, []string{ticketKey}).Result()
	if err != nil || res == nil {
		return nil, ErrInvalidTicket
	}

	ticketJSON, ok := res.(string)
	if !ok || ticketJSON == "" {
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

	canDownload := strings.EqualFold(storedTicket.Action, "download") && storedTicket.IsThaIDVerified

	var timeWindow *domain.TimeWindow
	if storedTicket.StartTime != "" || storedTicket.EndTime != "" {
		timeWindow = &domain.TimeWindow{
			StartTime: storedTicket.StartTime,
			EndTime:   storedTicket.EndTime,
		}
	}

	// 5. Data Minimization: Return assertion and pseudonymous user ref ONLY (No citizen ID)
	// Output fields strictly match Ph6 Specification: success, user_identity, name
	return &domain.VerifyTicketResponse{
		Success:         true,
		UserIdentity:    storedTicket.UserReferenceID, // Required by Ph6: Unique ID of user
		Name:            storedTicket.DisplayName,     // Required by Ph6: Display Name
		UserReferenceID: storedTicket.UserReferenceID,
		UserStatus:      "active",
		AuthChannel:     storedTicket.AuthChannel,
		IsThaIDVerified: storedTicket.IsThaIDVerified,
		Permissions: domain.TicketPermissions{
			AllowedCCTVID: storedTicket.CCTVID,
			CanViewLive:   true,
			CanDownload:   canDownload,
			TimeWindow:    timeWindow,
		},
		IssuedAt:  storedTicket.IssuedAt.Format(time.RFC3339),
		ExpiresAt: storedTicket.ExpiresAt.Format(time.RFC3339),
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

	for _, allowed := range u.cfg.CCTVAllowedCallbacks {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		if strings.HasPrefix(callbackURL, allowed) {
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
