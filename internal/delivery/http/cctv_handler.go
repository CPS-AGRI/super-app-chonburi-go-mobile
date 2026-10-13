package http

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/internal/infrastructure"
	"super-app-chonburi-go-mobile/internal/usecase"
	"super-app-chonburi-go-mobile/pkg/jwtutil"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type cctvHandler struct {
	useCase     domain.MobileCCTVUseCase
	cfg         *config.Config
	redisClient *infrastructure.RedisClient
}

// NewCCTVHandler registers the Mobile CCTV & Unified Handshake Ticket endpoints.
func NewCCTVHandler(app *fiber.App, useCase domain.MobileCCTVUseCase, cfg *config.Config, redisClient *infrastructure.RedisClient) {
	handler := &cctvHandler{
		useCase:     useCase,
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

	// 3. Web ThaID Login Landing Page for Partner (Channel 2)
	app.Get("/sso/login", handler.RenderThaIDLoginPage)        // Exact Ph6 Next.js SSO URL
	app.Get("/login/cctv/thaid", handler.RenderThaIDLoginPage) // Alias
}

func (h *cctvHandler) checkRateLimit(key string, maxRequests int, window time.Duration) bool {
	if h.redisClient == nil || h.redisClient.Raw() == nil {
		return true // Fallback if Redis is not configured
	}
	ctx := context.Background()
	pipe := h.redisClient.Raw().Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return true
	}
	return incr.Val() <= int64(maxRequests)
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

	res, err := h.useCase.CreateHandshakeTicket(userID, input)
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
// Renders an Enterprise-grade Glassmorphism ThaID QR Login Web Page with OWASP Security Headers.
func (h *cctvHandler) RenderThaIDLoginPage(c fiber.Ctx) error {
	callbackURL := c.Query("redirect_to")
	if callbackURL == "" {
		callbackURL = c.Query("callback_url")
	}
	partnerID := c.Query("partner_id")

	// Validate Strict Callback Whitelist (OWASP A01 Open Redirect & A10 SSRF Defense)
	if callbackURL == "" || !h.useCase.ValidateCallbackURL(callbackURL) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid or unauthorized callback URL")
	}

	// OWASP Top 10 Security Headers
	c.Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline' https:; frame-ancestors 'none';")
	c.Set("X-Frame-Options", "DENY")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	c.Set("Content-Type", "text/html; charset=utf-8")

	qrGatewayURL := "https://thaid.dopa.go.th/oauth/authorize"
	if h.cfg != nil && h.cfg.ThaiIDQRGatewayURL != "" {
		qrGatewayURL = h.cfg.ThaiIDQRGatewayURL
	}
	qrDataURL := fmt.Sprintf("%s?partner=%s", qrGatewayURL, partnerID)

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
      border: 1.5px solid rgba(255, 255, 255, 0.2);
      border-radius: 28px;
      padding: 40px 32px;
      max-width: 440px;
      width: 100%%;
      box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
      text-align: center;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 6px 14px;
      background: rgba(56, 189, 248, 0.15);
      border: 1px solid rgba(56, 189, 248, 0.3);
      border-radius: 9999px;
      font-size: 13px;
      font-weight: 500;
      color: #38bdf8;
      margin-bottom: 20px;
    }
    .badge-dot { width: 8px; height: 8px; border-radius: 50%%; background: #38bdf8; animation: pulse 2s infinite; }
    @keyframes pulse { 0%%, 100%% { opacity: 1; transform: scale(1); } 50%% { opacity: 0.4; transform: scale(0.85); } }
    h1 { font-size: 24px; font-weight: 700; margin-bottom: 8px; color: #ffffff; }
    p.subtitle { font-size: 14px; color: #cbd5e1; margin-bottom: 28px; line-height: 1.5; }
    .qr-container {
      background: #ffffff;
      padding: 20px;
      border-radius: 20px;
      display: inline-block;
      box-shadow: 0 10px 25px rgba(0, 0, 0, 0.3);
      margin-bottom: 20px;
    }
    .qr-img { width: 220px; height: 220px; display: block; border-radius: 8px; }
    .timer-box { font-size: 13px; color: #94a3b8; margin-bottom: 24px; }
    .timer-val { color: #f59e0b; font-weight: 600; }
    .pdpa-notice {
      background: rgba(15, 23, 42, 0.4);
      border: 1px solid rgba(255, 255, 255, 0.1);
      border-radius: 16px;
      padding: 14px;
      font-size: 12px;
      color: #94a3b8;
      text-align: left;
      line-height: 1.6;
    }
    .pdpa-notice strong { color: #e2e8f0; }
  </style>
</head>
<body>
  <div class="glass-card">
    <div class="badge">
      <span class="badge-dot"></span>
      ระบบยืนยันตัวตนกลาง อบจ. ชลบุรี
    </div>
    <h1>สแกนเพื่อยืนยันตัวตน</h1>
    <p class="subtitle">เปิดแอปพลิเคชัน <strong>ThaID</strong> บนสมาร์ตโฟนของคุณ<br>เพื่อยืนยันตัวตนเข้าสู่ระบบกล้องวงจรปิด (CCTV)</p>
    
    <div class="qr-container">
      <img class="qr-img" src="https://api.qrserver.com/v1/create-qr-code/?size=220x220&data=%s" alt="ThaID QR Code">
    </div>

    <div class="timer-box">
      รหัส QR มีอายุใช้งานอีก <span class="timer-val" id="countdown">180</span> วินาที
    </div>

    <div class="pdpa-notice">
      <strong>🔒 การคุ้มครองข้อมูลส่วนบุคคล (PDPA):</strong><br>
      ระบบส่งต่อเฉพาะสถานะการรับรองตัวตนและรหัสอ้างอิงความปลอดภัย (User Ref) โดย<strong>ไม่มีการส่งเลขบัตรประชาชน 13 หลัก</strong>ไปยังระบบภายนอก
    </div>
  </div>

  <script>
    let seconds = 180;
    const timerElem = document.getElementById('countdown');
    const timer = setInterval(() => {
      seconds--;
      if (seconds <= 0) {
        clearInterval(timer);
        timerElem.textContent = 'หมดอายุ';
      } else {
        timerElem.textContent = seconds;
      }
    }, 1000);
  </script>
</body>
</html>`, qrDataURL)

	return c.SendString(html)
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
