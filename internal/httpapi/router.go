package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/daddymaou/dart-otp/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type API struct {
	cfg  *config.Config
	pool *pgxpool.Pool
}

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

	otp := router.Group("/v1/otp") // TODO step 5: send, verify, resend
	_ = otp

	return router
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
