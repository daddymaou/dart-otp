package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/daddymaou/dart-otp/internal/bot"
	"github.com/daddymaou/dart-otp/internal/config"
	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/daddymaou/dart-otp/internal/httpapi"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/gin-gonic/gin"
)

//go:embed all:migrations
var migrationsFS embed.FS

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runMigrations(cfg.DatabaseURL); err != nil {
		slog.Error("migrations", "error", err)
		os.Exit(1)
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if cfg.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	var telegramBot *bot.Bot
	if cfg.TelegramBotToken != "" {
		telegramBot, err = bot.Start(cfg, pool)
		if err != nil {
			slog.Error("telegram bot", "error", err)
			os.Exit(1)
		}
		defer telegramBot.Stop()
	}

	router := httpapi.New(cfg, pool)
	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("server starting", "addr", cfg.HTTPAddr, "env", cfg.AppEnv)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

func runMigrations(databaseURL string) error {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, databaseURL)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
