package httpapi

import (
	"net/http"

	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/gin-gonic/gin"
)

func (a *API) logs(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 50)
	logs, total, err := db.ListOTPs(c.Request.Context(), a.pool, c.GetString("tenant_id"),
		c.Query("status"), c.Query("phone"), page, pageSize)
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	out := make([]gin.H, 0, len(logs))
	for _, l := range logs {
		out = append(out, gin.H{
			"id": l.ID, "phone": l.Phone, "status": l.Status,
			"delivery_status": l.DeliveryStatus, "attempts": l.Attempts,
			"is_test": l.IsTest, "expires_at": l.ExpiresAt,
			"verified_at": l.VerifiedAt, "created_at": l.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "page": page, "page_size": pageSize, "logs": out})
}

func (a *API) stats(c *gin.Context) {
	sent, verified, failed, today, rate, daily, err := db.GetStats(c.Request.Context(), a.pool, c.GetString("tenant_id"))
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	if daily == nil {
		daily = []db.DailyStat{}
	}
	c.JSON(http.StatusOK, gin.H{
		"sent": sent, "verified": verified, "failed": failed, "today": today,
		"success_rate": rate, "daily": daily,
	})
}

func queryInt(c *gin.Context, key string, def int) int {
	raw := c.Query(key)
	if raw == "" {
		return def
	}
	n := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	if n == 0 {
		return def
	}
	return n
}
