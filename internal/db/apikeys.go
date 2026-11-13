package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type APIKeyRow struct {
	ID      string
	Name    string
	Prefix  string
	IsTest  bool
	Revoked bool
}

func CreateAPIKey(ctx context.Context, pool *pgxpool.Pool, tenantID, name, prefix, keyHash string, isTest bool) (APIKeyRow, error) {
	row := pool.QueryRow(ctx,
		`INSERT INTO api_keys (tenant_id, name, prefix, key_hash, is_test)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, name, prefix, is_test, revoked_at IS NOT NULL`,
		tenantID, name, prefix, keyHash, isTest)
	var k APIKeyRow
	err := row.Scan(&k.ID, &k.Name, &k.Prefix, &k.IsTest, &k.Revoked)
	return k, err
}

type KeyAuth struct {
	APIKeyRow
	TenantID   string
	TenantStatus string
}

func GetKeyByHash(ctx context.Context, pool *pgxpool.Pool, keyHash string) (KeyAuth, error) {
	row := pool.QueryRow(ctx,
		`SELECT k.id, k.name, k.prefix, k.is_test, k.revoked_at IS NOT NULL, k.tenant_id, t.status
		 FROM api_keys k JOIN tenants t ON t.id = k.tenant_id
		 WHERE k.key_hash = $1`, keyHash)
	var ka KeyAuth
	err := row.Scan(&ka.ID, &ka.Name, &ka.Prefix, &ka.IsTest, &ka.Revoked, &ka.TenantID, &ka.TenantStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return ka, ErrTenantNotFound
	}
	return ka, err
}

func TouchKeyLastUsed(ctx context.Context, pool *pgxpool.Pool, keyID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE api_keys SET last_used_at = now()
		 WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, keyID)
	return err
}

func RevokeAPIKey(ctx context.Context, pool *pgxpool.Pool, tenantID, keyID string) error {
	tag, err := pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = now()
		 WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL`, keyID, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrTenantNotFound
	}
	return nil
}

func ListAPIKeys(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]APIKeyRow, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, name, prefix, is_test, revoked_at IS NOT NULL FROM api_keys
		 WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKeyRow
	for rows.Next() {
		var k APIKeyRow
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.IsTest, &k.Revoked); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
