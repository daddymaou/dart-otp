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

	router.GET("/health", api.health)
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
