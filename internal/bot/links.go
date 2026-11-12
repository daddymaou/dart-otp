package bot

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

type pendingLink struct {
	tokenHash string
	expiresAt time.Time
}

type PendingLinks struct {
	items map[int64]pendingLink
}

func NewPendingLinks() *PendingLinks {
	return &PendingLinks{items: map[int64]pendingLink{}}
}

func (p *PendingLinks) Set(chatID int64, tokenHash string, ttl time.Duration) {
	p.items[chatID] = pendingLink{tokenHash: tokenHash, expiresAt: time.Now().Add(ttl)}
}

func (p *PendingLinks) Get(chatID int64) (string, bool) {
	pl, ok := p.items[chatID]
	if !ok || time.Now().After(pl.expiresAt) {
		delete(p.items, chatID)
		return "", false
	}
	return pl.tokenHash, true
}

func (p *PendingLinks) Delete(chatID int64) {
	delete(p.items, chatID)
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func NewLinkTokenRaw() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
