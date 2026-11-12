package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LinkToken struct {
	TokenHash string
	TenantID  string
	Phone     string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func CreateLinkToken(ctx context.Context, pool *pgxpool.Pool, tokenHash, tenantID, phone string, expiresAt time.Time) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO link_tokens (token_hash, tenant_id, phone, expires_at) VALUES ($1, $2, $3, $4)`,
		tokenHash, tenantID, phone, expiresAt)
	return err
}

var ErrLinkTokenNotFound = errors.New("link token not found")

func GetLinkToken(ctx context.Context, pool *pgxpool.Pool, tokenHash string) (LinkToken, error) {
	row := pool.QueryRow(ctx,
		`SELECT token_hash, tenant_id, phone, expires_at, used_at FROM link_tokens WHERE token_hash = $1`,
		tokenHash)
	var lt LinkToken
	if err := row.Scan(&lt.TokenHash, &lt.TenantID, &lt.Phone, &lt.ExpiresAt, &lt.UsedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return lt, ErrLinkTokenNotFound
		}
		return lt, err
	}
	return lt, nil
}

func MarkLinkTokenUsed(ctx context.Context, tx pgx.Tx, tokenHash string) error {
	_, err := tx.Exec(ctx, `UPDATE link_tokens SET used_at = now() WHERE token_hash = $1`, tokenHash)
	return err
}

type TelegramLink struct {
	Phone          string
	ChatID         int64
	TelegramUserID int64
	Status         string
}

func UpsertTelegramLink(ctx context.Context, tx pgx.Tx, phone string, chatID, telegramUserID int64) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO telegram_links (phone, chat_id, telegram_user_id, status)
		 VALUES ($1, $2, $3, 'active')
		 ON CONFLICT (phone) DO UPDATE SET chat_id = $2, telegram_user_id = $3, status = 'active', updated_at = now()`,
		phone, chatID, telegramUserID)
	return err
}

func GetActiveTelegramLink(ctx context.Context, pool *pgxpool.Pool, phone string) (TelegramLink, error) {
	row := pool.QueryRow(ctx,
		`SELECT phone, chat_id, telegram_user_id, status FROM telegram_links WHERE phone = $1 AND status = 'active'`,
		phone)
	var l TelegramLink
	err := row.Scan(&l.Phone, &l.ChatID, &l.TelegramUserID, &l.Status)
	return l, err
}

func SetLinksInactiveForChat(ctx context.Context, pool *pgxpool.Pool, chatID int64) error {
	_, err := pool.Exec(ctx,
		`UPDATE telegram_links SET status = 'inactive', updated_at = now() WHERE chat_id = $1`, chatID)
	return err
}
