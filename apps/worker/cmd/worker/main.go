package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aerosight/worker/internal/algorithm"
	"aerosight/worker/internal/automation"
	"aerosight/worker/internal/config"
	"aerosight/worker/internal/dji"
	"aerosight/worker/internal/driver"
	"aerosight/worker/internal/heartbeat"
	"aerosight/worker/internal/media"
	"aerosight/worker/internal/mission"
	"aerosight/worker/internal/observability"
	"aerosight/worker/internal/outbox"
	"aerosight/worker/internal/perception"
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

func (store algorithmRawStore) ReadAlgorithmAsset(ctx context.Context, key string) (algorithm.AlgorithmAsset, error) {
	object, err := store.storage.GetObject(ctx, key)
	if err != nil {
		return algorithm.AlgorithmAsset{}, err
	}
	return algorithm.AlgorithmAsset{Body: object.Body, ContentType: object.ContentType}, nil
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
	driverRegistry := driver.NewRegistry()
	if err := dji.RegisterDriver(driverRegistry, func(context.Context, driver.AdapterConfig) error { return nil }); err != nil {
		logger.Error("DJI driver registration failed", "error", err.Error())
		os.Exit(1)
	}
	djiIngestor := dji.NewMessageIngestor(dji.NewSQLIngressStore(database))
	djiProjector := dji.NewProjector()
	consumer.Register("device.topology", djiProjector.Handler)
	consumer.Register("device.state", djiProjector.Handler)
	consumer.Register("device.telemetry", djiProjector.Handler)
	djiManager := dji.NewAdapterManager(
		dji.NewSQLLeaseRepository(database), dji.EnvironmentSecretResolver{},
		func(ctx context.Context, config dji.MQTTConfig, handler dji.MQTTMessageHandler) (dji.ManagedSession, error) {
			return dji.StartMQTTSession(ctx, config, handler)
		},
		func(lease dji.AdapterLease) dji.MQTTMessageHandler {
			scope, err := dji.RouteContextFromLease(lease)
			if err != nil {
				return func(context.Context, dji.MQTTMessage) error { return err }
			}
			return djiIngestor.Handle(scope)
		},
		workerConfig.WorkerName+":"+runID, logger,
	)
	djiCommandDispatcher, err := dji.NewCommandDispatcher(djiManager, nil)
	if err != nil {
		logger.Error("DJI command dispatcher initialization failed", "error", err.Error())
		os.Exit(1)
	}
	consumer.Register("device.command.dispatch", djiCommandDispatcher.DispatchHandler)
	consumer.Register("command.reply", djiCommandDispatcher.ReplyHandler)
	consumer.Register("device.event", djiCommandDispatcher.EventHandler)
	var liveStreamHealth *dji.LiveStreamHealthCoordinator
	if workerConfig.MediaAPIBaseURL != "" {
		mediaInspector, mediaErr := dji.NewMediaMTXInspector(
			workerConfig.MediaAPIBaseURL, workerConfig.MediaAPIUser, workerConfig.MediaAPIPassword, nil,
		)
		if mediaErr != nil {
			logger.Error("MediaMTX inspector initialization failed", "error", mediaErr.Error())
			os.Exit(1)
		}
		liveStreamHealth, mediaErr = dji.NewLiveStreamHealthCoordinator(
			mediaInspector, workerConfig.WorkerName+":"+runID, nil,
		)
		if mediaErr != nil {
			logger.Error("live stream health coordinator initialization failed", "error", mediaErr.Error())
			os.Exit(1)
		}
	}
	missionProcessor := mission.NewProcessor(nil)
	consumer.Register("task_run.transitioned", missionProcessor.Handler)
	consumer.Register("mission.control", missionProcessor.Handler)
	consumer.Register("command.ack", missionProcessor.Handler)
	var rawStore algorithm.RawResultStore
	var assetStore algorithm.AlgorithmAssetStore
	var assetHandler outbox.Handler
	if workerConfig.ObjectStorageLocalRoot == "" {
		assetHandler = func(context.Context, *sql.Tx, outbox.Event) error {
			return errors.New("OBJECT_STORAGE_LOCAL_ROOT is not configured")
		}
		logger.Warn("media derivative processing unavailable", "reason", "OBJECT_STORAGE_LOCAL_ROOT is not configured")
	} else {
		storage, err := media.NewLocalObjectStorage(workerConfig.ObjectStorageLocalRoot)
		if err != nil {
			logger.Error("object storage initialization failed", "error", err.Error())
			os.Exit(1)
		}
		processor := media.NewProcessor(storage, media.NewSQLRepository())
		assetHandler = processor.Handler
		rawStore = algorithmRawStore{storage: storage}
		assetStore = algorithmRawStore{storage: storage}
	}
	assetSigner := algorithm.NewAssetURLSigner(workerConfig.AssetURLSigningSecret, workerConfig.CallbackPublicBaseURL)
	detectionSink := perception.NewSQLDetectionSink()
	algorithmTrigger := algorithm.NewTrigger(assetSigner)
	consumer.Register("asset.available", func(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
		if err := assetHandler(ctx, tx, event); err != nil {
			return err
		}
		return algorithmTrigger.Handler(ctx, tx, event)
	})
	algorithmProcessor := algorithm.NewProcessor(
		algorithm.DefaultHTTPClient(), algorithm.NewCircuitBreaker(3, 30*time.Second), rawStore,
		workerConfig.CallbackPublicBaseURL, assetSigner, detectionSink,
	)
	consumer.Register("algorithm.run.requested", algorithmProcessor.Handler)
	consumer.Register("alert.automation.requested", automation.Processor{Generator: automation.DeterministicDraftGenerator{}}.Handler)
	wake := wakeup.Postgres(ctx, workerConfig.DatabaseURL, logger)
	runContext, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	callbackMux := http.NewServeMux()
	callbackMux.Handle("/metrics", observability.DefaultMetrics)
	healthHandler := observability.NewHealthHandler([]observability.DependencyCheck{
		{Name: "database", Critical: true, Check: database.PingContext},
		{Name: "object_storage", Critical: false, Check: func(context.Context) error {
			if workerConfig.ObjectStorageLocalRoot == "" {
				return errors.New("object storage is not configured")
			}
			return nil
		}},
	}, 2*time.Second)
	callbackMux.Handle("/healthz", healthHandler)
	callbackMux.Handle("/readyz", healthHandler)
	callbackMux.Handle("/callbacks/algorithms/", algorithm.NewCallbackHandler(database, rawStore, detectionSink))
	callbackMux.Handle("/algorithm-assets/", algorithm.NewAssetAccessHandler(database, assetStore, assetSigner))
	callbackServer := &http.Server{
		Addr: workerConfig.CallbackListenAddress, Handler: callbackMux,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second,
		IdleTimeout: 60 * time.Second,
	}
	runErrors := make(chan error, 5)
	go func() { runErrors <- consumer.RunWithWake(runContext, wake) }()
	go func() { runErrors <- heartbeat.NewProjector(database, nil).Run(runContext, 15*time.Second) }()
	go func() { runErrors <- djiManager.Run(runContext) }()
	go func() { runErrors <- djiCommandDispatcher.RunTimeoutReconciler(runContext, database, time.Second) }()
	if liveStreamHealth != nil {
		go func() { runErrors <- liveStreamHealth.Run(runContext, database, 2*time.Second) }()
	}
	go func() {
		logger.Info("algorithm callback endpoint started", "address", workerConfig.CallbackListenAddress)
		err := callbackServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		runErrors <- err
	}()
	if err := <-runErrors; err != nil {
		cancelRun()
		_ = callbackServer.Shutdown(context.Background())
		logger.Error("worker stopped unexpectedly", "error", err.Error())
		os.Exit(1)
	}
	cancelRun()
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	_ = callbackServer.Shutdown(shutdownContext)
	logger.Info("worker stopped")
}
