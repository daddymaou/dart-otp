package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OTP struct {
	ID             string
	TenantID       string
	Phone          string
	CodeHash       string
	Salt           string
	Status         string
	DeliveryStatus string
	DeliveryError  *string
	Attempts       int
	MaxAttempts    int
	IsTest         bool
	ExpiresAt      time.Time
	CreatedAt      time.Time
}

func CreateOTP(ctx context.Context, pool *pgxpool.Pool, o *OTP) error {
	return pool.QueryRow(ctx,
		`INSERT INTO otps (tenant_id, phone, code_hash, salt, status, delivery_status, attempts, max_attempts, is_test, expires_at)
		 VALUES ($1, $2, $3, $4, 'pending', $5, 0, $6, $7, $8)
		 RETURNING id, created_at`,
		o.TenantID, o.Phone, o.CodeHash, o.Salt, o.DeliveryStatus, o.MaxAttempts, o.IsTest, o.ExpiresAt,
	).Scan(&o.ID, &o.CreatedAt)
}

var ErrOTPNotFound = errors.New("otp not found")

const otpCols = `id, tenant_id, phone, code_hash, salt, status, delivery_status, delivery_error, attempts, max_attempts, is_test, expires_at, created_at`

func scanOTP(row pgx.Row) (OTP, error) {
	var o OTP
	err := row.Scan(&o.ID, &o.TenantID, &o.Phone, &o.CodeHash, &o.Salt, &o.Status, &o.DeliveryStatus,
		&o.DeliveryError, &o.Attempts, &o.MaxAttempts, &o.IsTest, &o.ExpiresAt, &o.CreatedAt)
	if err != nil {
		return o, err
	}
	return o, nil
}

func GetLatestOTP(ctx context.Context, pool *pgxpool.Pool, tenantID, phone string) (OTP, error) {
	o, err := scanOTP(pool.QueryRow(ctx,
		`SELECT `+otpCols+` FROM otps WHERE tenant_id = $1 AND phone = $2 ORDER BY created_at DESC LIMIT 1`,
		tenantID, phone))
	if errors.Is(err, pgx.ErrNoRows) {
		return o, ErrOTPNotFound
	}
	return o, err
}

func ExpirePendingOTPsForPhone(ctx context.Context, pool *pgxpool.Pool, tenantID, phone string) error {
	_, err := pool.Exec(ctx,
		`UPDATE otps SET status = 'expired' WHERE tenant_id = $1 AND phone = $2 AND status = 'pending'`,
		tenantID, phone)
	return err
}

// IncrementAttempts atomically bumps attempts and returns the row for comparison.
func IncrementAttempts(ctx context.Context, pool *pgxpool.Pool, otpID string) (OTP, bool, error) {
	o, err := scanOTP(pool.QueryRow(ctx,
		`UPDATE otps SET attempts = attempts + 1
		 WHERE id = $1 AND status = 'pending' AND attempts < max_attempts
		 RETURNING `+otpCols, otpID))
	if errors.Is(err, pgx.ErrNoRows) {
		return o, false, nil
	}
	return o, true, err
}

func SetOTPStatus(ctx context.Context, pool *pgxpool.Pool, otpID, status string) error {
	_, err := pool.Exec(ctx,
		`UPDATE otps SET status = $2, verified_at = CASE WHEN $2 = 'verified' THEN now() ELSE verified_at END
		 WHERE id = $1`, otpID, status)
	return err
}

func SetOTPDelivery(ctx context.Context, pool *pgxpool.Pool, otpID, deliveryStatus string, deliveryError *string) error {
	_, err := pool.Exec(ctx,
		`UPDATE otps SET delivery_status = $2, delivery_error = $3 WHERE id = $1`,
		otpID, deliveryStatus, deliveryError)
	return err
}

func SetOTPDeliveryAndExpiry(ctx context.Context, pool *pgxpool.Pool, otpID, deliveryStatus string, expiresAt time.Time) error {
	_, err := pool.Exec(ctx,
		`UPDATE otps SET delivery_status = $2, expires_at = $3 WHERE id = $1`,
		otpID, deliveryStatus, expiresAt)
	return err
}

// CheckCooldown returns retry-after seconds if a recent OTP for the pair exists.
func CheckCooldown(ctx context.Context, pool *pgxpool.Pool, tenantID, phone string, cooldown time.Duration) (int, bool, error) {
	var createdAt time.Time
	err := pool.QueryRow(ctx,
		`SELECT created_at FROM otps WHERE tenant_id = $1 AND phone = $2 ORDER BY created_at DESC LIMIT 1`,
		tenantID, phone).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	elapsed := time.Since(createdAt)
	if elapsed < cooldown {
		return int((cooldown - elapsed).Seconds()) + 1, true, nil
	}
	return 0, false, nil
}

func CountMonthlyOTPs(ctx context.Context, pool *pgxpool.Pool, tenantID string) (int, error) {
	var n int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM otps
		 WHERE tenant_id = $1 AND NOT is_test
		   AND created_at >= date_trunc('month', now() AT TIME ZONE 'UTC')`,
		tenantID).Scan(&n)
	return n, err
}

// TryIncrementAttemptsForLatest does the verify attempt update on the latest pending OTP for the pair.
func TryIncrementAttemptsForLatest(ctx context.Context, pool *pgxpool.Pool, tenantID, phone string) (OTP, bool, error) {
	o, err := scanOTP(pool.QueryRow(ctx,
		`UPDATE otps SET attempts = attempts + 1
		 WHERE id = (SELECT id FROM otps
		             WHERE tenant_id = $1 AND phone = $2 AND status = 'pending'
		             ORDER BY created_at DESC LIMIT 1)
		   AND status = 'pending' AND attempts < max_attempts
		 RETURNING `+otpCols, tenantID, phone))
	if errors.Is(err, pgx.ErrNoRows) {
		return o, false, nil
	}
	return o, true, err
}

func GetAwaitingLinkOTP(ctx context.Context, pool *pgxpool.Pool, tenantID, phone string) (OTP, error) {
	o, err := scanOTP(pool.QueryRow(ctx,
		`SELECT `+otpCols+` FROM otps
		 WHERE tenant_id = $1 AND phone = $2 AND delivery_status = 'awaiting_link' AND status = 'pending'
		   AND expires_at > now()
		 ORDER BY created_at DESC LIMIT 1`, tenantID, phone))
	if errors.Is(err, pgx.ErrNoRows) {
		return o, ErrOTPNotFound
	}
	return o, err
}
