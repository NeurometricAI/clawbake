package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/clawbake/clawbake/db"
	"github.com/clawbake/clawbake/internal/auth"
	"github.com/clawbake/clawbake/internal/bot"
	"github.com/clawbake/clawbake/internal/config"
	"github.com/clawbake/clawbake/internal/database"
	"github.com/clawbake/clawbake/internal/handler"
	"github.com/clawbake/clawbake/internal/k8s"
	"github.com/clawbake/clawbake/internal/version"
	"github.com/clawbake/clawbake/web/templates"
)

func runMigrations(databaseURL string) error {
	src, err := iofs.New(db.Migrations, "migrations")
	if err != nil {
		return err
	}
	// golang-migrate pgx5 driver expects "pgx5://" scheme
	connStr := strings.Replace(databaseURL, "postgresql://", "pgx5://", 1)
	connStr = strings.Replace(connStr, "postgres://", "pgx5://", 1)

	// Retry connecting — PostgreSQL may still be starting up
	var m *migrate.Migrate
	for attempt := 1; attempt <= 30; attempt++ {
		m, err = migrate.NewWithSourceInstance("iofs", src, connStr)
		if err == nil {
			break
		}
		log.Printf("waiting for database (attempt %d/30): %v", attempt, err)
		time.Sleep(2 * time.Second)
		// Re-create source since NewWithSourceInstance consumes it
		src, _ = iofs.New(db.Migrations, "migrations")
	}
	if err != nil {
		return fmt.Errorf("database not ready after 30 attempts: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func syncInstanceDefaults(ctx context.Context, db *database.Queries, cfg *config.Config) error {
	current, err := db.GetDefaults(ctx)
	if err != nil {
		return fmt.Errorf("reading current defaults: %w", err)
	}

	if cfg.InstanceDefaultImage != "" {
		current.Image = cfg.InstanceDefaultImage
	}
	if cfg.InstanceDefaultCPURequest != "" {
		current.CpuRequest = cfg.InstanceDefaultCPURequest
	}
	if cfg.InstanceDefaultMemoryRequest != "" {
		current.MemoryRequest = cfg.InstanceDefaultMemoryRequest
	}
	if cfg.InstanceDefaultCPULimit != "" {
		current.CpuLimit = cfg.InstanceDefaultCPULimit
	}
	if cfg.InstanceDefaultMemoryLimit != "" {
		current.MemoryLimit = cfg.InstanceDefaultMemoryLimit
	}
	if cfg.InstanceDefaultStorageSize != "" {
		current.StorageSize = cfg.InstanceDefaultStorageSize
	}

	if cfg.InstanceDefaultGatewayConfig != "" {
		current.GatewayConfig = cfg.InstanceDefaultGatewayConfig
	}

	_, err = db.UpdateDefaults(ctx, database.UpdateDefaultsParams{
		Image:         current.Image,
		CpuRequest:    current.CpuRequest,
		MemoryRequest: current.MemoryRequest,
		CpuLimit:      current.CpuLimit,
		MemoryLimit:   current.MemoryLimit,
		StorageSize:   current.StorageSize,
		GatewayConfig: current.GatewayConfig,
	})
	return err
}

func main() {
	migrateFlag := flag.Bool("migrate", false, "Run database migrations and exit")
	flag.Parse()

	cfg := config.Load()
	templates.AppVersion = version.Version
	log.Printf("clawbake server %s", version.Version)

	if *migrateFlag {
		log.Println("running database migrations...")
		if err := runMigrations(cfg.DatabaseURL); err != nil {
			log.Fatalf("migration failed: %v", err)
		}
		log.Println("migrations completed successfully")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	db := database.New(pool)

	// Sync instance defaults from config/env into the database.
	// This keeps the DB in sync with Helm values on every deploy.
	if err := syncInstanceDefaults(ctx, db, cfg); err != nil {
		log.Fatalf("failed to sync instance defaults: %v", err)
	}

	var oidcAuth *auth.OIDCAuth
	var devAuth *auth.DevAuth
	if cfg.OIDCIssuer != "" {
		oidcAuth, err = auth.NewOIDCAuth(ctx, cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.OIDCRedirectURL, cfg.SessionSecret, cfg.BaseURL, db)
		if err != nil {
			log.Fatalf("failed to setup OIDC: %v", err)
		}
	} else {
		log.Println("OIDC not configured, using dev login")
		devAuth = auth.NewDevAuth(cfg.SessionSecret, cfg.BaseURL, db)
	}

	k8sClient, k8sConfig, err := k8s.NewClient()
	if err != nil {
		log.Fatalf("failed to create k8s client: %v", err)
	}

	var slackBot *bot.Bot
	if cfg.SlackBotToken != "" && cfg.SlackSigningSecret != "" {
		slackBot = bot.New(cfg.SlackBotToken, cfg.SlackSigningSecret, db, k8sClient, cfg.KubeNamespace, cfg.BaseURL, cfg.TUIEnabled, cfg.ShellEnabled)
		log.Println("Slack bot enabled")
	}

	h := &handler.Handler{
		DB:        db,
		K8s:       k8sClient,
		K8sConfig: k8sConfig,
		Auth:      oidcAuth,
		DevAuth:   devAuth,
		Config:    cfg,
		Bot:       slackBot,
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
	}))

	h.RegisterRoutes(e)

	go func() {
		if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}

	log.Println("server stopped")
}
