package main

import (
	"aerosight/server/internal/config"
	"aerosight/server/internal/database"
	"aerosight/server/internal/flighthub"
	"aerosight/server/internal/httpapi"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/runtime"
	"aerosight/server/internal/webassets"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("application stopped", "error", err)
		os.Exit(1)
	}
}
func run(logger *slog.Logger) error {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command != "serve" && command != "migrate" {
		return fmt.Errorf("usage: aerosight [serve|migrate]")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is required")
	}
	if command == "migrate" {
		db, err := database.Open(ctx, url, 1)
		if err != nil {
			return err
		}
		defer db.Close()
		applied, err := migrations.Embedded(ctx, db)
		if err != nil {
			return err
		}
		logger.Info("migrations complete", "applied", len(applied))
		return nil
	}
	httpCfg, err := config.LoadHTTP(os.Getenv)
	if err != nil {
		return err
	}
	pages, err := webassets.Embedded()
	if err != nil {
		return err
	}
	workerCfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(workerCfg.AuthSecret) < 32 {
		return errors.New("AUTH_SECRET must contain at least 32 characters")
	}
	if workerCfg.CallbackPublicBaseURL == "" {
		workerCfg.CallbackPublicBaseURL = httpCfg.PublicOrigin
	}
	db, err := database.Open(ctx, url, httpCfg.HTTPPool)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err = migrations.Embedded(ctx, db); err != nil {
		return err
	}
	if err = database.Bootstrap(ctx, db); err != nil {
		return err
	}
	bgDB, err := database.Open(ctx, url, httpCfg.WorkerPool)
	if err != nil {
		return err
	}
	defer bgDB.Close()
	bg, err := runtime.New(bgDB, workerCfg, logger)
	if err != nil {
		return err
	}
	api, err := httpapi.New(db, httpCfg, logger)
	if err != nil {
		return err
	}
	defer api.Close()
	api.AttachStaticPages(pages)
	api.AttachRuntime(bg.Callbacks)
	flightHubClient, err := flighthub.NewChinaClient(flighthub.Config{Timeout: workerCfg.FlightHubHTTPTimeout, MaxRetries: workerCfg.FlightHubMaxRetries, MaxProjectPages: workerCfg.FlightHubMaxProjectPages, MaxResponseBytes: workerCfg.FlightHubMaxResponseBytes, RequestID: uuid.NewString})
	if err != nil {
		return err
	}
	api.AttachFlightHub(flightHubClient, workerCfg.FlightHubEnabled, workerCfg.AuthSecret)
	api.AttachDeviceCredentials(workerCfg.AuthSecret)
	api.AttachMediaStorage(workerCfg.ObjectStorageLocalRoot)
	server := &http.Server{Addr: httpCfg.Address, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	results := make(chan error, 2)
	go func() { results <- bg.Run(ctx) }()
	go func() { results <- server.ListenAndServe() }()
	api.SetReady(true)
	var first error
	completed := 0
	select {
	case <-ctx.Done():
	case first = <-results:
		completed++
	}
	api.SetReady(false)
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), httpCfg.ShutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdown)
	for completed < 2 {
		select {
		case err := <-results:
			completed++
			if first == nil && !errors.Is(err, http.ErrServerClosed) {
				first = err
			}
		case <-shutdown.Done():
			return shutdown.Err()
		}
	}
	if errors.Is(first, http.ErrServerClosed) {
		first = nil
	}
	if first != nil {
		return first
	}
	return shutdownErr
}
