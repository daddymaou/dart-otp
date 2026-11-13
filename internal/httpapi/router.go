package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/daddymaou/dart-otp/internal/config"
	"github.com/daddymaou/dart-otp/internal/otp"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type API struct {
	cfg    *config.Config
	pool   *pgxpool.Pool
	otpSvc *otp.Service
}

// SetOTPService wires the OTP core in before route registration.
func (a *API) SetOTPService(s *otp.Service) { a.otpSvc = s }

func New(cfg *config.Config, pool *pgxpool.Pool) *gin.Engine {
	api := &API{cfg: cfg, pool: pool}
	router := gin.New()

	router.Use(gin.CustomRecovery(func(c *gin.Context, _ any) {
		fail(c, "internal", "Something went wrong.", nil)
	}))
	router.Use(RequestID())
	router.Use(RequestLogger())
	router.Use(CORSMiddleware(cfg.CORSAllowedOrigins))

	router.GET("/health", api.health)
	router.NoRoute(noRoute)

	if cfg.AppEnv == "development" {
		router.POST("/dev/link-token", api.devLinkToken)
	}

	auth := router.Group("/v1/auth")
	auth.Group("") // TODO step 6: signup, login, logout, me

	apiKeys := router.Group("/v1") // TODO step 7: JWT-protected api-keys, templates, logs, stats, webhooks
	_ = apiKeys

	otpGroup := router.Group("/v1/otp")
	otpGroup.POST("/send", api.devTenantMiddleware(), api.otpSend)
	otpGroup.POST("/verify", api.devTenantMiddleware(), api.otpVerify)
	otpGroup.POST("/resend", api.devTenantMiddleware(), api.otpResend)

	return router
}

func (a *API) devTenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.cfg.AppEnv != "development" {
			fail(c, "unauthorized", "Missing or invalid API key.", nil)
			return
		}
		t := c.GetHeader("X-Dev-Tenant")
		if t == "" {
			fail(c, "unauthorized", "X-Dev-Tenant header required in development.", nil)
			return
		}
		c.Set("tenant_id", t)
		c.Next()
	}
}

func (a *API) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	dbOK := a.pool.Ping(ctx) == nil
	status := "ok"
	if !dbOK {
		status = "degraded"
	}
	c.JSON(http.StatusOK, gin.H{"status": status, "db": dbOK})
}
