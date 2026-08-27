package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aerosight/worker/internal/algorithm"
	"aerosight/worker/internal/config"
	"aerosight/worker/internal/heartbeat"
	"aerosight/worker/internal/media"
	"aerosight/worker/internal/mission"
	"aerosight/worker/internal/observability"
	"aerosight/worker/internal/outbox"
	"aerosight/worker/internal/wakeup"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type algorithmRawStore struct {
	storage *media.LocalObjectStorage
}

func (store algorithmRawStore) PutRawResult(
	ctx context.Context, key string, reader io.Reader, contentType string,
) (algorithm.RawResultObject, error) {
	object, err := store.storage.PutObject(ctx, key, reader, contentType)
	if err != nil {
		return algorithm.RawResultObject{}, err
	}
	return algorithm.RawResultObject{Key: object.Key, ChecksumSHA256: object.ChecksumSHA256}, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	workerConfig, err := config.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("worker configuration invalid", "error", err.Error())
		os.Exit(1)
	}

	runID := observability.CorrelationID("")
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		"worker", workerConfig.WorkerName,
		"request_id", runID,
	)
	logger.Info("worker started")

	database, err := sql.Open("pgx", workerConfig.DatabaseURL)
	if err != nil {
		logger.Error("database driver initialization failed", "error", err.Error())
		os.Exit(1)
	}
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		logger.Error("database connection failed", "error", err.Error())
		os.Exit(1)
	}

	consumer := outbox.NewConsumer(outbox.NewStore(database), runID, "aerosight-worker", logger)
	missionProcessor := mission.NewProcessor(nil)
	consumer.Register("task_run.transitioned", missionProcessor.Handler)
	consumer.Register("mission.control", missionProcessor.Handler)
	consumer.Register("command.ack", missionProcessor.Handler)
	var rawStore algorithm.RawResultStore
	if workerConfig.ObjectStorageLocalRoot == "" {
		consumer.Register("asset.available", func(context.Context, *sql.Tx, outbox.Event) error {
			return errors.New("OBJECT_STORAGE_LOCAL_ROOT is not configured")
		})
		logger.Warn("media derivative processing unavailable", "reason", "OBJECT_STORAGE_LOCAL_ROOT is not configured")
	} else {
		storage, err := media.NewLocalObjectStorage(workerConfig.ObjectStorageLocalRoot)
		if err != nil {
			logger.Error("object storage initialization failed", "error", err.Error())
			os.Exit(1)
		}
		processor := media.NewProcessor(storage, media.NewSQLRepository())
		consumer.Register("asset.available", processor.Handler)
		rawStore = algorithmRawStore{storage: storage}
	}
	algorithmProcessor := algorithm.NewProcessor(algorithm.DefaultHTTPClient(), algorithm.NewCircuitBreaker(3, 30*time.Second), rawStore)
	consumer.Register("algorithm.run.requested", algorithmProcessor.Handler)
	wake := wakeup.Postgres(ctx, workerConfig.DatabaseURL, logger)
	runContext, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	errors := make(chan error, 2)
	go func() { errors <- consumer.RunWithWake(runContext, wake) }()
	go func() { errors <- heartbeat.NewProjector(database, nil).Run(runContext, 15*time.Second) }()
	if err := <-errors; err != nil {
		cancelRun()
		logger.Error("worker stopped unexpectedly", "error", err.Error())
		os.Exit(1)
	}
	cancelRun()
	logger.Info("worker stopped")
}
