package httpapi

import (
	"errors"
	"net/http"

	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/daddymaou/dart-otp/internal/otp"
	"github.com/daddymaou/dart-otp/internal/phone"
	"github.com/daddymaou/dart-otp/internal/templates"
	"github.com/gin-gonic/gin"
)

func (a *API) listTemplates(c *gin.Context) {
	tpls, err := db.ListTemplates(c.Request.Context(), a.pool, c.GetString("tenant_id"))
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	out := make([]gin.H, 0, len(tpls))
	for _, t := range tpls {
		out = append(out, gin.H{"id": t.ID, "name": t.Name, "body": t.Body, "is_default": t.IsDefault})
	}
	c.JSON(http.StatusOK, gin.H{"templates": out})
}

func (a *API) createTemplate(c *gin.Context) {
	var req struct {
		Name    string `json:"name"`
		Body    string `json:"body"`
		Default bool   `json:"is_default"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Name == "" {
		fail(c, "invalid_request", "Name is required.", nil)
		return
	}
	if err := templates.Validate(req.Body); err != nil {
		fail(c, "invalid_request", err.Error(), nil)
		return
	}
	t, err := db.CreateTemplate(c.Request.Context(), a.pool, c.GetString("tenant_id"), req.Name, req.Body, req.Default)
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": t.ID, "name": t.Name, "body": t.Body, "is_default": t.IsDefault})
}

func (a *API) getTemplate(c *gin.Context) {
	t, err := db.GetTemplate(c.Request.Context(), a.pool, c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		fail(c, "not_found", "Template not found.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": t.ID, "name": t.Name, "body": t.Body, "is_default": t.IsDefault})
}

func (a *API) updateTemplate(c *gin.Context) {
	var req struct {
		Name    string `json:"name"`
		Body    string `json:"body"`
		Default bool   `json:"is_default"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := templates.Validate(req.Body); err != nil {
		fail(c, "invalid_request", err.Error(), nil)
		return
	}
	t, err := db.UpdateTemplate(c.Request.Context(), a.pool, c.GetString("tenant_id"), c.Param("id"), req.Name, req.Body, req.Default)
	if err != nil {
		if errors.Is(err, db.ErrTemplateNotFound) {
			fail(c, "not_found", "Template not found.", nil)
			return
		}
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": t.ID, "name": t.Name, "body": t.Body, "is_default": t.IsDefault})
}

func (a *API) deleteTemplate(c *gin.Context) {
	if err := db.DeleteTemplate(c.Request.Context(), a.pool, c.GetString("tenant_id"), c.Param("id")); err != nil {
		if errors.Is(err, db.ErrTemplateNotFound) {
			fail(c, "not_found", "Template not found.", nil)
			return
		}
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// testTemplate sends a rendered sample with code 123456 to a linked phone.
func (a *API) testTemplate(c *gin.Context) {
	var req struct {
		Phone string `json:"phone"`
	}
	if !bindJSON(c, &req) {
		return
	}
	p, err := phone.Normalize(req.Phone)
	if err != nil {
		fail(c, "invalid_phone", "Phone must be a valid number in international format.", nil)
		return
	}
	t, err := db.GetTemplate(c.Request.Context(), a.pool, c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		fail(c, "not_found", "Template not found.", nil)
		return
	}
	link, err := db.GetActiveTelegramLink(c.Request.Context(), a.pool, p)
	if err != nil {
		fail(c, "not_found", "This phone is not linked to the bot.", nil)
		return
	}
	rendered := otp.RenderCode(t.Body, "123456")
	if err := a.otpSvc.SendSample(link.ChatID, rendered); err != nil {
		fail(c, "delivery_failed", "Telegram delivery failed.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
