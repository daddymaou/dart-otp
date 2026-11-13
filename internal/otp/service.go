package otp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/daddymaou/dart-otp/internal/bot"
	"github.com/daddymaou/dart-otp/internal/config"
	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Sender interface {
	Send(chatID int64, renderedBody string) error
}

var ErrBlocked = bot.ErrBlocked

type Service struct {
	cfg    *config.Config
	pool   *pgxpool.Pool
	sender Sender
	pendingCodes *pendingCodeStore
}

// pendingCodeStore holds plaintext codes for OTPs waiting on a Telegram link.
// Lost on restart; the tenant simply sends a new code.
type pendingCodeStore struct {
	mu    sync.Mutex
	items map[string]pendingCode
}

type pendingCode struct {
	code      string
	expiresAt time.Time
}

func newPendingCodeStore() *pendingCodeStore {
	return &pendingCodeStore{items: map[string]pendingCode{}}
}

func (p *pendingCodeStore) Set(otpID, code string, ttl time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items[otpID] = pendingCode{code: code, expiresAt: time.Now().Add(ttl)}
}

func (p *pendingCodeStore) Get(otpID string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pc, ok := p.items[otpID]
	if !ok || time.Now().After(pc.expiresAt) {
		delete(p.items, otpID)
		return "", false
	}
	return pc.code, true
}

func (p *pendingCodeStore) Delete(otpID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.items, otpID)
}

func NewService(cfg *config.Config, pool *pgxpool.Pool, sender Sender) *Service {
	return &Service{cfg: cfg, pool: pool, sender: sender, pendingCodes: newPendingCodeStore()}
}

type SendOutcome struct {
	Status        string // "sent" | "pending_link"
	OtpID         string
	ExpiresAt     time.Time
	DeepLink      string
	LinkExpiresAt time.Time
}

type VerifyOutcome struct {
	Status string // verified | invalid | expired | too_many_attempts | not_found
}

var ErrTenantInactive = errors.New("account_suspended")
var ErrRateLimited = errors.New("rate_limited")
var ErrCooldown = errors.New("cooldown_active")
var ErrQuota = errors.New("quota_exceeded")

type CooldownErr struct{ RetryAfter int }

func (e *CooldownErr) Error() string { return "cooldown_active" }

func (s *Service) Send(ctx context.Context, tenantID, phoneRaw, templateID string, isTest bool) (SendOutcome, error) {
	out := SendOutcome{}

	t, err := db.GetTenant(ctx, s.pool, tenantID)
	if err != nil {
		return out, err
	}
	if t.Status != "active" {
		return out, ErrTenantInactive
	}

	if !isTest {
		n, err := db.CountEvents(ctx, s.pool, fmt.Sprintf("tenant:%s:send", tenantID), time.Minute)
		if err != nil {
			return out, err
		}
		if n >= s.cfg.TenantRatePerMin {
			return out, ErrRateLimited
		}
	}

	retry, cooling, err := db.CheckCooldown(ctx, s.pool, tenantID, phoneRaw, s.cfg.OTPCooldown)
	if err != nil {
		return out, err
	}
	if cooling {
		return out, &CooldownErr{RetryAfter: retry}
	}

	if !isTest {
		quota, err := db.CountMonthlyOTPs(ctx, s.pool, tenantID)
		if err != nil {
			return out, err
		}
		plan, ok := s.cfg.Plans[t.Plan]
		if ok && quota >= plan.Monthly {
			return out, ErrQuota
		}
	}

	body, err := s.resolveTemplate(ctx, tenantID, templateID)
	if err != nil {
		return out, err
	}

	code := "123456"
	if !isTest {
		code, err = GenerateCode(s.cfg.OTPLength)
		if err != nil {
			return out, err
		}
	}
	salt, err := NewSalt()
	if err != nil {
		return out, err
	}

	if err := db.ExpirePendingOTPsForPhone(ctx, s.pool, tenantID, phoneRaw); err != nil {
		return out, err
	}

	o := db.OTP{
		TenantID: tenantID, Phone: phoneRaw,
		CodeHash: HashCode(s.cfg.OTPPepper, salt, code), Salt: salt,
		DeliveryStatus: "queued", MaxAttempts: s.cfg.OTPMaxAttempts, IsTest: isTest,
		ExpiresAt: time.Now().Add(s.cfg.OTPTTL),
	}
	if err := db.CreateOTP(ctx, s.pool, &o); err != nil {
		return out, err
	}
	out.OtpID = o.ID
	s.pendingCodes.Set(o.ID, code, s.cfg.LinkTokenTTL)

	link, linkErr := db.GetActiveTelegramLink(ctx, s.pool, phoneRaw)
	switch {
	case linkErr == nil:
		if err := s.sender.Send(link.ChatID, RenderCode(body, code)); err != nil {
			if errors.Is(err, ErrBlocked) {
				_ = s.setLinkInactive(ctx, link.Phone)
				_ = db.SetOTPDelivery(ctx, s.pool, o.ID, "failed", ptr("telegram_blocked"))
				return out, ErrBlocked
			}
			_ = db.SetOTPDelivery(ctx, s.pool, o.ID, "failed", ptr("delivery_failed"))
			return out, fmt.Errorf("delivery failed")
		}
		expires := time.Now().Add(s.cfg.OTPTTL)
		_ = db.SetOTPDeliveryAndExpiry(ctx, s.pool, o.ID, "sent", expires)
		out.Status = "sent"
		out.ExpiresAt = expires
		s.pendingCodes.Delete(o.ID)
		if !isTest {
			_ = db.InsertEvent(ctx, s.pool, fmt.Sprintf("tenant:%s:send", tenantID))
		}
		return out, nil
	default:
		raw, hash := newLinkToken()
		ltExpires := time.Now().Add(s.cfg.LinkTokenTTL)
		if err := db.CreateLinkToken(ctx, s.pool, hash, tenantID, phoneRaw, ltExpires); err != nil {
			return out, err
		}
		_ = db.SetOTPDeliveryAndExpiry(ctx, s.pool, o.ID, "awaiting_link", ltExpires)
		out.Status = "pending_link"
		out.DeepLink = "https://t.me/" + url.PathEscape(s.cfg.TelegramBotUsername) + "?start=" + raw
		out.LinkExpiresAt = ltExpires
		if !isTest {
			_ = db.InsertEvent(ctx, s.pool, fmt.Sprintf("tenant:%s:send", tenantID))
		}
		return out, nil
	}
}

