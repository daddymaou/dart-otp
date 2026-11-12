package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

var statusByCode = map[string]int{
	"invalid_request":   http.StatusBadRequest,
	"unauthorized":      http.StatusUnauthorized,
	"account_suspended": http.StatusForbidden,
	"not_found":         http.StatusNotFound,
	"conflict":          http.StatusConflict,
	"invalid_phone":     http.StatusUnprocessableEntity,
	"telegram_blocked":  http.StatusUnprocessableEntity,
	"cooldown_active":   http.StatusTooManyRequests,
	"rate_limited":      http.StatusTooManyRequests,
	"quota_exceeded":    http.StatusTooManyRequests,
	"delivery_failed":   http.StatusBadGateway,
	"internal":          http.StatusInternalServerError,
}

func fail(c *gin.Context, code, message string, extra map[string]any) {
	errBody := gin.H{"code": code, "message": message}
	for k, v := range extra {
		errBody[k] = v
	}
	c.AbortWithStatusJSON(statusByCode[code], gin.H{"error": errBody})
}
