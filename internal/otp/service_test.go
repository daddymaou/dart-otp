package otp

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/daddymaou/dart-otp/internal/config"
	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeSender struct{ failBlocked bool }

func (f *fakeSender) Send(chatID int64, body string) error { return nil }

func newTestService(t *testing.T) (*Service, string) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		cfg = nil
	}
	if cfg == nil {
		t.Skip("config not loadable without env")
	}
	s := NewService(cfg, pool, &fakeSender{})
	var id string
	err = pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		"t", uniqueEmail(), "x").Scan(&id)
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	if _, err := db.CreateDefaultTemplate(context.Background(), pool, id, "Default", BuiltInTemplate); err != nil {
		t.Fatalf("template: %v", err)
	}
	return s, id
}

var emailSeq int

func uniqueEmail() string {
	emailSeq++
	return "t" + time.Now().Format("20060102150405") + string(rune('a'+emailSeq)) + "@test.local"
}

func TestVerifyStateMachineAndRace(t *testing.T) {
	s, tenantID := newTestService(t)
	ctx := context.Background()

	// pending_link path: no linked phone
	out, err := s.Send(ctx, tenantID, "+2348012345678", "", false)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out.Status != "pending_link" || out.DeepLink == "" {
		t.Fatalf("outcome: %+v", out)
	}

	// not sent yet: verify should be not_found
	if v, _ := s.Verify(ctx, tenantID, "+2348012345678", "123456"); v.Status != "not_found" {
		t.Fatalf("verify before delivery = %s", v.Status)
	}

	// simulate delivered: fetch latest otp, flip delivery to sent
	latest, err := db.GetLatestOTP(ctx, s.pool, tenantID, "+2348012345678")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if err := db.SetOTPDelivery(ctx, s.pool, latest.ID, "sent", nil); err != nil {
		t.Fatalf("set delivery: %v", err)
	}

	code := s.pendingCodes.Get2(latest.ID)
	if code == "" {
		t.Fatal("pending code missing")
	}

	// race: two verifies in parallel; only one may verify
	var wg sync.WaitGroup
	results := make([]string, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, _ := s.Verify(ctx, tenantID, "+2348012345678", code)
			results[i] = v.Status
		}(i)
	}
	wg.Wait()
	verified := 0
	for _, r := range results {
		if r == "verified" {
			verified++
		}
	}
	if verified != 1 {
		t.Fatalf("verified count = %d, results = %v", verified, results)
	}

	// replay: same code again -> not_found
	if v, _ := s.Verify(ctx, tenantID, "+2348012345678", code); v.Status != "not_found" {
		t.Fatalf("replay = %s", v.Status)
	}
}
