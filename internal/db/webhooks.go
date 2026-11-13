package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Webhook struct {
	ID       string
	TenantID string
	URL      string
	Secret   string
	Events   []string
	Active   bool
}

func CreateWebhook(ctx context.Context, pool *pgxpool.Pool, tenantID, url, secret string, events []string) (Webhook, error) {
	row := pool.QueryRow(ctx,
		`INSERT INTO webhooks (tenant_id, url, secret, events) VALUES ($1, $2, $3, $4)
		 RETURNING id, tenant_id, url, secret, events, active`,
		tenantID, url, secret, events)
	var w Webhook
	err := row.Scan(&w.ID, &w.TenantID, &w.URL, &w.Secret, &w.Events, &w.Active)
	return w, err
}

func ListWebhooks(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]Webhook, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, tenant_id, url, secret, events, active FROM webhooks
		 WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.TenantID, &w.URL, &w.Secret, &w.Events, &w.Active); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func GetWebhook(ctx context.Context, pool *pgxpool.Pool, tenantID, id string) (Webhook, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, tenant_id, url, secret, events, active FROM webhooks WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	var w Webhook
	err := row.Scan(&w.ID, &w.TenantID, &w.URL, &w.Secret, &w.Events, &w.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrWebhookNotFound
	}
	return w, err
}

var ErrWebhookNotFound = errors.New("webhook not found")

func UpdateWebhook(ctx context.Context, pool *pgxpool.Pool, tenantID, id, url string, events []string, active bool) error {
	tag, err := pool.Exec(ctx,
		`UPDATE webhooks SET url = $1, events = $2, active = $3 WHERE id = $4 AND tenant_id = $5`,
		url, events, active, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrWebhookNotFound
	}
	return nil
}

func DeleteWebhook(ctx context.Context, pool *pgxpool.Pool, tenantID, id string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM webhooks WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrWebhookNotFound
	}
	return nil
}

func InsertDelivery(ctx context.Context, pool *pgxpool.Pool, webhookID, event string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO webhook_deliveries (webhook_id, event, payload) VALUES ($1, $2, $3)`,
		webhookID, event, body)
	return err
}

type Delivery struct {
	ID        string
	WebhookID string
	URL       string
	Secret    string
	Event     string
	Payload   []byte
	Attempts  int
	NextRetry time.Time
}

func ClaimDueDeliveries(ctx context.Context, pool *pgxpool.Pool, limit int) ([]Delivery, error) {
	rows, err := pool.Query(ctx,
		`UPDATE webhook_deliveries d SET attempts = attempts + 1
		 FROM webhooks w
		 WHERE w.id = d.webhook_id AND d.status = 'pending' AND d.attempts < 5 AND d.next_retry_at <= now()
		 RETURNING d.id, d.webhook_id, w.url, w.secret, d.event, d.payload, d.attempts, d.next_retry_at
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Delivery
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.URL, &d.Secret, &d.Event, &d.Payload, &d.Attempts, &d.NextRetry); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func SetDeliveryResult(ctx context.Context, pool *pgxpool.Pool, id string, status string, httpStatus int, lastErr string, nextRetry *time.Time) error {
	_, err := pool.Exec(ctx,
		`UPDATE webhook_deliveries
		 SET status = $2, last_status = $3, last_error = $4,
		     next_retry_at = COALESCE($5, next_retry_at)
		 WHERE id = $1`, id, status, httpStatus, lastErr, nextRetry)
	return err
}

func ListDeliveries(ctx context.Context, pool *pgxpool.Pool, tenantID, webhookID string) ([]map[string]any, error) {
	rows, err := pool.Query(ctx,
		`SELECT d.id, d.event, d.status, d.attempts, d.last_status, d.last_error,
		        to_char(d.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM webhook_deliveries d JOIN webhooks w ON w.id = d.webhook_id
		 WHERE w.tenant_id = $1 AND d.webhook_id = $2
		 ORDER BY d.created_at DESC LIMIT 100`, tenantID, webhookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, event, status, createdAt string
		var attempts, lastStatus int
		var lastErr *string
		if err := rows.Scan(&id, &event, &status, &attempts, &lastStatus, &lastErr, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "event": event, "status": status, "attempts": attempts,
			"last_status": lastStatus, "last_error": lastErr, "created_at": createdAt,
		})
	}
	return out, rows.Err()
}
