package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/daddymaou/dart-otp/internal/apikeys"
	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/gin-gonic/gin"
)

// apiKeyMiddleware authenticates public API requests via X-API-Key.
func (a *API) apiKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if key == "" {
			fail(c, "unauthorized", "Missing or invalid API key.", nil)
			return
		}
		sum := sha256.Sum256([]byte(key))
		ka, err := db.GetKeyByHash(c.Request.Context(), a.pool, hex.EncodeToString(sum[:]))
		if err != nil {
			fail(c, "unauthorized", "Missing or invalid API key.", nil)
			return
		}
		if ka.Revoked {
			fail(c, "unauthorized", "Missing or invalid API key.", nil)
			return
		}
		if ka.TenantStatus != "active" {
			fail(c, "account_suspended", "This account is suspended.", nil)
			return
		}
		_ = db.TouchKeyLastUsed(c.Request.Context(), a.pool, ka.ID)
		c.Set("tenant_id", ka.TenantID)
		c.Set("is_test", ka.IsTest)
		c.Next()
	}
}

func (a *API) createAPIKey(c *gin.Context) {
	var req struct {
		Name   string `json:"name"`
		IsTest bool   `json:"is_test"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Name == "" {
		fail(c, "invalid_request", "Name is required.", nil)
		return
	}
	fullKey, prefix, keyHash, err := apikeys.Generate(req.IsTest)
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	k, err := db.CreateAPIKey(c.Request.Context(), a.pool, c.GetString("tenant_id"), req.Name, prefix, keyHash, req.IsTest)
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id": k.ID, "name": k.Name, "prefix": k.Prefix, "is_test": k.IsTest,
		"key": fullKey,
	})
}

func (a *API) listAPIKeys(c *gin.Context) {
	keys, err := db.ListAPIKeys(c.Request.Context(), a.pool, c.GetString("tenant_id"))
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	out := make([]gin.H, 0, len(keys))
	for _, k := range keys {
		out = append(out, gin.H{"id": k.ID, "name": k.Name, "prefix": k.Prefix, "is_test": k.IsTest, "revoked": k.Revoked})
	}
	c.JSON(http.StatusOK, gin.H{"api_keys": out})
}

func (a *API) revokeAPIKey(c *gin.Context) {
	err := db.RevokeAPIKey(c.Request.Context(), a.pool, c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		if errors.Is(err, db.ErrTenantNotFound) {
			fail(c, "not_found", "API key not found.", nil)
			return
		}
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
