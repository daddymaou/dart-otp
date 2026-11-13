package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/daddymaou/dart-otp/internal/otp"
	"github.com/daddymaou/dart-otp/internal/phone"
	"github.com/gin-gonic/gin"
)

func (a *API) otpSend(c *gin.Context) {
	var req struct {
		Phone      string `json:"phone"`
		TemplateID string `json:"template_id"`
	}
	if !bindJSON(c, &req) {
		return
	}
	p, err := phone.Normalize(req.Phone)
	if err != nil {
		fail(c, "invalid_phone", "Phone must be a valid number in international format.", nil)
		return
	}
	out, err := a.otpSvc.Send(c.Request.Context(), c.GetString("tenant_id"), p, req.TemplateID, false)
	otpRespond(c, out, err)
}

func (a *API) otpResend(c *gin.Context) {
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
	out, err := a.otpSvc.Send(c.Request.Context(), c.GetString("tenant_id"), p, "", false)
	otpRespond(c, out, err)
}

func (a *API) otpVerify(c *gin.Context) {
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if !bindJSON(c, &req) {
		return
	}
	p, err := phone.Normalize(req.Phone)
	if err != nil {
		fail(c, "invalid_phone", "Phone must be a valid number in international format.", nil)
		return
	}
	out, err := a.otpSvc.Verify(c.Request.Context(), c.GetString("tenant_id"), p, req.Code)
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": out.Status})
}

func otpRespond(c *gin.Context, out otp.SendOutcome, err error) {
	if err != nil {
		otpError(c, err)
		return
	}
	if out.Status == "sent" {
		c.JSON(http.StatusOK, gin.H{
			"otp_id": out.OtpID, "status": "sent", "expires_at": out.ExpiresAt.UTC().Format(time.RFC3339),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"otp_id": out.OtpID, "status": "pending_link", "deep_link": out.DeepLink,
		"link_expires_at": out.LinkExpiresAt.UTC().Format(time.RFC3339),
	})
}

func otpError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, otp.ErrBlocked):
		fail(c, "telegram_blocked", "The user blocked the bot.", nil)
	case errors.Is(err, otp.ErrTenantInactive):
		fail(c, "account_suspended", "This account is suspended.", nil)
	case errors.Is(err, otp.ErrRateLimited):
		fail(c, "rate_limited", "Too many requests. Try again later.", nil)
	case errors.Is(err, otp.ErrQuota):
		fail(c, "quota_exceeded", "Monthly plan quota used up.", nil)
	case errors.Is(err, otp.ErrTemplateNotFound):
		fail(c, "not_found", "Template not found.", nil)
	default:
		var ce *otp.CooldownErr
		if errors.As(err, &ce) {
			fail(c, "cooldown_active", "Please wait before requesting another code.", map[string]any{"retry_after": ce.RetryAfter})
			return
		}
		fail(c, "delivery_failed", "Telegram delivery failed.", nil)
	}
}
