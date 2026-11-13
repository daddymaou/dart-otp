package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ValidateURL enforces https (or http in development) and rejects loopback,
// private and link-local targets (SSRF).
func ValidateURL(rawURL string, development bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	if u.Scheme != "https" && !(development && u.Scheme == "http") {
		return fmt.Errorf("url must use https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("invalid url")
	}
	if development {
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("cannot resolve host")
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("url must not point at private addresses")
		}
	}
	return nil
}

func BuildPayload(deliveryID, event string, createdAt time.Time, otpID, phone string) map[string]any {
	return map[string]any{
		"id": deliveryID, "event": event,
		"created_at": createdAt.UTC().Format(time.RFC3339),
		"data":       map[string]any{"otp_id": otpID, "phone": phone},
	}
}

func Sign(secret, body string, timestamp time.Time) (string, string) {
	ts := strconv.FormatInt(timestamp.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + body))
	return ts, "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{
		Timeout: 10 * time.Second,
	}}
}

// Deliver posts the payload with signature headers. Returns HTTP status or error.
func (c *Client) Deliver(ctx context.Context, url, secret string, payload []byte) (int, error) {
	ts, sig := Sign(secret, string(payload), time.Now())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DartOTP-Timestamp", ts)
	req.Header.Set("X-DartOTP-Signature", sig)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, errors.New("non-2xx response")
}
