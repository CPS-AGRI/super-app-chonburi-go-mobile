package http

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/infrastructure"
	"super-app-chonburi-go-mobile/internal/usecase"
	"super-app-chonburi-go-mobile/pkg/jwtutil"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/skip2/go-qrcode"
)

type blockedEntry struct {
	expiresAt time.Time
}

type cctvHandler struct {
	useCase     domain.MobileCCTVUseCase
	authUseCase domain.AuthUseCase
	cfg         *config.Config
	redisClient *infrastructure.RedisClient
	blocklist   sync.Map // key: rate limit key, value: blockedEntry
}

// NewCCTVHandler registers the Mobile CCTV & Unified Handshake Ticket endpoints.
func NewCCTVHandler(
	app *fiber.App,
	useCase domain.MobileCCTVUseCase,
	cfg *config.Config,
	redisClient *infrastructure.RedisClient,
	authUseCase ...domain.AuthUseCase,
) {
	var auc domain.AuthUseCase
	if len(authUseCase) > 0 {
		auc = authUseCase[0]
	}
	handler := &cctvHandler{
		useCase:     useCase,
		authUseCase: auc,
		cfg:         cfg,
		redisClient: redisClient,
	}

	// 1. Mobile App Endpoints (JWT Protected)
	mobileGroup := app.Group("/api/v1/mobile/cctv", jwtutil.RequireAuth(cfg))
	mobileGroup.Get("/", handler.GetCameras)
	mobileGroup.Post("/requests", handler.CreateRequest)
	mobileGroup.Get("/requests/my-requests", handler.GetMyRequests)
	mobileGroup.Post("/handshake-ticket", handler.CreateHandshakeTicket) // Channel 1

	// 2. Partner Server-to-Server Endpoints (Ph6 Direct Spec & Alias)
	app.Post("/api/v1/verify-ticket", handler.VerifyTicket)               // Exact Ph6 Next.js endpoint
	app.Post("/api/v1/external/cctv/verify-ticket", handler.VerifyTicket) // Alias for backwards compatibility

	// 3. Web ThaID Login Flow for Partner (Channel 2)
	app.Get("/sso/login", handler.RenderThaIDLoginPage)              // Exact Ph6 Next.js SSO URL
	app.Get("/login/cctv/thaid", handler.RenderThaIDLoginPage)       // Alias
	app.Get("/sso/callback", handler.HandleThaIDCallback)            // Callback handler after ThaID approval
	app.Get("/sso/simulate-callback", handler.SimulateThaIDCallback) // Dev / UAT Bypass Simulator

	// 4. Mobile CCTV Telemetry (Optional Auth - Citizen / Guest)
	app.Post("/api/v1/mobile/cctv/logs", handler.CreateViewLog)
	app.Post("/mobile/cctv/logs", handler.CreateViewLog)
	app.Post("/api/v1/cctv/logs", handler.CreateViewLog)
	app.Post("/cctv/logs", handler.CreateViewLog)
}

func (h *cctvHandler) checkRateLimit(key string, maxRequests int, window time.Duration) bool {
	// L1 Check: Fast in-memory blocklist check with lazy eviction
	if v, ok := h.blocklist.Load(key); ok {
		if entry, ok := v.(blockedEntry); ok {
			if time.Now().Before(entry.expiresAt) {
				return false // Short-circuit blocked request without hitting Redis
			}
		}
		h.blocklist.Delete(key) // Evict expired entry
	}

	// Fail-secure: Deny access if Redis client is unavailable
	if h.redisClient == nil || h.redisClient.Raw() == nil {
		return false
	}

	ctx := context.Background()
	pipe := h.redisClient.Raw().Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false // Fail-secure on Redis error
	}

	if incr.Val() > int64(maxRequests) {
		// Populate L1 in-memory blocklist for the remainder of the window
		h.blocklist.Store(key, blockedEntry{expiresAt: time.Now().Add(window)})
		return false
	}

	return true
}