func (s *Service) setLinkInactive(ctx context.Context, phone string) error {
	_, err := s.pool.Exec(ctx, `UPDATE telegram_links SET status = 'inactive', updated_at = now() WHERE phone = $1`, phone)
	return err
}

func (s *Service) resolveTemplate(ctx context.Context, tenantID, templateID string) (string, error) {
	if templateID != "" {
		tpl, err := db.GetTemplate(ctx, s.pool, tenantID, templateID)
		if err != nil {
			return "", err
		}
		return tpl.Body, nil
	}
	tpl, err := db.GetDefaultTemplate(ctx, s.pool, tenantID)
	if err != nil {
		return BuiltInTemplate, nil
	}
	return tpl.Body, nil
}

func (s *Service) Verify(ctx context.Context, tenantID, phoneRaw, code string) (VerifyOutcome, error) {
	if !isDigitsLen(code, s.cfg.OTPLength) {
		return VerifyOutcome{Status: "invalid"}, nil
	}
	latest, err := db.GetLatestOTP(ctx, s.pool, tenantID, phoneRaw)
	if err != nil {
		return VerifyOutcome{Status: "not_found"}, nil
	}
	if latest.Status != "pending" || latest.DeliveryStatus != "sent" {
		return VerifyOutcome{Status: "not_found"}, nil
	}
	if time.Now().After(latest.ExpiresAt) {
		_ = db.SetOTPStatus(ctx, s.pool, latest.ID, "expired")
		return VerifyOutcome{Status: "expired"}, nil
	}
	o, ok, err := db.TryIncrementAttemptsForLatest(ctx, s.pool, tenantID, phoneRaw)
	if err != nil {
		return VerifyOutcome{}, err
	}
	if !ok {
		_ = db.SetOTPStatus(ctx, s.pool, latest.ID, "failed")
		return VerifyOutcome{Status: "too_many_attempts"}, nil
	}
	if CompareCodes(HashCode(s.cfg.OTPPepper, o.Salt, code), o.CodeHash) {
		_ = db.SetOTPStatus(ctx, s.pool, o.ID, "verified")
		return VerifyOutcome{Status: "verified"}, nil
	}
	if o.Attempts >= o.MaxAttempts {
		_ = db.SetOTPStatus(ctx, s.pool, o.ID, "failed")
		return VerifyOutcome{Status: "too_many_attempts"}, nil
	}
	return VerifyOutcome{Status: "invalid"}, nil
}

// DeliverWaitingCode implements the bot's post-link delivery (6.7 step 7).
// The plaintext code is kept only in memory (pendingCodes) between send and link.
func (s *Service) DeliverWaitingCode(tenantID, phone string, chatID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	o, err := db.GetAwaitingLinkOTP(ctx, s.pool, tenantID, phone)
	if err != nil {
		return
	}
	code, ok := s.pendingCodes.Get(o.ID)
	if !ok {
		_ = db.SetOTPDelivery(ctx, s.pool, o.ID, "failed", ptr("delivery_failed"))
		return
	}
	body, err := s.resolveTemplate(ctx, tenantID, "")
	if err != nil {
		return
	}
	if err := s.sender.Send(chatID, RenderCode(body, code)); err != nil {
		_ = db.SetOTPDelivery(ctx, s.pool, o.ID, "failed", ptr("delivery_failed"))
		return
	}
	_ = db.SetOTPDeliveryAndExpiry(ctx, s.pool, o.ID, "sent", time.Now().Add(s.cfg.OTPTTL))
	s.pendingCodes.Delete(o.ID)
}

func newLinkToken() (raw, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:])
}

func ptr(s string) *string { return &s }

func isDigitsLen(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func RenderCode(body, code string) string {
	return bot.RenderBody(body, code)
}
