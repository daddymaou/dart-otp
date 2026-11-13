package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type OTPLog struct {
	ID             string
	Phone          string
	Status         string
	DeliveryStatus string
	Attempts       int
	IsTest         bool
	ExpiresAt      *string
	VerifiedAt     *string
	CreatedAt      string
}

func ListOTPs(ctx context.Context, pool *pgxpool.Pool, tenantID string, status, phone string, page, pageSize int) ([]OTPLog, int, error) {
	where := " WHERE tenant_id = $1"
	args := []any{tenantID}
	if status != "" {
		args = append(args, status)
		where += " AND status = $" + itoa(len(args))
	}
	if phone != "" {
		args = append(args, phone)
		where += " AND phone = $" + itoa(len(args))
	}
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM otps`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := pool.Query(ctx,
		`SELECT id, phone, status, delivery_status, attempts, is_test,
		        to_char(expires_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        to_char(verified_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM otps`+where+` ORDER BY created_at DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []OTPLog
	for rows.Next() {
		var l OTPLog
		if err := rows.Scan(&l.ID, &l.Phone, &l.Status, &l.DeliveryStatus, &l.Attempts, &l.IsTest, &l.ExpiresAt, &l.VerifiedAt, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

type DailyStat struct {
	Date     string `json:"date"`
	Sent     int    `json:"sent"`
	Verified int    `json:"verified"`
}

func GetStats(ctx context.Context, pool *pgxpool.Pool, tenantID string) (sent, verified, failed, today int, rate float64, daily []DailyStat, err error) {
	err = pool.QueryRow(ctx,
		`SELECT
		   count(*) FILTER (WHERE TRUE),
		   count(*) FILTER (WHERE status = 'verified'),
		   count(*) FILTER (WHERE status = 'failed'),
		   count(*) FILTER (WHERE created_at >= date_trunc('day', now()))
		 FROM otps WHERE tenant_id = $1`, tenantID).Scan(&sent, &verified, &failed, &today)
	if err != nil {
		return
	}
	if sent > 0 {
		rate = float64(verified) / float64(sent) * 100
	}
	rows, qerr := pool.Query(ctx,
		`SELECT d::date::text,
		        count(o.id),
		        count(o.id) FILTER (WHERE o.status = 'verified')
		 FROM generate_series(date_trunc('day', now()) - interval '6 days', date_trunc('day', now()), interval '1 day') d
		 LEFT JOIN otps o ON o.tenant_id = $1 AND o.created_at >= d AND o.created_at < d + interval '1 day'
		 GROUP BY d ORDER BY d`, tenantID)
	if qerr != nil {
		err = qerr
		return
	}
	defer rows.Close()
	for rows.Next() {
		var ds DailyStat
		if err = rows.Scan(&ds.Date, &ds.Sent, &ds.Verified); err != nil {
			return
		}
		daily = append(daily, ds)
	}
	err = rows.Err()
	return
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
