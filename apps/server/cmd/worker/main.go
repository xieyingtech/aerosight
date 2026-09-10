package main

import (
	"aerosight/server/internal/config"
	"aerosight/server/internal/observability"
	"aerosight/server/internal/runtime"
	"context"
	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Transitional entrypoint until the unified API migration is complete.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = db.PingContext(ctx); err != nil {
		return err
	}
	rt, err := runtime.New(db, cfg, logger)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/", rt.Callbacks)
	health := observability.NewHealthHandler([]observability.DependencyCheck{{Name: "database", Critical: true, Check: db.PingContext}}, 2*time.Second)
	mux.Handle("/healthz", health)
	mux.Handle("/readyz", health)
	mux.Handle("/metrics", observability.DefaultMetrics)
	server := &http.Server{Addr: cfg.CallbackListenAddress, Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	done := make(chan error, 2)
	go func() { done <- rt.Run(ctx) }()
	go func() { done <- server.ListenAndServe() }()
	var result error
	select {
	case <-ctx.Done():
	case result = <-done:
	}
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
	if result == http.ErrServerClosed {
		return nil
	}
	return result
}
