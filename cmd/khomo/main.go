package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Normious/Khomo/internal/config"
	"github.com/Normious/Khomo/internal/db"
	"github.com/Normious/Khomo/internal/handlers"
	"github.com/Normious/Khomo/internal/health"
	"github.com/Normious/Khomo/internal/proxy"
	"github.com/Normious/Khomo/internal/routing"
)

func main() {
	cfg := config.Load()
	cfg.SetupLogger()

	database, err := db.Open(cfg.Database)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Load routes into registry
	registry, err := routing.NewRegistry(database)
	if err != nil {
		slog.Error("Failed to load routes", "error", err)
		os.Exit(1)
	}

	cbCfg := proxy.CBConfig{
		FailureThreshold: cfg.CBFailureThreshold,
		SuccessThreshold: cfg.CBSuccessThreshold,
		Timeout:          time.Duration(cfg.CBTimeoutSeconds) * time.Second,
		HalfOpenMax:      cfg.CBHalfOpenMaxCalls,
	}

	// Build proxy + register all route pools
	px := proxy.NewProxy(database)
	px.SetPreserveHost(cfg.PreserveHostHeader)
	px.SyncRoutes(registry.Get().Routes(), registry.Get().BackendsFor, cbCfg)

	// Start health checker
	checker := health.NewChecker(database, cfg.HealthCheckEnabled,
		cfg.HealthCheckTimeoutSeconds, cfg.HealthCheckPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checker.Start(ctx, cfg.HealthCheckIntervalSeconds)

	// Handlers
	gatewayH := handlers.NewGatewayHandler(database, cfg, registry, px)
	adminH := handlers.NewAdminHandler(registry, database, px, cbCfg)
	healthH := handlers.NewHealthHandler(registry, checker)
	statsH := handlers.NewStatsHandler(database)

	// Route wiring
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthH.Handle)
	mux.HandleFunc("GET /gateway/stats", statsH.Handle)
	mux.HandleFunc("POST /gateway/admin/reload", adminH.Reload)
	mux.HandleFunc("GET /gateway/admin/routes", adminH.ListRoutes)
	mux.HandleFunc("POST /gateway/admin/routes", adminH.CreateRoute)
	mux.HandleFunc("DELETE /gateway/admin/routes/{id}", adminH.DeleteRoute)
	mux.HandleFunc("POST /gateway/admin/backends", adminH.CreateBackend)
	mux.HandleFunc("DELETE /gateway/admin/backends/{id}", adminH.DeleteBackend)
	mux.HandleFunc("POST /gateway/admin/projects", adminH.CreateProject)
	mux.HandleFunc("GET /gateway/admin/projects", adminH.ListProjects)

	// Gateway catches everything else
	mux.Handle("/", gatewayH)

	srv := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", cfg.Port),
		Handler:      chain(mux),
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("Shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("Khomo listening", "port", cfg.Port, "routes", len(registry.Get().Routes()))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}

func chain(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Single CORS config for the whole platform (spec §1).
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Request-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
		slog.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds())
	})
}
