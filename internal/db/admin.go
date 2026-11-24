package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAdminStateConflict = errors.New("tenant state conflict")
	ErrProtectedAdmin     = errors.New("configured admin tenant cannot be suspended")
	ErrAdminPlanConflict  = errors.New("tenant already has requested plan")
)

type AdminTenant struct {
	ID           string
	Name         string
	Email        string
	Plan         string
	Status       string
	CreatedAt    time.Time
	APIKeyCount  int64
	OTPCount     int64
	WebhookCount int64
}

type AdminAuditEntry struct {
	ID             int64
	ActorTenantID  string
	ActorEmail     string
	Action         string
	TargetTenantID string
	Reason         string
	PreviousPlan   string
	NewPlan        string
	CreatedAt      time.Time
}

func AdminTenantCounts(ctx context.Context, pool *pgxpool.Pool) (total, active, suspended int64, err error) {
	err = pool.QueryRow(ctx,
		`SELECT count(*),
		        count(*) FILTER (WHERE status = 'active'),
		        count(*) FILTER (WHERE status = 'suspended')
		 FROM tenants`).Scan(&total, &active, &suspended)
	return
}

const adminTenantColumns = `
	t.id, t.name, t.email, t.plan, t.status, t.created_at,
	(SELECT count(*) FROM api_keys k WHERE k.tenant_id = t.id),
	(SELECT count(*) FROM otps o WHERE o.tenant_id = t.id),
	(SELECT count(*) FROM webhooks w WHERE w.tenant_id = t.id)`

func scanAdminTenant(row pgx.Row) (AdminTenant, error) {
	var tenant AdminTenant
	err := row.Scan(
		&tenant.ID, &tenant.Name, &tenant.Email, &tenant.Plan, &tenant.Status, &tenant.CreatedAt,
		&tenant.APIKeyCount, &tenant.OTPCount, &tenant.WebhookCount,
	)
	return tenant, err
}

func ListAdminTenants(ctx context.Context, pool *pgxpool.Pool, search string, limit int64, offset int64) ([]AdminTenant, int64, error) {
	var total int64
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM tenants
		 WHERE $1 = '' OR strpos(lower(name), lower($1)) > 0
		    OR strpos(lower(email), lower($1)) > 0
		    OR strpos(lower(id::text), lower($1)) > 0`, search,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := pool.Query(ctx,
		`SELECT `+adminTenantColumns+`
		 FROM tenants t
		 WHERE $1 = '' OR strpos(lower(t.name), lower($1)) > 0
		    OR strpos(lower(t.email), lower($1)) > 0
		    OR strpos(lower(t.id::text), lower($1)) > 0
		 ORDER BY t.created_at DESC, t.id DESC
		 LIMIT $2 OFFSET $3`, search, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tenants := make([]AdminTenant, 0)
	for rows.Next() {
		tenant, err := scanAdminTenant(rows)
		if err != nil {
			return nil, 0, err
		}
		tenants = append(tenants, tenant)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return tenants, total, nil
}

func GetAdminTenant(ctx context.Context, pool *pgxpool.Pool, id string) (AdminTenant, error) {
	tenant, err := scanAdminTenant(pool.QueryRow(ctx,
		`SELECT `+adminTenantColumns+` FROM tenants t WHERE t.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminTenant{}, ErrTenantNotFound
	}
	return tenant, err
}

func ChangeAdminTenantStatus(
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID, actorTenantID, actorEmail, action, reason string,
	adminEmails []string,
) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var email, status string
	if err := tx.QueryRow(ctx,
		`SELECT email, status FROM tenants WHERE id = $1 FOR UPDATE`, tenantID,
	).Scan(&email, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrTenantNotFound
		}
		return 0, err
	}

	var nextStatus, auditAction string
	switch action {
	case "suspend":
		if status != "active" {
			return 0, ErrAdminStateConflict
		}
		for _, adminEmail := range adminEmails {
			if strings.EqualFold(strings.TrimSpace(email), strings.TrimSpace(adminEmail)) {
				return 0, ErrProtectedAdmin
			}
		}
		nextStatus, auditAction = "suspended", "tenant_suspended"
	case "reactivate":
		if status != "suspended" {
			return 0, ErrAdminStateConflict
		}
		nextStatus, auditAction = "active", "tenant_reactivated"
	default:
		return 0, ErrAdminStateConflict
	}

	if _, err := tx.Exec(ctx, `UPDATE tenants SET status = $1 WHERE id = $2`, nextStatus, tenantID); err != nil {
		return 0, err
	}
	var auditID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO admin_audit (actor_tenant_id, actor_email, action, target_tenant_id, reason)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		actorTenantID, actorEmail, auditAction, tenantID, reason,
	).Scan(&auditID); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return auditID, nil
}

func ChangeAdminTenantPlan(
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID, actorTenantID, actorEmail, newPlan, reason string,
) (int64, string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback(ctx)

	var previousPlan string
	if err := tx.QueryRow(ctx,
		`SELECT plan FROM tenants WHERE id = $1 FOR UPDATE`, tenantID,
	).Scan(&previousPlan); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", ErrTenantNotFound
		}
		return 0, "", err
	}
	if previousPlan == newPlan {
		return 0, previousPlan, ErrAdminPlanConflict
	}

	if _, err := tx.Exec(ctx, `UPDATE tenants SET plan = $1 WHERE id = $2`, newPlan, tenantID); err != nil {
		return 0, "", err
	}
	var auditID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO admin_audit
		 (actor_tenant_id, actor_email, action, target_tenant_id, reason, previous_plan, new_plan)
		 VALUES ($1, $2, 'tenant_plan_changed', $3, $4, $5, $6) RETURNING id`,
		actorTenantID, actorEmail, tenantID, reason, previousPlan, newPlan,
	).Scan(&auditID); err != nil {
		return 0, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, "", err
	}
	return auditID, previousPlan, nil
}

func ListAdminAudit(ctx context.Context, pool *pgxpool.Pool, limit int64, offset int64) ([]AdminAuditEntry, int64, error) {
	var total int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM admin_audit`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := pool.Query(ctx,
		`SELECT id, actor_tenant_id, actor_email, action, target_tenant_id, reason,
		        COALESCE(previous_plan, ''), COALESCE(new_plan, ''), created_at
		 FROM admin_audit
		 ORDER BY created_at DESC, id DESC
		 LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries := make([]AdminAuditEntry, 0)
	for rows.Next() {
		var entry AdminAuditEntry
		if err := rows.Scan(
			&entry.ID, &entry.ActorTenantID, &entry.ActorEmail, &entry.Action,
			&entry.TargetTenantID, &entry.Reason, &entry.PreviousPlan, &entry.NewPlan, &entry.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}
