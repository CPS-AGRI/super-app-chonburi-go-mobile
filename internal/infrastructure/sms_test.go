package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSMSClient_PrimarySuccessStopsBeforeRelay(t *testing.T) {
	var calledURLs []string
	var sentSender string
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calledURLs = append(calledURLs, r.URL.String())
			raw, _ := io.ReadAll(r.Body)
			vals, _ := url.ParseQuery(string(raw))
			sentSender = vals.Get("Sender")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("Status=0\nTxID=TX123\nMessage=Success\n")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	sms := NewSMSClient(
		"https://apismsplus.dtac.co.th/servlet/com.iess.socket.SmsCorplink",
		"apiuser",
		"apipass",
		"CPAOConnext",
		WithSMSRelay(true, "http://10.1.2.25:50060", "0123456789abcdef0123456789abcdef"),
		WithHTTPClient(client),
	)

	if err := sms.SendOTP(context.Background(), "0812345678", "123456", "ABCD"); err != nil {
		t.Fatalf("expected primary success, got %v", err)
	}
	if len(calledURLs) != 1 || sentSender != "CPAOConnext" {
		t.Fatalf("expected single primary call with CPAOConnext, got urls=%v sender=%s", calledURLs, sentSender)
	}
}

func TestSMSClient_PrimaryRejectedTriggersSignedRelayFallback(t *testing.T) {
	relaySecret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var callOrder []string
	var gotSig, gotNonce, gotTS string
	var gotBody []byte

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			callOrder = append(callOrder, r.URL.String())
			raw, _ := io.ReadAll(r.Body)

			if strings.Contains(r.URL.Host, "dtac.co.th") {
				// Simulate dtac returning HTTP 200 OK with IP/Sender rejection
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("Status=-101\nTxID=TXERR\nMessage=Invalid IP or Sender\n")),
					Header:     make(http.Header),
				}, nil
			}

			gotSig = r.Header.Get("X-Signature")
			gotNonce = r.Header.Get("X-Nonce")
			gotTS = r.Header.Get("X-Timestamp")
			gotBody = raw
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"success":true,"sender":"FAHFON","tx_id":"RELAY-1"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	sms := NewSMSClient(
		"https://apismsplus.dtac.co.th/servlet/com.iess.socket.SmsCorplink",
		"apiuser",
		"apipass",
		"CPAOConnext",
		WithSMSRelay(true, "http://10.1.2.25:50060", relaySecret),
		WithHTTPClient(client),
	)

	if err := sms.SendOTP(context.Background(), "081-234-5678", "654321", "WXYZ"); err != nil {
		t.Fatalf("expected relay fallback success, got %v", err)
	}
	if len(callOrder) != 2 || !strings.Contains(callOrder[1], "10.1.2.25:50060/api/v1/sms/otp") {
		t.Fatalf("expected primary then relay call, got %v", callOrder)
	}

	// Verify HMAC signature matches expected canonical digest
	sum := sha256.Sum256(gotBody)
	expectedSig := computeRelayHMAC(relaySecret, http.MethodPost, "/api/v1/sms/otp", gotTS, gotNonce, gotBody)
	if gotSig != expectedSig || hex.EncodeToString(sum[:]) == "" {
		t.Fatalf("HMAC mismatch: got=%s want=%s", gotSig, expectedSig)
	}

	var payload map[string]string
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}
	if payload["phone_number"] != "0812345678" || payload["otp_code"] != "654321" || payload["ref"] != "WXYZ" {
		t.Fatalf("unexpected relay payload: %+v", payload)
	}
}

func TestSMSClient_InvalidMsnSkipsRelay(t *testing.T) {
	calls := 0
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("Status=-109\nTxID=TX109\nMessage=Invalid Msn\n")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	sms := NewSMSClient(
		"https://apismsplus.dtac.co.th/servlet/com.iess.socket.SmsCorplink",
		"apiuser",
		"apipass",
		"CPAOConnext",
		WithSMSRelay(true, "http://10.1.2.25:50060", "0123456789abcdef0123456789abcdef"),
		WithHTTPClient(client),
	)

	err := sms.SendOTP(context.Background(), "0812345678", "123456", "ABCD")
	if !errors.Is(err, ErrInvalidSMSDestination) {
		t.Fatalf("expected ErrInvalidSMSDestination, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected relay NOT to be called on -109 Invalid Msn, got calls=%d", calls)
	}
}

func TestEncodeUCS2Hex(t *testing.T) {
	got := encodeUCS2Hex("รหัส 123")
	want := "0E230E2B0E310E2A0020003100320033"
	if got != want {
		t.Fatalf("encodeUCS2Hex = %s, want %s", got, want)
	}
}

