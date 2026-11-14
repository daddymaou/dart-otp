package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/daddymaou/dart-otp/internal/config"
	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/daddymaou/dart-otp/internal/webhooks"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	cfg    *config.Config
	pool   *pgxpool.Pool
	client *webhooks.Client
	stop   chan struct{}
}

func Start(cfg *config.Config, pool *pgxpool.Pool) *Worker {
	w := &Worker{cfg: cfg, pool: pool, client: webhooks.NewClient(), stop: make(chan struct{})}
	go w.loop(10*time.Second, w.deliverDue)
	go w.loop(60*time.Second, w.expireOTPs)
	go w.loop(time.Hour, w.cleanup)
	return w
}

func (w *Worker) Stop() { close(w.stop) }

func (w *Worker) loop(interval time.Duration, fn func(ctx context.Context)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			fn(ctx)
			cancel()
		}
	}
}

var backoff = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second}

func (w *Worker) deliverDue(ctx context.Context) {
	deliveries, err := db.ClaimDueDeliveries(ctx, w.pool, 20)
	if err != nil {
		slog.Error("claim deliveries", "error", err)
		return
	}
	for _, d := range deliveries {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		status, err := w.client.Deliver(cctx, d.URL, d.Secret, d.Payload)
		cancel()
		switch {
		case err == nil:
			_ = db.SetDeliveryResult(ctx, w.pool, d.ID, "success", status, "", nil)
		default:
			if d.Attempts >= len(backoff) {
				_ = db.SetDeliveryResult(ctx, w.pool, d.ID, "failed", status, err.Error(), nil)
				continue
			}
			next := time.Now().Add(backoff[d.Attempts])
			_ = db.SetDeliveryResult(ctx, w.pool, d.ID, "pending", status, err.Error(), &next)
		}
	}
}

func (w *Worker) expireOTPs(ctx context.Context) {
	tag, err := w.pool.Exec(ctx,
		`UPDATE otps SET status = 'expired'
		 WHERE status = 'pending' AND expires_at < now()`)
	if err != nil {
		slog.Error("expire otps", "error", err)
		return
	}
	_ = tag
}

func (w *Worker) cleanup(ctx context.Context) {
	_, _ = w.pool.Exec(ctx, `DELETE FROM rate_events WHERE created_at < now() - interval '1 day'`)
	_, _ = w.pool.Exec(ctx, `DELETE FROM otps WHERE created_at < now() - make_interval(days => $1)`, w.cfg.RetentionDays)
	_, _ = w.pool.Exec(ctx, `DELETE FROM link_tokens
		WHERE (used_at IS NOT NULL AND used_at < now() - interval '1 day')
		   OR (used_at IS NULL AND expires_at < now() - interval '1 day')`)
}
