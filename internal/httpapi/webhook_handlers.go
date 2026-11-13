package httpapi

import (
	"errors"
	"net/http"

	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/daddymaou/dart-otp/internal/webhooks"
	"github.com/gin-gonic/gin"
)

func (a *API) createWebhook(c *gin.Context) {
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.URL == "" || len(req.Events) == 0 {
		fail(c, "invalid_request", "URL and at least one event are required.", nil)
		return
	}
	for _, e := range req.Events {
		switch e {
		case "otp.sent", "otp.verified", "otp.failed", "otp.expired":
		default:
			fail(c, "invalid_request", "Unknown event: "+e, nil)
			return
		}
	}
	if err := webhooks.ValidateURL(req.URL, a.cfg.AppEnv == "development"); err != nil {
		fail(c, "invalid_request", err.Error(), nil)
		return
	}
	secret, err := webhooks.NewSecret()
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	w, err := db.CreateWebhook(c.Request.Context(), a.pool, c.GetString("tenant_id"), req.URL, secret, req.Events)
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id": w.ID, "url": w.URL, "events": w.Events, "active": w.Active, "secret": secret,
	})
}

func (a *API) listWebhooks(c *gin.Context) {
	ws, err := db.ListWebhooks(c.Request.Context(), a.pool, c.GetString("tenant_id"))
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	out := make([]gin.H, 0, len(ws))
	for _, w := range ws {
		out = append(out, gin.H{"id": w.ID, "url": w.URL, "events": w.Events, "active": w.Active})
	}
	c.JSON(http.StatusOK, gin.H{"webhooks": out})
}

func (a *API) updateWebhook(c *gin.Context) {
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
		Active bool     `json:"active"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := webhooks.ValidateURL(req.URL, a.cfg.AppEnv == "development"); err != nil {
		fail(c, "invalid_request", err.Error(), nil)
		return
	}
	if err := db.UpdateWebhook(c.Request.Context(), a.pool, c.GetString("tenant_id"), c.Param("id"), req.URL, req.Events, req.Active); err != nil {
		if errors.Is(err, db.ErrWebhookNotFound) {
			fail(c, "not_found", "Webhook not found.", nil)
			return
		}
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *API) deleteWebhook(c *gin.Context) {
	if err := db.DeleteWebhook(c.Request.Context(), a.pool, c.GetString("tenant_id"), c.Param("id")); err != nil {
		if errors.Is(err, db.ErrWebhookNotFound) {
			fail(c, "not_found", "Webhook not found.", nil)
			return
		}
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *API) webhookDeliveries(c *gin.Context) {
	list, err := db.ListDeliveries(c.Request.Context(), a.pool, c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	if list == nil {
		list = []map[string]any{}
	}
	c.JSON(http.StatusOK, gin.H{"deliveries": list})
}