// CreateHandshakeTicket [Channel 1]
// Mobile App calls this endpoint to get a single-use ticket after validating ThaID status.
func (h *cctvHandler) CreateHandshakeTicket(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}

	// OWASP Anti-Abuse: Configurable Rate Limit per minute per user (default 10)
	handshakeLimit := 10
	if h.cfg != nil && h.cfg.CCTVHandshakeRateLimit > 0 {
		handshakeLimit = h.cfg.CCTVHandshakeRateLimit
	}
	rateKey := fmt.Sprintf("ratelimit:cctv:handshake:%s", userID.String())
	if !h.checkRateLimit(rateKey, handshakeLimit, time.Minute) {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"success":    false,
			"error_code": "RATE_LIMIT_EXCEEDED",
			"message":    "คำขอเกินขีดจำกัดความปลอดภัย กรุณาลองใหม่อีกครั้งในภายหลัง",
		})
	}

	var input domain.CreateHandshakeTicketInput
	if err := c.Bind().JSON(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	meta := domain.RequestMetadata{
		IP:        c.IP(),
		UserAgent: string(c.Request().Header.UserAgent()),
		Platform:  "mobile",
	}

	res, err := h.useCase.CreateHandshakeTicket(userID, input, meta)
	if err != nil {
		if errors.Is(err, usecase.ErrUserNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success":    false,
				"error_code": "USER_NOT_FOUND_IN_SYSTEM",
				"message":    "ไม่พบข้อมูลผู้ใช้งานในระบบชลบุรีพลัส กรุณาสมัครสมาชิกหรือลงทะเบียนผ่านแอป ชลบุรีคอนเน็กซ์ ก่อนใช้งาน",
			})
		}
		if errors.Is(err, usecase.ErrThaIDRequired) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"success":    false,
				"error_code": "THAID_VERIFICATION_REQUIRED",
				"message":    "การขอดูกล้องย้อนหลังจำเป็นต้องยืนยันตัวตนผ่าน ThaID กรุณายืนยันตัวตนก่อนดำเนินการ",
			})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    res,
	})
}

// VerifyTicket [Server-to-Server]
// CCTV Partner Backend calls this endpoint to verify and atomically invalidate a ticket.
// Supports both Ph6 client_id/client_secret in JSON body and X-Partner-API-Key in header.
func (h *cctvHandler) VerifyTicket(c fiber.Ctx) error {
	// OWASP Anti-DDoS: Configurable Rate Limit per minute per partner IP (default 60)
	verifyLimit := 60
	if h.cfg != nil && h.cfg.CCTVVerifyRateLimit > 0 {
		verifyLimit = h.cfg.CCTVVerifyRateLimit
	}
	clientIP := c.IP()
	rateKey := fmt.Sprintf("ratelimit:cctv:verify:%s", clientIP)
	if !h.checkRateLimit(rateKey, verifyLimit, time.Minute) {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"success":    false,
			"error_code": "RATE_LIMIT_EXCEEDED",
			"message":    "คำขอเกินขีดจำกัดความปลอดภัย กรุณาลองใหม่อีกครั้งในภายหลัง",
		})
	}

	var input domain.VerifyTicketInput
	if err := c.Bind().JSON(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success":    false,
			"error_code": "INVALID_OR_EXPIRED_TICKET",
			"message":    "invalid request body",
		})
	}

	partnerHeaderKey := c.Get("X-Partner-API-Key")
	res, err := h.useCase.VerifyHandshakeTicket(partnerHeaderKey, input)
	if err != nil {
		if errors.Is(err, usecase.ErrUnauthorizedPartner) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success":    false,
				"error_code": "UNAUTHORIZED_PARTNER",
				"message":    "Client credentials or X-Partner-API-Key is invalid or missing",
			})
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success":    false,
			"error_code": "INVALID_OR_EXPIRED_TICKET",
			"message":    "ตั๋วการเข้าถึงนี้ไม่ถูกต้อง หมดอายุ หรือถูกใช้งานไปแล้ว",
		})
	}

	return c.JSON(fiber.Map{
		"success":       res.Success,
		"user_identity": res.UserIdentity,
		"name":          res.Name,
		"data":          res,
	})
}

