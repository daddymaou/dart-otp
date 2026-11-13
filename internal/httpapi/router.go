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

func (a *API) OTPService() *otp.Service { return a.otpSvc }

func New(cfg *config.Config, pool *pgxpool.Pool, otpSender otp.Sender) (*gin.Engine, *API) {
	api := &API{cfg: cfg, pool: pool}
	api.otpSvc = otp.NewService(cfg, pool, otpSender)
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
	auth.POST("/signup", api.signup)
	auth.POST("/login", api.login)
	auth.POST("/logout", api.logout)
	auth.GET("/me", api.jwtMiddleware(), api.me)
	auth.DELETE("/me", api.jwtMiddleware(), api.deleteMe)

	jwtGroup := router.Group("/v1", api.jwtMiddleware())
	jwtGroup.POST("/api-keys", api.createAPIKey)
	jwtGroup.GET("/api-keys", api.listAPIKeys)
	jwtGroup.DELETE("/api-keys/:id", api.revokeAPIKey)

	jwtGroup.GET("/templates", api.listTemplates)
	jwtGroup.POST("/templates", api.createTemplate)
	jwtGroup.GET("/templates/:id", api.getTemplate)
	jwtGroup.PUT("/templates/:id", api.updateTemplate)
	jwtGroup.DELETE("/templates/:id", api.deleteTemplate)
	jwtGroup.POST("/templates/:id/test", api.testTemplate)

	jwtGroup.GET("/logs", api.logs)
	jwtGroup.GET("/stats", api.stats)

	otpGroup := router.Group("/v1/otp")
	otpGroup.POST("/send", api.apiKeyMiddleware(), api.otpSend)
	otpGroup.POST("/verify", api.apiKeyMiddleware(), api.otpVerify)
	otpGroup.POST("/resend", api.apiKeyMiddleware(), api.otpResend)

	return router, api
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
