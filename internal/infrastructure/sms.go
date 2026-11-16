package infrastructure

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

var thaiMobileRegex = regexp.MustCompile(`^0[689]\d{8}$`)

// ErrInvalidSMSDestination indicates the phone number itself is invalid or rejected as -109 by dtac.
var ErrInvalidSMSDestination = errors.New("invalid Thai mobile number")

type SMSService interface {
	SendOTP(ctx context.Context, phoneNumber, otpCode, ref string) error
}

// SMSClientOption configures optional features (such as HMAC fallback relay) on SMSClient.
type SMSClientOption func(*SMSClient)

type SMSClient struct {
	gatewayURL   string
	username     string
	password     string
	senderName   string
	relayEnabled bool
	relayURL     string
	relaySecret  string
	httpClient   *http.Client
}

// WithSMSRelay configures the fallback relay.
func WithSMSRelay(enabled bool, relayURL, relaySecret string) SMSClientOption {
	return func(s *SMSClient) {
		s.relayEnabled = enabled
		s.relayURL = strings.TrimRight(strings.TrimSpace(relayURL), "/")
		s.relaySecret = strings.TrimSpace(relaySecret)
	}
}

// WithHTTPClient overrides the HTTP client (primarily for deterministic unit testing).
func WithHTTPClient(client *http.Client) SMSClientOption {
	return func(s *SMSClient) {
		if client != nil {
			s.httpClient = client
		}
	}
}

func NewSMSClient(gatewayURL, username, password, senderName string, opts ...SMSClientOption) *SMSClient {
	client := &SMSClient{
		gatewayURL: strings.TrimSpace(gatewayURL),
		username:   strings.TrimSpace(username),
		password:   password,
		senderName: strings.TrimSpace(senderName),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(client)
		}
	}
	return client
}

// NormalizeThaiMobile strips whitespace/hyphens and validates Thai 10-digit mobile numbers.
func NormalizeThaiMobile(phone string) (string, error) {
	clean := strings.TrimSpace(phone)
	clean = strings.ReplaceAll(clean, "-", "")
	clean = strings.ReplaceAll(clean, " ", "")
	if strings.HasPrefix(clean, "+66") && len(clean) == 12 {
		clean = "0" + clean[3:]
	}
	if !thaiMobileRegex.MatchString(clean) {
		return "", ErrInvalidSMSDestination
	}
	return clean, nil
}

// MaskPhone hides middle digits so full subscriber numbers are never logged in plaintext.
func MaskPhone(phone string) string {
	clean := strings.TrimSpace(phone)
	if len(clean) <= 6 {
		return "***"
	}
	return clean[:3] + strings.Repeat("*", len(clean)-6) + clean[len(clean)-3:]
}

func (s *SMSClient) SendOTP(ctx context.Context, phoneNumber, otpCode, ref string) error {
	cleanPhone, err := NormalizeThaiMobile(phoneNumber)
	if err != nil {
		return err
	}

	canRelay := s.relayEnabled && s.relayURL != "" && len(s.relaySecret) >= 32
	if (s.gatewayURL == "" || s.username == "") && !canRelay {
		log.Printf("📱 [Mock SMS Service] Phone: %s | Ref: %s", MaskPhone(cleanPhone), ref)
		return nil
	}

	var primaryErr error
	if s.gatewayURL != "" && s.username != "" {
		txID, err := s.sendViaDtacPrimary(ctx, cleanPhone, otpCode, ref)
		if err == nil {
			sender := s.senderName
			if sender == "" {
				sender = "FAHFON"
			}
			log.Printf("✅ [SMS Primary Sent] Phone: %s | Ref: %s | Sender: %s | TxID: %s",
				MaskPhone(cleanPhone), ref, sender, txID)
			return nil
		}
		primaryErr = err

		// Do not retry via fallback if dtac explicitly reported Invalid Msn (-109).
		if errors.Is(err, ErrInvalidSMSDestination) {
			return err
		}
		log.Printf("⚠️ [SMS Primary Failed] Phone: %s | Ref: %s | Err: %v", MaskPhone(cleanPhone), ref, err)
	}

	if canRelay {
		if err := s.sendViaRelayFallback(ctx, cleanPhone, otpCode, ref); err != nil {
			if primaryErr != nil {
				return fmt.Errorf("primary sms failed (%v) and relay fallback failed (%w)", primaryErr, err)
			}
			return err
		}
		return nil
	}

	if primaryErr != nil {
		return primaryErr
	}
	return errors.New("no sms delivery channel available")
}

