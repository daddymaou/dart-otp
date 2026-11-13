package webhooks

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSignAndDeliver(t *testing.T) {
	var gotSig, gotTS string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTS = r.Header.Get("X-DartOTP-Timestamp")
		gotSig = r.Header.Get("X-DartOTP-Signature")
		gotBody = make([]byte, r.ContentLength)
		_, _ = r.Body.Read(gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := []byte(`{"event":"otp.verified"}`)
	c := NewClient()
	status, err := c.Deliver(srv.URL, "secret", payload)
	if err != nil || status != http.StatusOK {
		t.Fatalf("deliver: %d %v", status, err)
	}
	ts, sig := Sign("secret", string(gotBody), time.Now())
	if sig != gotSig {
		t.Errorf("signature mismatch: %s vs %s", sig, gotSig)
	}
	if ts != gotTS {
		t.Errorf("timestamp mismatch")
	}
}

func TestValidateURL(t *testing.T) {
	if err := ValidateURL("http://127.0.0.1/hook", false); err == nil {
		t.Error("http loopback should be rejected outside development")
	}
	if err := ValidateURL("https://example.com/hook", false); err != nil {
		t.Errorf("https public should pass, got %v", err)
	}
	if err := ValidateURL("ftp://example.com/hook", false); err == nil {
		t.Error("non-http scheme should be rejected")
	}
	if err := ValidateURL("http://127.0.0.1/hook", true); err != nil {
		t.Errorf("http loopback allowed in development, got %v", err)
	}
}
