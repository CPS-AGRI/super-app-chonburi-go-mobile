package infrastructure

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SMSService interface {
	SendOTP(ctx context.Context, phoneNumber, otpCode, ref string) error
}

type SMSClient struct {
	gatewayURL string
	username   string
	password   string
	senderName string
	httpClient *http.Client
}

func NewSMSClient(gatewayURL, username, password, senderName string) *SMSClient {
	return &SMSClient{
		gatewayURL: gatewayURL,
		username:   username,
		password:   password,
		senderName: senderName,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *SMSClient) SendOTP(ctx context.Context, phoneNumber, otpCode, ref string) error {
	// หากไม่ได้ตั้งค่า gatewayURL หรือ username ให้ fallback เป็น Log ใน local dev
	if s.gatewayURL == "" || s.username == "" {
		log.Printf("📱 [Mock SMS Service] Phone: %s | OTP: %s | Ref: %s", phoneNumber, otpCode, ref)
		return nil
	}

	// แปลงเบอร์โทรศัพท์ให้อยู่ในรูปแบบสากล/ตัวเลข (เช่น 0812345678)
	cleanPhone := strings.ReplaceAll(phoneNumber, "-", "")
	cleanPhone = strings.TrimSpace(cleanPhone)

	msg := fmt.Sprintf("Your OTP is %s. Reference: %s. Mueang Smart", otpCode, ref)
	sender := s.senderName
	if sender == "" {
		sender = "FAHFON"
	}

	// โครงสร้าง Payload ของ dtac SMS Corporate Link (apismsplus.dtac.co.th)
	formData := url.Values{}
	formData.Set("User", s.username)
	formData.Set("Password", s.password)
	formData.Set("Msisdn", cleanPhone)
	formData.Set("Msg", msg)
	formData.Set("Sender", sender)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.gatewayURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create dtac sms request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send dtac sms request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("dtac sms gateway returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	log.Printf("✅ [dtac SMS Sent Successfully] Phone: %s | Ref: %s | Response: %s", cleanPhone, ref, string(bodyBytes))
	return nil
}
