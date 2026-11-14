package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/daddymaou/dart-otp/internal/bot"
	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/daddymaou/dart-otp/internal/phone"
	"github.com/gin-gonic/gin"
)

func (a *API) devLinkToken(c *gin.Context) {
	var req struct {
		TenantID string `json:"tenant_id"`
		Phone    string `json:"phone"`
	}
	if !bindJSON(c, &req) {
		return
	}
	p, err := phone.Normalize(req.Phone)
	if err != nil {
		fail(c, "invalid_phone", "Phone must be a valid number in international format.", nil)
		return
	}
	raw := bot.NewLinkTokenRaw()
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])
	expires := time.Now().Add(a.cfg.LinkTokenTTL)
	if err := db.CreateLinkToken(context.Background(), a.pool, hash, req.TenantID, p, expires); err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"deep_link":       "https://t.me/" + a.cfg.TelegramBotUsername + "?start=" + raw,
		"link_expires_at": expires.UTC().Format(time.RFC3339),
	})
}
