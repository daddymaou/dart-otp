package ratelimit

import (
	"context"
	"time"

	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Allow records an event and reports whether the caller is over the limit.
func Allow(ctx context.Context, pool *pgxpool.Pool, bucket string, limit int, window time.Duration) (bool, error) {
	n, err := db.CountEvents(ctx, pool, bucket, window)
	if err != nil {
		return false, err
	}
	if n >= limit {
		return false, nil
	}
	if err := db.InsertEvent(ctx, pool, bucket); err != nil {
		return false, err
	}
	return true, nil
}
