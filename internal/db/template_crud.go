package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ListTemplates(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]Template, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, tenant_id, name, body, is_default FROM templates
		 WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Body, &t.IsDefault); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func CreateTemplate(ctx context.Context, pool *pgxpool.Pool, tenantID, name, body string, makeDefault bool) (Template, error) {
	var t Template
	if !makeDefault {
		var anyDefault bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM templates WHERE tenant_id = $1 AND is_default)`, tenantID).Scan(&anyDefault); err != nil {
			return t, err
		}
		makeDefault = !anyDefault
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return t, err
	}
	defer tx.Rollback(ctx)
	if makeDefault {
		if _, err := tx.Exec(ctx, `UPDATE templates SET is_default = false WHERE tenant_id = $1`, tenantID); err != nil {
			return t, err
		}
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO templates (tenant_id, name, body, is_default) VALUES ($1, $2, $3, $4)
		 RETURNING id, tenant_id, name, body, is_default`,
		tenantID, name, body, makeDefault)
	if err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Body, &t.IsDefault); err != nil {
		return t, err
	}
	return t, tx.Commit(ctx)
}

func UpdateTemplate(ctx context.Context, pool *pgxpool.Pool, tenantID, templateID, name, body string, makeDefault bool) (Template, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Template{}, err
	}
	defer tx.Rollback(ctx)
	if makeDefault {
		if _, err := tx.Exec(ctx, `UPDATE templates SET is_default = false WHERE tenant_id = $1 AND is_default`, tenantID); err != nil {
			return Template{}, err
		}
	}
	row := tx.QueryRow(ctx,
		`UPDATE templates SET name = $1, body = $2,
		   is_default = CASE WHEN $3 THEN true ELSE is_default END,
		   updated_at = now()
		 WHERE id = $4 AND tenant_id = $5
		 RETURNING id, tenant_id, name, body, is_default`,
		name, body, makeDefault, templateID, tenantID)
	var t Template
	if err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Body, &t.IsDefault); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return t, ErrTemplateNotFound
		}
		return t, err
	}
	return t, tx.Commit(ctx)
}

func DeleteTemplate(ctx context.Context, pool *pgxpool.Pool, tenantID, templateID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `DELETE FROM templates WHERE id = $1 AND tenant_id = $2`, templateID, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrTemplateNotFound
	}
	// if the deleted one was default, promote the next remaining template
	if _, err := tx.Exec(ctx,
		`WITH remaining AS (
		   SELECT id FROM templates WHERE tenant_id = $1 ORDER BY created_at LIMIT 1
		 )
		 UPDATE templates SET is_default = true
		 WHERE tenant_id = $1
		   AND id IN (SELECT id FROM remaining)
		   AND NOT EXISTS (SELECT 1 FROM templates WHERE tenant_id = $1 AND is_default)`, tenantID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func CountTemplates(ctx context.Context, pool *pgxpool.Pool, tenantID string) (int, error) {
	var n int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM templates WHERE tenant_id = $1`, tenantID).Scan(&n)
	return n, err
}
