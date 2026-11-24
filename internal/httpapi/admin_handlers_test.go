package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/daddymaou/dart-otp/internal/auth"
	"github.com/daddymaou/dart-otp/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type adminTestAccount struct {
	id    string
	email string
}

func TestAdminAPIAuthorizationValidationAndAudit(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping test database: %v", err)
	}
	t.Cleanup(pool.Close)

	fixtures := createAdminTestAccounts(t, ctx, pool)
	cfg := &config.Config{
		AppEnv:      "test",
		JWTSecret:   "test-only-admin-jwt-secret",
		AdminEmails: []string{fixtures.admin.email, fixtures.protected.email},
		Plans: map[string]config.Plan{
			"free": {Monthly: 100}, "starter": {Monthly: 1000}, "growth": {Monthly: 5000},
			"pro": {Monthly: 10000}, "business": {Monthly: 50000},
		},
	}
	router, _ := New(cfg, pool, nil)

	t.Run("missing session", func(t *testing.T) {
		response := serveAdminTestRequest(t, router, http.MethodGet, "/v1/admin/overview", "", nil, nil)
		assertAdminError(t, response, http.StatusUnauthorized, "unauthorized")
	})

	t.Run("tenant session cannot read or mutate admin data", func(t *testing.T) {
		requests := []struct {
			method string
			path   string
			body   any
		}{
			{http.MethodGet, "/v1/admin/overview", nil},
			{http.MethodGet, "/v1/admin/tenants", nil},
			{http.MethodGet, "/v1/admin/tenants/" + fixtures.target.id, nil},
			{http.MethodGet, "/v1/admin/audit", nil},
			{http.MethodPost, "/v1/admin/tenants/" + fixtures.target.id + "/suspend", map[string]string{"reason": "test"}},
			{http.MethodPost, "/v1/admin/tenants/" + fixtures.target.id + "/plan", map[string]string{"plan": "pro", "reason": "test"}},
		}
		for _, test := range requests {
			response := serveAdminTestRequest(t, router, test.method, test.path, fixtures.tenant.id, test.body, map[string]string{"X-Admin": "true"})
			assertAdminError(t, response, http.StatusForbidden, "forbidden")
		}
	})

	t.Run("admin list bounds search and limited output", func(t *testing.T) {
		for _, query := range []string{
			"?page=0",
			"?page=1000001",
			"?page_size=101",
			"?page_size=0",
			"?q=" + strings.Repeat("x", 101),
			"?q=%00",
		} {
			response := serveAdminTestRequest(t, router, http.MethodGet, "/v1/admin/tenants"+query, fixtures.admin.id, nil, nil)
			assertAdminError(t, response, http.StatusBadRequest, "invalid_request")
		}

		response := serveAdminTestRequest(t, router, http.MethodGet,
			"/v1/admin/tenants?page=1&page_size=100&q="+fixtures.target.email,
			fixtures.admin.id, nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("list tenants status = %d, body = %s", response.Code, response.Body.String())
		}
		var body struct {
			Tenants []map[string]any `json:"tenants"`
			Page    int64            `json:"page"`
			Size    int64            `json:"page_size"`
			Total   int64            `json:"total"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode tenant list: %v", err)
		}
		if body.Page != 1 || body.Size != 100 || body.Total != 1 || len(body.Tenants) != 1 {
			t.Fatalf("unexpected tenant list pagination/search: %+v", body)
		}
		for _, forbidden := range []string{"password_hash", "api_key", "key_hash", "code_hash", "phone", "secret"} {
			if _, found := body.Tenants[0][forbidden]; found {
				t.Fatalf("tenant response leaked field %q", forbidden)
			}
		}
	})

	t.Run("admin overview and detail", func(t *testing.T) {
		response := serveAdminTestRequest(t, router, http.MethodGet, "/v1/admin/overview", fixtures.admin.id, nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("overview status = %d, body = %s", response.Code, response.Body.String())
		}
		var overview struct {
			Health struct {
				API      string `json:"api"`
				Database string `json:"database"`
			} `json:"health"`
			Tenants struct {
				Total     int64 `json:"total"`
				Active    int64 `json:"active"`
				Suspended int64 `json:"suspended"`
			} `json:"tenants"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &overview); err != nil {
			t.Fatalf("decode overview: %v", err)
		}
		if overview.Health.API != "ok" || overview.Health.Database != "ok" || overview.Tenants.Total < 4 {
			t.Fatalf("unexpected overview: %+v", overview)
		}

		response = serveAdminTestRequest(t, router, http.MethodGet,
			"/v1/admin/tenants/"+fixtures.target.id, fixtures.admin.id, nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("tenant detail status = %d, body = %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "password_hash") || strings.Contains(response.Body.String(), "secret") {
			t.Fatalf("tenant detail leaked a sensitive field: %s", response.Body.String())
		}
	})

	t.Run("IDs reasons transitions and admin protection", func(t *testing.T) {
		for _, test := range []struct {
			path   string
			body   any
			status int
			code   string
		}{
			{"/v1/admin/tenants/not-a-uuid", nil, http.StatusBadRequest, "invalid_request"},
			{"/v1/admin/tenants/not-a-uuid/suspend", map[string]string{"reason": "test"}, http.StatusBadRequest, "invalid_request"},
			{"/v1/admin/tenants/" + fixtures.target.id + "/suspend", map[string]string{"reason": "   "}, http.StatusBadRequest, "invalid_request"},
			{"/v1/admin/tenants/" + fixtures.target.id + "/suspend", map[string]string{"reason": "invalid\x00reason"}, http.StatusBadRequest, "invalid_request"},
			{"/v1/admin/tenants/" + fixtures.target.id + "/suspend", map[string]string{"reason": strings.Repeat("x", 1001)}, http.StatusBadRequest, "invalid_request"},
			{"/v1/admin/tenants/" + fixtures.target.id + "/plan", map[string]string{"plan": "unknown", "reason": "test"}, http.StatusBadRequest, "invalid_request"},
			{"/v1/admin/tenants/" + fixtures.target.id + "/plan", map[string]string{"plan": "starter", "reason": "   "}, http.StatusBadRequest, "invalid_request"},
			{"/v1/admin/tenants/00000000-0000-0000-0000-000000000001", nil, http.StatusNotFound, "not_found"},
			{"/v1/admin/tenants/00000000-0000-0000-0000-000000000001/suspend", map[string]string{"reason": "test"}, http.StatusNotFound, "not_found"},
			{"/v1/admin/tenants/00000000-0000-0000-0000-000000000001/plan", map[string]string{"plan": "starter", "reason": "test"}, http.StatusNotFound, "not_found"},
			{"/v1/admin/tenants/" + fixtures.admin.id + "/suspend", map[string]string{"reason": "self lockout"}, http.StatusConflict, "conflict"},
			{"/v1/admin/tenants/" + fixtures.protected.id + "/suspend", map[string]string{"reason": "admin lockout"}, http.StatusConflict, "conflict"},
		} {
			response := serveAdminTestRequest(t, router, http.MethodPost, test.path, fixtures.admin.id, test.body, nil)
			assertAdminError(t, response, test.status, test.code)
		}

		response := serveAdminTestRequest(t, router, http.MethodPost,
			"/v1/admin/tenants/"+fixtures.target.id+"/suspend",
			fixtures.admin.id, map[string]string{"reason": "abuse investigation"}, nil)
		assertAdminStatus(t, response, http.StatusOK, "suspended")
		response = serveAdminTestRequest(t, router, http.MethodPost,
			"/v1/admin/tenants/"+fixtures.target.id+"/suspend",
			fixtures.admin.id, map[string]string{"reason": "duplicate"}, nil)
		assertAdminError(t, response, http.StatusConflict, "conflict")

		response = serveAdminTestRequest(t, router, http.MethodPost,
			"/v1/admin/tenants/"+fixtures.target.id+"/reactivate",
			fixtures.admin.id, map[string]string{"reason": "appeal accepted"}, nil)
		assertAdminStatus(t, response, http.StatusOK, "active")
		response = serveAdminTestRequest(t, router, http.MethodPost,
			"/v1/admin/tenants/"+fixtures.target.id+"/reactivate",
			fixtures.admin.id, map[string]string{"reason": "duplicate"}, nil)
		assertAdminError(t, response, http.StatusConflict, "conflict")

		response = serveAdminTestRequest(t, router, http.MethodPost,
			"/v1/admin/tenants/"+fixtures.target.id+"/plan",
			fixtures.admin.id, map[string]string{"plan": "starter", "reason": "approved upgrade"}, nil)
		assertAdminPlanStatus(t, response, http.StatusOK, "starter", "free")
		response = serveAdminTestRequest(t, router, http.MethodPost,
			"/v1/admin/tenants/"+fixtures.target.id+"/plan",
			fixtures.admin.id, map[string]string{"plan": "starter", "reason": "duplicate"}, nil)
		assertAdminError(t, response, http.StatusConflict, "conflict")
	})

	t.Run("successful changes are audited newest first", func(t *testing.T) {
		response := serveAdminTestRequest(t, router, http.MethodGet, "/v1/admin/audit?page=1&page_size=100", fixtures.admin.id, nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("audit status = %d, body = %s", response.Code, response.Body.String())
		}
		var body struct {
			Entries []struct {
				Action         string `json:"action"`
				TargetTenantID string `json:"target_tenant_id"`
				Reason         string `json:"reason"`
				PreviousPlan   string `json:"previous_plan"`
				NewPlan        string `json:"new_plan"`
				Actor          struct {
					ID string `json:"id"`
				} `json:"actor"`
			} `json:"audit"`
			Total int64 `json:"total"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode audit list: %v", err)
		}
		var found []string
		for _, entry := range body.Entries {
			if entry.TargetTenantID == fixtures.target.id {
				if entry.Actor.ID != fixtures.admin.id {
					t.Fatalf("audit actor = %q, want %q", entry.Actor.ID, fixtures.admin.id)
				}
				found = append(found, entry.Action+":"+entry.Reason)
			}
		}
		if len(found) != 3 || found[0] != "tenant_plan_changed:approved upgrade" || found[1] != "tenant_reactivated:appeal accepted" || found[2] != "tenant_suspended:abuse investigation" {
			t.Fatalf("unexpected audit entries (newest first): %v", found)
		}
		var planChangeFound bool
		for _, entry := range body.Entries {
			if entry.TargetTenantID == fixtures.target.id && entry.Action == "tenant_plan_changed" {
				planChangeFound = entry.PreviousPlan == "free" && entry.NewPlan == "starter"
			}
		}
		if !planChangeFound {
			t.Fatal("plan change audit did not record previous and new plans")
		}
		if body.Total < 2 {
			t.Fatalf("audit total = %d, want at least 2", body.Total)
		}

		response = serveAdminTestRequest(t, router, http.MethodGet, "/v1/admin/audit?page_size=101", fixtures.admin.id, nil, nil)
		assertAdminError(t, response, http.StatusBadRequest, "invalid_request")
	})
}

func createAdminTestAccounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) struct {
	admin     adminTestAccount
	tenant    adminTestAccount
	protected adminTestAccount
	target    adminTestAccount
} {
	t.Helper()
	prefix := fmt.Sprintf("%d", time.Now().UnixNano())
	create := func(label string) adminTestAccount {
		t.Helper()
		email := fmt.Sprintf("%s-%s@example.test", label, prefix)
		var id string
		if err := pool.QueryRow(ctx,
			`INSERT INTO tenants (name, email, password_hash, plan) VALUES ($1, $2, $3, 'free') RETURNING id::text`,
			label+" admin test", email, "admin-api-test-not-a-password-hash",
		).Scan(&id); err != nil {
			t.Fatalf("create test tenant: %v", err)
		}
		return adminTestAccount{id: id, email: email}
	}
	fixtures := struct {
		admin     adminTestAccount
		tenant    adminTestAccount
		protected adminTestAccount
		target    adminTestAccount
	}{
		admin:     create("admin"),
		tenant:    create("tenant"),
		protected: create("protected-admin"),
		target:    create("target"),
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, account := range []adminTestAccount{fixtures.admin, fixtures.tenant, fixtures.protected, fixtures.target} {
			if _, err := pool.Exec(cleanupCtx,
				`DELETE FROM admin_audit WHERE actor_tenant_id = $1 OR target_tenant_id = $1`, account.id); err != nil {
				t.Errorf("delete test audit entries: %v", err)
			}
		}
		for _, account := range []adminTestAccount{fixtures.admin, fixtures.tenant, fixtures.protected, fixtures.target} {
			if _, err := pool.Exec(cleanupCtx, `DELETE FROM tenants WHERE id = $1`, account.id); err != nil {
				t.Errorf("delete test tenant: %v", err)
			}
		}
	})
	return fixtures
}

func serveAdminTestRequest(
	t *testing.T,
	router http.Handler,
	method, path, tenantID string,
	body any,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody *strings.Reader
	if body == nil {
		requestBody = strings.NewReader("")
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request body: %v", err)
		}
		requestBody = strings.NewReader(string(encoded))
	}
	request := httptest.NewRequest(method, path, requestBody)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if tenantID != "" {
		token, err := auth.NewJWT("test-only-admin-jwt-secret", tenantID)
		if err != nil {
			t.Fatalf("create test session: %v", err)
		}
		request.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertAdminError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error: %v; body = %s", err, response.Body.String())
	}
	if body.Error.Code != code {
		t.Fatalf("error code = %q, want %q; body = %s", body.Error.Code, code, response.Body.String())
	}
}

func assertAdminStatus(t *testing.T, response *httptest.ResponseRecorder, status int, wantState string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if body.Status != wantState {
		t.Fatalf("status field = %q, want %q", body.Status, wantState)
	}
}

func assertAdminPlanStatus(t *testing.T, response *httptest.ResponseRecorder, status int, wantPlan, wantPrevious string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
	var body struct {
		Plan         string `json:"plan"`
		PreviousPlan string `json:"previous_plan"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode plan response: %v", err)
	}
	if body.Plan != wantPlan || body.PreviousPlan != wantPrevious {
		t.Fatalf("plan response = %+v, want plan %q and previous plan %q", body, wantPlan, wantPrevious)
	}
}
