package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Template struct {
	ID        string
	TenantID  string
	Name      string
	Body      string
	IsDefault bool
}

func GetDefaultTemplate(ctx context.Context, pool *pgxpool.Pool, tenantID string) (Template, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, body, is_default FROM templates
		 WHERE tenant_id = $1 AND is_default = true LIMIT 1`, tenantID)
	var t Template
	if err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Body, &t.IsDefault); err != nil {
		return t, err
	}
	return t, nil
}

func GetTemplate(ctx context.Context, pool *pgxpool.Pool, tenantID, templateID string) (Template, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, body, is_default FROM templates
		 WHERE id = $1 AND tenant_id = $2`, templateID, tenantID)
	var t Template
	err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Body, &t.IsDefault)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrTemplateNotFound
	}
	return t, err
}

var ErrTemplateNotFound = errors.New("template not found")

func CreateDefaultTemplate(ctx context.Context, pool *pgxpool.Pool, tenantID, name, body string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO templates (tenant_id, name, body, is_default) VALUES ($1, $2, $3, true) RETURNING id`,
		tenantID, name, body).Scan(&id)
	return id, err
}

func GetTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) (Tenant, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, name, email, plan, status FROM tenants WHERE id = $1`, tenantID)
	var t Tenant
	err := row.Scan(&t.ID, &t.Name, &t.Email, &t.Plan, &t.Status)
	return t, err
}
