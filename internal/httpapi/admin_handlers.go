package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const (
	defaultAdminPageSize int64 = 20
	maxAdminPageSize     int64 = 100
	maxAdminPage         int64 = 1_000_000
	maxAdminSearchRunes        = 100
	maxAdminReasonRunes        = 1000
)

var canonicalUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (a *API) adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenant, err := db.GetTenant(c.Request.Context(), a.pool, c.GetString("tenant_id"))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				fail(c, "unauthorized", "Not signed in.", nil)
				return
			}
			fail(c, "internal", "Something went wrong.", nil)
			return
		}

		email := strings.ToLower(strings.TrimSpace(tenant.Email))
		allowed := false
		for _, adminEmail := range a.cfg.AdminEmails {
			if email == adminEmail {
				allowed = true
				break
			}
		}
		if !allowed {
			fail(c, "forbidden", "Not authorized.", nil)
			return
		}
		c.Set("admin_actor_email", email)
		c.Next()
	}
}

func (a *API) adminOverview(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	health := gin.H{"api": "ok", "database": "ok"}
	var tenantCounts any
	if err := a.pool.Ping(ctx); err != nil {
		health["database"] = "unavailable"
	} else {
		total, active, suspended, err := db.AdminTenantCounts(ctx, a.pool)
		if err != nil {
			fail(c, "internal", "Something went wrong.", nil)
			return
		}
		tenantCounts = gin.H{"total": total, "active": active, "suspended": suspended}
	}
	c.JSON(http.StatusOK, gin.H{"health": health, "tenants": tenantCounts})
}

func (a *API) adminListTenants(c *gin.Context) {
	page, pageSize, offset, ok := adminPagination(c)
	if !ok {
		return
	}
	search := strings.TrimSpace(c.Query("q"))
	if !utf8.ValidString(search) || strings.ContainsRune(search, '\x00') || utf8.RuneCountInString(search) > maxAdminSearchRunes {
		fail(c, "invalid_request", "Search query is too long.", nil)
		return
	}
	tenants, total, err := db.ListAdminTenants(c.Request.Context(), a.pool, search, pageSize, offset)
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	out := make([]gin.H, 0, len(tenants))
	for _, tenant := range tenants {
		out = append(out, adminTenantResponse(tenant))
	}
	c.JSON(http.StatusOK, gin.H{
		"tenants": out, "page": page, "page_size": pageSize, "total": total,
	})
}

func (a *API) adminGetTenant(c *gin.Context) {
	id := c.Param("id")
	if !canonicalUUID.MatchString(id) {
		fail(c, "invalid_request", "Tenant ID must be a UUID.", nil)
		return
	}
	tenant, err := db.GetAdminTenant(c.Request.Context(), a.pool, id)
	if errors.Is(err, db.ErrTenantNotFound) {
		fail(c, "not_found", "Tenant not found.", nil)
		return
	}
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tenant": adminTenantResponse(tenant)})
}

func (a *API) adminSuspendTenant(c *gin.Context) {
	a.adminChangeTenantStatus(c, "suspend", "suspended")
}

func (a *API) adminReactivateTenant(c *gin.Context) {
	a.adminChangeTenantStatus(c, "reactivate", "active")
}

func (a *API) adminChangeTenantPlan(c *gin.Context) {
	id := c.Param("id")
	if !canonicalUUID.MatchString(id) {
		fail(c, "invalid_request", "Tenant ID must be a UUID.", nil)
		return
	}
	var req struct {
		Plan   string `json:"plan"`
		Reason string `json:"reason"`
	}
	if !bindJSON(c, &req) {
		return
	}
	req.Plan = strings.TrimSpace(req.Plan)
	if _, ok := a.cfg.Plans[req.Plan]; !ok {
		fail(c, "invalid_request", "Plan must be one of the configured plans.", nil)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" || strings.ContainsRune(req.Reason, '\x00') || utf8.RuneCountInString(req.Reason) > maxAdminReasonRunes {
		fail(c, "invalid_request", "A non-empty reason of at most 1000 characters is required.", nil)
		return
	}

	auditID, previousPlan, err := db.ChangeAdminTenantPlan(
		c.Request.Context(), a.pool, id, c.GetString("tenant_id"),
		c.GetString("admin_actor_email"), req.Plan, req.Reason,
	)
	switch {
	case errors.Is(err, db.ErrTenantNotFound):
		fail(c, "not_found", "Tenant not found.", nil)
		return
	case errors.Is(err, db.ErrAdminPlanConflict):
		fail(c, "conflict", "Tenant already has that plan.", nil)
		return
	case err != nil:
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": id, "plan": req.Plan, "action": "plan_changed",
		"previous_plan": previousPlan, "audit_id": auditID,
	})
}

