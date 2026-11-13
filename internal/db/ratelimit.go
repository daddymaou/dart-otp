package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Tenant struct {
	ID     string
	Name   string
	Email  string
	Plan   string
	Status string
}

func CountEvents(ctx context.Context, pool *pgxpool.Pool, bucket string, window time.Duration) (int, error) {
	var n int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM rate_events WHERE bucket = $1 AND created_at > now() - make_interval(secs => $2)`,
		bucket, window.Seconds()).Scan(&n)
	return n, err
}

func InsertEvent(ctx context.Context, pool *pgxpool.Pool, bucket string) error {
	_, err := pool.Exec(ctx, `INSERT INTO rate_events (bucket) VALUES ($1)`, bucket)
	return err
}
