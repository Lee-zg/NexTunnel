package main

import (
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nextunnel/server/internal/dashboard"
)

const (
	defaultDashboardListenAddr  = "0.0.0.0:8080"
	defaultDashboardTokenExpiry = 24 * time.Hour
	defaultDashboardAdmin       = "admin"
)

var dashboardVersion = "0.6.4-alpha"

func main() {
	fs := flag.NewFlagSet("dashboard", flag.ExitOnError)
	cfg := dashboard.DefaultServerConfig()
	var allowedOrigins string
	var storePath string
	var tokenExpiry time.Duration

	cfg.ListenAddr = defaultDashboardListenAddr
	cfg.Auth.DefaultAdmin = defaultDashboardAdmin
	cfg.Auth.TokenExpiry = defaultDashboardTokenExpiry
	fs.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "Dashboard HTTP listen address")
	fs.StringVar(&allowedOrigins, "allowed-origins", strings.Join(cfg.AllowedOrigins, ","), "comma-separated CORS allowed origins")
	fs.StringVar(&cfg.Auth.SecretKey, "secret-key", cfg.Auth.SecretKey, "Dashboard auth secret key")
	fs.StringVar(&cfg.Auth.DefaultAdmin, "admin-user", cfg.Auth.DefaultAdmin, "default admin username")
	fs.StringVar(&cfg.Auth.DefaultPass, "admin-password", cfg.Auth.DefaultPass, "default admin password; required when the store has no users")
	fs.DurationVar(&tokenExpiry, "token-expiry", cfg.Auth.TokenExpiry, "Dashboard auth token expiry")
	fs.StringVar(&storePath, "store-path", "", "SQLite database path for persistent storage; empty uses in-memory store")
	fs.StringVar(&cfg.StaticDir, "static-dir", cfg.StaticDir, "optional Dashboard web static assets directory")
	fs.StringVar(&cfg.TLSCertFile, "tls-cert", "", "TLS certificate file for HTTPS (enables HTTPS when set)")
	fs.StringVar(&cfg.TLSKeyFile, "tls-key", "", "TLS private key file for HTTPS")
	fs.StringVar(&cfg.AuditLogPath, "audit-log", "", "JSON Lines audit log path (empty = disabled)")
	fs.StringVar(&cfg.RelayAdminURL, "relay-admin-url", "", "optional Relay admin API base URL for client monitoring")
	fs.StringVar(&cfg.RelayAdminToken, "relay-admin-token", "", "Bearer token for Relay admin API")
	fs.Parse(os.Args[1:])

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	cfg.Logger = logger
	cfg.Auth.TokenExpiry = tokenExpiry
	cfg.AllowedOrigins = parseAllowedOrigins(allowedOrigins)

	var store dashboard.DashboardStore
	if storePath != "" {
		createdStore, err := dashboard.NewSQLiteDashboardStore(storePath)
		if err != nil {
			logger.Error("failed to create dashboard store", "error", err)
			os.Exit(1)
		}
		store = createdStore
		defer createdStore.Close()
	}
	cfg.Store = store
	cfg.StorePath = storePath
	cfg.Version = dashboardVersion

	server := dashboard.NewServer(cfg)
	errCh := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	logger.Info("NexTunnel Dashboard started", "addr", cfg.ListenAddr, "store_path", storePath, "static_dir", cfg.StaticDir)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		logger.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		if err != nil {
			logger.Error("failed to start dashboard", "error", err)
			os.Exit(1)
		}
	}

	if err := server.Stop(); err != nil {
		logger.Error("dashboard shutdown error", "error", err)
		os.Exit(1)
	}
	logger.Info("NexTunnel Dashboard stopped")
}

// parseAllowedOrigins 将逗号分隔配置转成 CORS 白名单，过滤空值以减少误配置。
func parseAllowedOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}