func (a *API) adminChangeTenantStatus(c *gin.Context, action, status string) {
	id := c.Param("id")
	if !canonicalUUID.MatchString(id) {
		fail(c, "invalid_request", "Tenant ID must be a UUID.", nil)
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !bindJSON(c, &req) {
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" || strings.ContainsRune(req.Reason, '\x00') || utf8.RuneCountInString(req.Reason) > maxAdminReasonRunes {
		fail(c, "invalid_request", "A non-empty reason of at most 1000 characters is required.", nil)
		return
	}

	auditID, err := db.ChangeAdminTenantStatus(
		c.Request.Context(), a.pool, id, c.GetString("tenant_id"),
		c.GetString("admin_actor_email"), action, req.Reason, a.cfg.AdminEmails,
	)
	switch {
	case errors.Is(err, db.ErrTenantNotFound):
		fail(c, "not_found", "Tenant not found.", nil)
		return
	case errors.Is(err, db.ErrAdminStateConflict), errors.Is(err, db.ErrProtectedAdmin):
		fail(c, "conflict", "Tenant status cannot be changed.", nil)
		return
	case err != nil:
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": id, "status": status, "action": action, "audit_id": auditID,
	})
}

func (a *API) adminListAudit(c *gin.Context) {
	page, pageSize, offset, ok := adminPagination(c)
	if !ok {
		return
	}
	entries, total, err := db.ListAdminAudit(c.Request.Context(), a.pool, pageSize, offset)
	if err != nil {
		fail(c, "internal", "Something went wrong.", nil)
		return
	}
	out := make([]gin.H, 0, len(entries))
	for _, entry := range entries {
		out = append(out, gin.H{
			"id":               entry.ID,
			"actor":            gin.H{"id": entry.ActorTenantID, "email": entry.ActorEmail},
			"action":           entry.Action,
			"target_tenant_id": entry.TargetTenantID,
			"reason":           entry.Reason,
			"previous_plan":    entry.PreviousPlan,
			"new_plan":         entry.NewPlan,
			"created_at":       entry.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"audit": out, "page": page, "page_size": pageSize, "total": total,
	})
}

func adminPagination(c *gin.Context) (page, pageSize, offset int64, ok bool) {
	page = 1
	pageSize = defaultAdminPageSize
	if raw := c.Query("page"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 || parsed > maxAdminPage {
			fail(c, "invalid_request", "Page must be between 1 and 1000000.", nil)
			return 0, 0, 0, false
		}
		page = parsed
	}
	if raw := c.Query("page_size"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 || parsed > maxAdminPageSize {
			fail(c, "invalid_request", "Page size must be between 1 and 100.", nil)
			return 0, 0, 0, false
		}
		pageSize = parsed
	}
	return page, pageSize, (page - 1) * pageSize, true
}

func adminTenantResponse(tenant db.AdminTenant) gin.H {
	return gin.H{
		"id": tenant.ID, "name": tenant.Name, "email": tenant.Email,
		"plan": tenant.Plan, "status": tenant.Status, "created_at": tenant.CreatedAt,
		"counts": gin.H{
			"api_keys": tenant.APIKeyCount, "otp_records": tenant.OTPCount, "webhooks": tenant.WebhookCount,
		},
	}
}
