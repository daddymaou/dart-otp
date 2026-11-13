package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateTenant(ctx context.Context, pool *pgxpool.Pool, name, email, passwordHash string) (Tenant, error) {
	row := pool.QueryRow(ctx,
		`INSERT INTO tenants (name, email, password_hash) VALUES ($1, $2, $3)
		 RETURNING id, name, email, plan, status`, name, email, passwordHash)
	var t Tenant
	if err := row.Scan(&t.ID, &t.Name, &t.Email, &t.Plan, &t.Status); err != nil {
		return t, err
	}
	return t, nil
}

var ErrTenantNotFound = errors.New("tenant not found")

func GetTenantByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (TenantWithPassword, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, name, email, plan, status, password_hash FROM tenants WHERE lower(email) = lower($1)`, email)
	var t TenantWithPassword
	err := row.Scan(&t.ID, &t.Name, &t.Email, &t.Plan, &t.Status, &t.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrTenantNotFound
	}
	return t, err
}

type TenantWithPassword struct {
	Tenant
	PasswordHash string
}

func DeleteTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrTenantNotFound
	}
	return nil
}