func (s *SMSClient) sendViaDtacPrimary(ctx context.Context, cleanPhone, otpCode, ref string) (string, error) {
	rawMsg := fmt.Sprintf("รหัส OTP คือ %s (Ref: %s) สำหรับ Chonburi PAO Connext", otpCode, ref)
	sender := s.senderName
	if sender == "" {
		sender = "FAHFON"
	}

	formData := url.Values{}
	formData.Set("User", s.username)
	formData.Set("Password", s.password)
	formData.Set("Msg", encodeUCS2Hex(rawMsg))
	formData.Set("Encoding", "8")
	formData.Set("Msn", cleanPhone)
	formData.Set("MsgType", "H")
	formData.Set("Sender", sender)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.gatewayURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create dtac sms request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send dtac sms request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	bodyStr := string(bodyBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("dtac sms gateway returned status %d: %s", resp.StatusCode, strings.TrimSpace(bodyStr))
	}

	return parseDtacCorplinkResponse(bodyStr)
}

func (s *SMSClient) sendViaRelayFallback(ctx context.Context, cleanPhone, otpCode, ref string) error {
	payload, err := json.Marshal(struct {
		PhoneNumber string `json:"phone_number"`
		OTPCode     string `json:"otp_code"`
		Ref         string `json:"ref"`
	}{
		PhoneNumber: cleanPhone,
		OTPCode:     otpCode,
		Ref:         ref,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal sms relay payload: %w", err)
	}

	const relayPath = "/api/v1/sms/otp"
	endpoint := s.relayURL + relayPath

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return fmt.Errorf("failed to generate relay nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeRelayHMAC(s.relaySecret, http.MethodPost, relayPath, ts, nonce, payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create sms relay request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", sig)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call sms relay: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sms relay returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var parsed struct {
		Success bool   `json:"success"`
		Sender  string `json:"sender"`
		TxID    string `json:"tx_id"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil || !parsed.Success {
		return fmt.Errorf("sms relay unsuccessful response: %s", strings.TrimSpace(string(bodyBytes)))
	}

	log.Printf("✅ [SMS Fallback Relay Sent] Phone: %s | Ref: %s | RelaySender: %s | TxID: %s",
		MaskPhone(cleanPhone), ref, parsed.Sender, parsed.TxID)
	return nil
}

func computeRelayHMAC(secret, method, path, timestamp, nonce string, body []byte) string {
	bodySum := sha256.Sum256(body)
	canonical := strings.Join([]string{
		strings.ToUpper(method),
		path,
		timestamp,
		nonce,
		hex.EncodeToString(bodySum[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func encodeUCS2Hex(rawMsg string) string {
	units := utf16.Encode([]rune(rawMsg))
	var hexBuilder strings.Builder
	hexBuilder.Grow(len(units) * 4)
	for _, u := range units {
		hexBuilder.WriteString(fmt.Sprintf("%04X", u))
	}
	return hexBuilder.String()
}

// parseDtacCorplinkResponse parses dtac's multiline response (Status=0 / TxID=... / Message=...).
func parseDtacCorplinkResponse(body string) (string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", errors.New("empty response from dtac sms gateway")
	}

	var statusVal, txID, message string
	lines := strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "status":
			statusVal = val
		case "txid":
			txID = val
		case "message":
			message = val
		}
	}

	if statusVal == "" {
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "<status>") && strings.Contains(lower, "</status>") {
			start := strings.Index(lower, "<status>") + len("<status>")
			end := strings.Index(lower[start:], "</status>")
			if end >= 0 {
				statusVal = strings.TrimSpace(trimmed[start : start+end])
			}
		}
	}

	if statusVal == "" {
		return "", fmt.Errorf("missing Status in dtac response: %s", trimmed)
	}
	if statusVal != "0" {
		if statusVal == "-109" {
			return txID, fmt.Errorf("%w (status=%s txid=%s message=%s)", ErrInvalidSMSDestination, statusVal, txID, message)
		}
		return txID, fmt.Errorf("dtac rejected sms (status=%s txid=%s message=%s)", statusVal, txID, message)
	}

	return txID, nil
}