// RenderThaIDLoginPage [Channel 2]
// Redirects the citizen's browser directly to DOPA Official ThaID Digital-ID login gateway.
// Official DOPA gateway renders AUTHEN-{txID} QR code with WebSocket live synchronization.
func (h *cctvHandler) RenderThaIDLoginPage(c fiber.Ctx) error {
	callbackURL := c.Query("redirect_to")
	if callbackURL == "" {
		callbackURL = c.Query("callback_url")
	}
	partnerID := c.Query("partner_id")
	mode := c.Query("mode")

	// Validate Strict Callback Whitelist (OWASP A01 Open Redirect & A10 SSRF Defense)
	if callbackURL == "" || !h.useCase.ValidateCallbackURL(callbackURL) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid or unauthorized callback URL")
	}

	// ThaiD OAuth Configuration for UAT Chonburi (DOPA Auth Gateway)
	clientID := "RnRBS0JOemx2dUZBSlhmVnNLcXU0VDM5T0J0Y1FDSTY"
	if h.cfg != nil && h.cfg.ThaiIDClientID != "" {
		clientID = h.cfg.ThaiIDClientID
	}
	redirectURI := c.BaseURL() + "/sso/callback"
	scopes := "pid name"
	state := fmt.Sprintf("cctv_%s_%d", partnerID, time.Now().UnixNano())

	// Persist session state in Redis (TTL: 10 minutes)
	if h.redisClient != nil {
		sessionData := map[string]string{
			"callback_url": callbackURL,
			"partner_id":   partnerID,
		}
		_ = h.redisClient.SetJSON(context.Background(), fmt.Sprintf("cctv_state:%s", state), sessionData, 10*time.Minute)
	}

	gatewayBase := "https://imauth.bora.dopa.go.th/api/v2/oauth2/auth/"
	if h.cfg != nil && h.cfg.ThaiIDQRGatewayURL != "" {
		gatewayBase = h.cfg.ThaiIDQRGatewayURL
	}
	separator := "?"
	if strings.Contains(gatewayBase, "?") {
		separator = "&"
	}
	dopaAuthURL := fmt.Sprintf("%s%sresponse_type=code&client_id=%s&redirect_uri=%s&scope=%s&state=%s",
		gatewayBase,
		separator,
		url.QueryEscape(clientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(scopes),
		url.QueryEscape(state),
	)

	// Mode "simulate" renders the dev/tester bypass page
	if mode == "simulate" {
		simURL := fmt.Sprintf("/sso/simulate-callback?callback_url=%s&partner_id=%s",
			url.QueryEscape(callbackURL),
			url.QueryEscape(partnerID),
		)
		qrBytes, _ := qrcode.Encode(dopaAuthURL, qrcode.Medium, 256)
		qrImgSrc := "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrBytes)

		html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="th">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>เข้าสู่ระบบด้วย ThaID — ชลบุรีพลัส (Chonburi Plus)</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Prompt:wght@300;400;500;600;700&display=swap" rel="stylesheet">
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; font-family: 'Prompt', -apple-system, sans-serif; }
    body {
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      background: linear-gradient(135deg, #0f172a 0%%, #1e3a8a 50%%, #0369a1 100%%);
      color: #f8fafc;
      padding: 20px;
    }
    .glass-card {
      background: rgba(255, 255, 255, 0.08);
      backdrop-filter: blur(20px);
      -webkit-backdrop-filter: blur(20px);
      border: 1px solid rgba(255, 255, 255, 0.18);
      border-radius: 28px;
      padding: 36px 32px;
      max-width: 440px;
      width: 100%%;
      text-align: center;
      box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
    }
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      background: rgba(56, 189, 248, 0.15);
      border: 1px solid rgba(56, 189, 248, 0.3);
      color: #38bdf8;
      font-size: 12px;
      font-weight: 600;
      padding: 6px 14px;
      border-radius: 9999px;
      margin-bottom: 16px;
    }
    .badge-dot { width: 8px; height: 8px; border-radius: 50%%; background: #38bdf8; }
    h1 { font-size: 23px; font-weight: 700; margin-bottom: 6px; color: #ffffff; }
    p.subtitle { font-size: 13.5px; color: #cbd5e1; margin-bottom: 22px; line-height: 1.5; }
    .qr-container {
      background: #ffffff;
      padding: 16px;
      border-radius: 20px;
      display: inline-block;
      box-shadow: 0 10px 25px rgba(0, 0, 0, 0.3);
      margin-bottom: 16px;
    }
    .qr-img { width: 220px; height: 220px; display: block; border-radius: 6px; }
    .sim-btn-wrapper { margin-bottom: 18px; }
    .sim-btn {
      display: inline-block;
      width: 100%%;
      padding: 12px 16px;
      background: linear-gradient(135deg, rgba(14, 165, 233, 0.35) 0%%, rgba(2, 132, 199, 0.5) 100%%);
      border: 1px solid rgba(56, 189, 248, 0.4);
      border-radius: 14px;
      color: #ffffff;
      text-decoration: none;
      font-size: 13px;
      font-weight: 600;
      transition: all 0.2s ease;
      cursor: pointer;
    }
  </style>
</head>
<body>
  <div class="glass-card">
    <div class="badge"><span class="badge-dot"></span>จำลองระบบยืนยันตัวตน (Dev Bypass)</div>
    <h1>จำลองการเข้าสู่ระบบ</h1>
    <p class="subtitle">โหมดทดสอบสำหรับนักพัฒนาโดยไม่ต้องใช้โทรศัพท์จริง</p>
    <div class="qr-container"><img class="qr-img" src="%s" alt="ThaID QR"></div>
    <div class="sim-btn-wrapper"><a href="%s" class="sim-btn">🧪 จำลองการสแกนผ่าน ThaID สำเร็จ</a></div>
  </div>
</body>
</html>`, qrImgSrc, simURL)
		return c.SendString(html)
	}

	// Default: Direct redirect to official DOPA ThaID Digital-ID gateway
	log.Printf("🔗 [SSO Login] Redirecting citizen to DOPA Official Gateway: %s", dopaAuthURL)
	return c.Redirect().To(dopaAuthURL)
}

// HandleThaIDCallback [Channel 2]
// Handles the redirect after citizen approves on the official ThaID mobile application.
func (h *cctvHandler) HandleThaIDCallback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	if code == "" || state == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Missing authorization code or state")
	}

	// 1. Retrieve session state from Redis
	stateKey := fmt.Sprintf("cctv_state:%s", state)
	var session struct {
		CallbackURL string `json:"callback_url"`
		PartnerID   string `json:"partner_id"`
	}

	if h.redisClient != nil {
		if err := h.redisClient.GetJSON(context.Background(), stateKey, &session); err != nil {
			log.Printf("⚠️ [SSO Callback] Expired or invalid state: %s (%v)", state, err)
		} else {
			_ = h.redisClient.Delete(context.Background(), stateKey)
		}
	}

	if session.CallbackURL == "" {
		session.CallbackURL = "http://localhost:3000/portal"
	}
	if session.PartnerID == "" {
		session.PartnerID = "ph6"
	}

	redirectURI := c.BaseURL() + "/sso/callback"

	if h.authUseCase == nil {
		log.Printf("❌ [SSO Callback] authUseCase not injected")
		return c.Status(fiber.StatusInternalServerError).SendString("Authentication system unavailable")
	}

	authResp, err := h.authUseCase.LoginWithThaiID(code, redirectURI)
	if err != nil {
		log.Printf("❌ [SSO Callback] LoginWithThaiID failed: %v", err)
		return c.Status(fiber.StatusUnauthorized).SendString(fmt.Sprintf("ThaID verification failed: %v", err))
	}

	if authResp == nil || authResp.User == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("User not verified")
	}

	// 3. Create Web Handshake Ticket
	redirectTarget, err := h.useCase.CreateWebHandshakeTicket(authResp.User.ID, session.CallbackURL, session.PartnerID)
	if err != nil {
		log.Printf("❌ [SSO Callback] CreateWebHandshakeTicket failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString(fmt.Sprintf("Failed to generate ticket: %v", err))
	}

	log.Printf("✅ [SSO Callback] ThaID Login Success for User %s. Redirecting to: %s", authResp.User.ID, redirectTarget)
	return c.Redirect().To(redirectTarget)
}

// SimulateThaIDCallback [Dev/UAT Simulator]
// Allows developers/testers to simulate a successful ThaID QR scan without mobile devices.
func (h *cctvHandler) SimulateThaIDCallback(c fiber.Ctx) error {
	callbackURL := c.Query("redirect_to")
	if callbackURL == "" {
		callbackURL = c.Query("callback_url")
	}
	partnerID := c.Query("partner_id")

	if callbackURL == "" || !h.useCase.ValidateCallbackURL(callbackURL) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid or unauthorized callback URL")
	}

	// Use Somchai (ThaID verified test user in UAT DB)
	testUserID, _ := uuid.Parse("1a5152bd-d98f-448c-865c-6f74fe47e1ea")
	targetURL, err := h.useCase.CreateWebHandshakeTicket(testUserID, callbackURL, partnerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate simulation ticket: " + err.Error())
	}

	return c.Redirect().To(targetURL)
}

func (h *cctvHandler) GetCameras(c fiber.Ctx) error {
	latMin, _ := strconv.ParseFloat(c.Query("lat_min"), 64)
	latMax, _ := strconv.ParseFloat(c.Query("lat_max"), 64)
	lngMin, _ := strconv.ParseFloat(c.Query("lng_min"), 64)
	lngMax, _ := strconv.ParseFloat(c.Query("lng_max"), 64)

	query := domain.MobileCCTVQuery{
		LatMin: latMin,
		LatMax: latMax,
		LngMin: lngMin,
		LngMax: lngMax,
	}

	cameras, err := h.useCase.GetCameras(query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "data": cameras})
}

type createRequestInput struct {
	CCTVID          string `json:"cctv_id"`
	IncidentDate    string `json:"incident_date"`
	StartTime       string `json:"start_time"`
	EndTime         string `json:"end_time"`
	Reason          string `json:"reason"`
	EvidenceFileURL string `json:"evidence_file_url"`
}

func (h *cctvHandler) CreateRequest(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}

	var input createRequestInput
	if err := c.Bind().JSON(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	cctvID, err := uuid.Parse(input.CCTVID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid camera ID"})
	}

	incidentDate, err := time.Parse("2006-01-02", input.IncidentDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid incident_date format, must be YYYY-MM-DD"})
	}

	req := &domain.CCTVRequest{
		UserID:          userID,
		CCTVID:          cctvID,
		IncidentDate:    incidentDate,
		StartTime:       input.StartTime,
		EndTime:         input.EndTime,
		Reason:          input.Reason,
		EvidenceFileURL: input.EvidenceFileURL,
	}

	if err := h.useCase.CreateRequest(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "CCTV request submitted successfully", "data": req})
}

func (h *cctvHandler) GetMyRequests(c fiber.Ctx) error {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}

	requests, err := h.useCase.GetMyRequests(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "data": requests})
}

// CreateViewLog handles citizen mobile camera view session telemetry.
// Allows both authenticated users (extracts userID from JWT) and guest users.
func (h *cctvHandler) CreateViewLog(c fiber.Ctx) error {
	var input domain.CreateCCTVLogInput
	if err := c.Bind().JSON(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "invalid request body",
		})
	}

	if input.CCTVID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "cctv_id is required",
		})
	}

	// Try extracting user_id if token provided (optional authentication)
	var userUUID *uuid.UUID
	authHeader := c.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") && h.cfg != nil {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if claims, err := jwtutil.ParseMobileToken(tokenStr, h.cfg.JWTSecret); err == nil && claims.UserID != "" {
			if parsed, pErr := uuid.Parse(claims.UserID); pErr == nil {
				userUUID = &parsed
			}
		}
	}

	clientIP := c.IP()
	userAgent := c.Get("User-Agent")

	if err := h.useCase.RecordViewLog(input, userUUID, clientIP, userAgent); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": "view log recorded successfully",
	})
}
