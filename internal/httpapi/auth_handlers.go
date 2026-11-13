package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/daddymaou/dart-otp/internal/auth"
	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/daddymaou/dart-otp/internal/otp"
	"github.com/daddymaou/dart-otp/internal/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

const sessionCookie = "dartotp_session"

func (a *API) setSession(c *gin.Context, tenantID string) error {
	token, err := auth.NewJWT(a.cfg.JWTSecret, tenantID)
	if err != nil {
		return err
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.cfg.CookieSecure,
	})
	return nil
}

func (a *API) clearSession(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: a.cfg.CookieSecure, MaxAge: -1,
	})
}

func (a *API) authIPBucket(c *gin.Context) string {
	return "ip:" + c.ClientIP() + ":auth"
}

func (a *API) signup(c *gin.Context) {
	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Name == "" || req.Email == "" {
		fail(c, "invalid_request", "Name and email are required.", nil)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		fail(c, "invalid_request", "Password must be at least 8 characters.", nil)
		return
	}
	if ok, _ := ratelimit.Allow(c.Request.Context(), a.pool, a.authIPBucket(c), a.cfg.AuthRatePerMin, time.Minute); !ok {
		fail(c, "rate_limited", "Too many attempts from this IP. Try again later.", nil)
		return
	}
	tenant, err := db.CreateTenant(c.Request.Context(), a.pool, req.Name, req.Email, hash)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			fail(c, "conflict", "That email is already registered.", nil)
			return
		}
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	if _, err := db.CreateDefaultTemplate(c.Request.Context(), a.pool, tenant.ID, "Default", otp.BuiltInTemplate); err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	if err := a.setSession(c, tenant.ID); err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": tenant.ID, "name": tenant.Name, "email": tenant.Email, "plan": tenant.Plan})
}

func (a *API) login(c *gin.Context) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if ok, _ := ratelimit.Allow(c.Request.Context(), a.pool, a.authIPBucket(c), a.cfg.AuthRatePerMin, time.Minute); !ok {
		fail(c, "rate_limited", "Too many attempts from this IP. Try again later.", nil)
		return
	}
	const msg = "Invalid email or password."
	tenant, err := db.GetTenantByEmail(c.Request.Context(), a.pool, req.Email)
	if err != nil {
		fail(c, "unauthorized", msg, nil)
		return
	}
	if !auth.CheckPassword(tenant.PasswordHash, req.Password) {
		fail(c, "unauthorized", msg, nil)
		return
	}
	if err := a.setSession(c, tenant.ID); err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": tenant.ID, "name": tenant.Name, "email": tenant.Email, "plan": tenant.Plan})
}

func (a *API) logout(c *gin.Context) {
	a.clearSession(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *API) jwtMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie(sessionCookie)
		if err != nil {
			fail(c, "unauthorized", "Not signed in.", nil)
			return
		}
		tenantID, err := auth.ParseJWT(a.cfg.JWTSecret, cookie)
		if err != nil {
			fail(c, "unauthorized", "Session expired. Please sign in again.", nil)
			return
		}
		tenant, err := db.GetTenant(c.Request.Context(), a.pool, tenantID)
		if err != nil {
			fail(c, "unauthorized", "Not signed in.", nil)
			return
		}
		c.Set("tenant_id", tenant.ID)
		c.Next()
	}
}

func (a *API) me(c *gin.Context) {
	tenant, err := db.GetTenant(c.Request.Context(), a.pool, c.GetString("tenant_id"))
	if err != nil {
		fail(c, "not_found", "Not found.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": tenant.ID, "name": tenant.Name, "email": tenant.Email, "plan": tenant.Plan, "status": tenant.Status})
}

func (a *API) deleteMe(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}
	twp, err := db.GetTenantWithPasswordByID(c.Request.Context(), a.pool, c.GetString("tenant_id"))
	if err != nil {
		fail(c, "not_found", "Not found.", nil)
		return
	}
	if !auth.CheckPassword(twp.PasswordHash, req.Password) {
		fail(c, "unauthorized", "Invalid email or password.", nil)
		return
	}
	if err := db.DeleteTenant(c.Request.Context(), a.pool, twp.ID); err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	a.clearSession(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
