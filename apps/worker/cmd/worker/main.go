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

	"aerosight/worker/internal/agent"
	"aerosight/worker/internal/algorithm"
	"aerosight/worker/internal/config"
	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/dji"
	"aerosight/worker/internal/driver"
	"aerosight/worker/internal/flighthub"
	"aerosight/worker/internal/heartbeat"
	issueworker "aerosight/worker/internal/issue"
	"aerosight/worker/internal/media"
	"aerosight/worker/internal/mission"
	"aerosight/worker/internal/observability"
	"aerosight/worker/internal/outbox"
	"aerosight/worker/internal/perception"
	reportworker "aerosight/worker/internal/report"
	"aerosight/worker/internal/tasktrigger"
	"aerosight/worker/internal/telemetry"
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

func (store algorithmRawStore) ReadWaylineSource(ctx context.Context, key string) (flighthub.WaylineSourceObject, error) {
	object, err := store.storage.GetObject(ctx, key)
	if err != nil {
		return flighthub.WaylineSourceObject{}, err
	}
	return flighthub.WaylineSourceObject{Body: object.Body, ContentType: object.ContentType}, nil
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
	var flightHubScheduler *connector.Scheduler
	var flightHubClient *flighthub.Client
	var flightHubModelProjector flighthub.ModelJobProjector
	var flightHubControlReconciler *flighthub.SQLControlCommandReconciler
	var flightHubCommandStatusReconciler *flighthub.ControlCommandStatusReconciler
	flightHubTokenResolver := flighthub.EncryptedTokenResolver{AuthSecret: workerConfig.AuthSecret}
	if workerConfig.FlightHubEnabled {
		createdFlightHubClient, flightHubErr := flighthub.NewChinaClient(flighthub.Config{
			Timeout: workerConfig.FlightHubHTTPTimeout, MaxRetries: workerConfig.FlightHubMaxRetries,
			MaxProjectPages: 50, MaxResponseBytes: workerConfig.FlightHubMaxResponseBytes,
			RequestID:        func() string { return observability.CorrelationID("") },
			AllowedLinkHosts: workerConfig.FlightHubAllowedLinkHosts,
		})
		if flightHubErr != nil {
			logger.Error("FlightHub client initialization failed", "error", flightHubErr.Error())
			os.Exit(1)
		}
		flightHubClient = createdFlightHubClient
		connectorRegistry := connector.NewRegistry()
		if flightHubErr = flighthub.RegisterRuntime(
			connectorRegistry, flightHubClient, flightHubTokenResolver,
		); flightHubErr != nil {
			logger.Error("FlightHub runtime registration failed", "error", flightHubErr.Error())
			os.Exit(1)
		}
		synchronizer, syncErr := connector.NewSynchronizer(connectorRegistry, connector.NewSQLSyncStore(database))
		if syncErr != nil {
			logger.Error("connector synchronizer initialization failed", "error", syncErr.Error())
			os.Exit(1)
		}
		resourceRepository := connector.NewSQLResourceRepository(database)
		telemetryIngestor := telemetry.NewIngestor(database)
		flightHubControlReconciler, flightHubErr = flighthub.NewSQLControlCommandReconciler(database, nil)
		if flightHubErr != nil {
			logger.Error("FlightHub control reconciliation initialization failed", "error", flightHubErr.Error())
			os.Exit(1)
		}
		flightHubCommandStatusReconciler, flightHubErr = flighthub.NewControlCommandStatusReconciler(
			database, flightHubClient, flightHubTokenResolver, nil,
		)
		if flightHubErr != nil {
			logger.Error("FlightHub command status reconciliation initialization failed", "error", flightHubErr.Error())
			os.Exit(1)
		}
		liveReconciler, liveErr := flighthub.NewFlightHubLiveReconciler(database, nil)
		if liveErr != nil {
			logger.Error("FlightHub live reconciliation initialization failed", "error", liveErr.Error())
			os.Exit(1)
		}
		resourceSink, resourceErr := flighthub.NewSQLResourceStreamSink(
			telemetryIngestor, resourceRepository, heartbeat.NewProjector(database, nil), flighthub.NewSQLDeviceHealthProjector(database),
			flighthub.NewSQLFlightCatalogProjector(database, telemetryIngestor, nil, 30*time.Minute, workerConfig.AuthSecret),
			flightHubControlReconciler,
		)
		if resourceErr != nil {
			logger.Error("FlightHub resource sink initialization failed", "error", resourceErr.Error())
			os.Exit(1)
		}
		flightHubModelProjector = resourceSink
		resourceStreams, resourceErr := flighthub.NewResourceStreamCoordinator(
			flightHubClient, flightHubTokenResolver, resourceRepository, resourceSink,
			flighthub.ResourceStreamConfig{
				OnlineInterval: 15 * time.Second, OfflineInterval: 60 * time.Second, HealthInterval: 5 * time.Minute, CatalogInterval: 15 * time.Minute,
				MaxBackoff: 5 * time.Minute, LiveReconciler: liveReconciler,
				OnError: func(kind string, _ error) {
					logger.Warn("FlightHub resource stream degraded", "stream", kind)
				},
			},
		)
		if resourceErr != nil {
			logger.Error("FlightHub resource stream initialization failed", "error", resourceErr.Error())
			os.Exit(1)
		}
		inventoryRunner, resourceErr := flighthub.NewScheduledInventoryRunner(
			synchronizer, resourceRepository, workerConfig.FlightHubPollInterval, 5*time.Minute, nil,
		)
		if resourceErr != nil {
			logger.Error("FlightHub inventory schedule initialization failed", "error", resourceErr.Error())
			os.Exit(1)
		}
		resourceRunner, resourceErr := flighthub.NewConcurrentResourceRunner(inventoryRunner, resourceStreams)
		if resourceErr != nil {
			logger.Error("FlightHub concurrent resource runner initialization failed", "error", resourceErr.Error())
			os.Exit(1)
		}
		capabilityRunner, resourceErr := flighthub.NewCapabilityProbeRunner(
			resourceRunner, flightHubClient, flightHubTokenResolver,
			resourceRepository, 15*time.Minute, nil,
		)
		if resourceErr != nil {
			logger.Error("FlightHub capability probe initialization failed", "error", resourceErr.Error())
			os.Exit(1)
		}
		flightHubScheduler, syncErr = connector.NewScheduler(
			connector.NewSQLLeaseRepository(database), capabilityRunner, connector.NewSQLSyncOutcomeStore(database),
			connector.SchedulerConfig{
				Owner: workerConfig.WorkerName + ":" + runID, ConnectorKey: flighthub.ConnectorKey, Version: flighthub.ConnectorVersion,
				PollInterval:   min(workerConfig.FlightHubPollInterval, 15*time.Second),
				JitterWindow:   min(workerConfig.FlightHubPollInterval/10, 5*time.Second),
				ReconcileEvery: min(workerConfig.FlightHubReconcileEvery, 5*time.Second),
				LeaseDuration:  60 * time.Second, RenewEvery: 20 * time.Second, BatchSize: 8, Logger: logger,
				Metrics: observability.DefaultMetrics,
			},
		)
		if syncErr != nil {
			logger.Error("FlightHub scheduler initialization failed", "error", syncErr.Error())
			os.Exit(1)
		}
		consumer.Register("connector.sync.requested", flightHubScheduler.OutboxHandler)
		logger.Info("FlightHub connector enabled", "region", "cn", "poll_interval", workerConfig.FlightHubPollInterval.String())
	}
	driverRegistry := driver.NewRegistry()
	if err := dji.RegisterDriver(driverRegistry, func(context.Context, driver.AdapterConfig) error { return nil }); err != nil {
		logger.Error("DJI driver registration failed", "error", err.Error())
		os.Exit(1)
	}
	djiDeviceTypes := driver.NewDeviceTypeRegistry(driverRegistry)
	for _, register := range []func(*driver.DeviceTypeRegistry) error{
		dji.RegisterUnknownDJIDeviceType, dji.RegisterDock2DeviceTypes, dji.RegisterDock3DeviceTypes,
	} {
		if err := register(djiDeviceTypes); err != nil {
			logger.Error("DJI DeviceType registration failed", "error", err.Error())
			os.Exit(1)
		}
	}
	djiIngestor := dji.NewMessageIngestor(dji.NewSQLIngressStore(database))
	djiProjector := dji.NewProjector()
	consumer.Register("device.topology", djiProjector.Handler)
	consumer.Register("device.state", djiProjector.Handler)
	consumer.Register("device.telemetry", djiProjector.Handler)
	djiManager := dji.NewAdapterManager(
		dji.NewSQLLeaseRepository(database), dji.EncryptedCredentialResolver{AuthSecret: workerConfig.AuthSecret},
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
	djiCommandDispatcher, err := dji.NewCommandDispatcher(djiManager, nil, workerConfig.AuthSecret)
	if err != nil {
		logger.Error("DJI command dispatcher initialization failed", "error", err.Error())
		os.Exit(1)
	}
	deviceCommandHandler := outbox.Handler(djiCommandDispatcher.DispatchHandler)
	if flightHubClient != nil {
		flightHubCommandDispatcher, commandErr := flighthub.NewControlCommandDispatcher(flightHubClient, flightHubTokenResolver, nil)
		if commandErr != nil {
			logger.Error("FlightHub command dispatcher initialization failed", "error", commandErr.Error())
			os.Exit(1)
		}
		deviceCommandHandler = flighthub.RouteDeviceCommand(flightHubCommandDispatcher.DispatchHandler, deviceCommandHandler)
	}
	consumer.Register("device.command.dispatch", deviceCommandHandler)
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
	taskTriggerScheduler := tasktrigger.NewScheduler(database, nil, 30*time.Second, logger)
	consumer.Register("task_run.triggered", missionProcessor.Handler)
	consumer.Register("task_run.transitioned", func(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
		if err := missionProcessor.Handler(ctx, tx, event); err != nil {
			return err
		}
		return taskTriggerScheduler.UpstreamHandler(ctx, tx, event)
	})
	consumer.Register("mission.control", missionProcessor.Handler)
	consumer.Register("command.ack", missionProcessor.Handler)
	var rawStore algorithm.RawResultStore
	var assetStore algorithm.AlgorithmAssetStore
	var assetHandler outbox.Handler
	var waylineSource flighthub.WaylineSourceReader
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
		waylineSource = algorithmRawStore{storage: storage}
	}
	if flightHubClient != nil && waylineSource != nil {
		waylineUploadHandler, uploadErr := flighthub.NewWaylineUploadHandler(
			flighthub.NewSQLWaylineUploadStore(database), flightHubClient, flightHubTokenResolver,
			waylineSource, flighthub.NewMinIOWaylineObjectUploader(), workerConfig.AuthSecret,
		)
		if uploadErr != nil {
			logger.Error("FlightHub wayline upload initialization failed", "error", uploadErr.Error())
			os.Exit(1)
		}
		consumer.Register(flighthub.WaylineUploadEventType, waylineUploadHandler.Handler)
	} else if workerConfig.FlightHubEnabled {
		logger.Warn("FlightHub wayline upload unavailable", "reason", "OBJECT_STORAGE_LOCAL_ROOT is not configured")
	}
	if flightHubClient != nil {
		modelJobHandler, modelJobErr := flighthub.NewModelJobHandler(
			flighthub.NewSQLModelJobStore(database), flightHubClient, flightHubTokenResolver,
			flightHubModelProjector, workerConfig.AuthSecret, nil,
		)
		if modelJobErr != nil {
			logger.Error("FlightHub model job initialization failed", "error", modelJobErr.Error())
			os.Exit(1)
		}
		consumer.Register(flighthub.ModelJobEventType, modelJobHandler.Handler)
		modelDeleteHandler, deleteErr := flighthub.NewModelDeleteHandler(
			flighthub.NewSQLModelDeleteStore(database), flightHubClient, flightHubTokenResolver, workerConfig.AuthSecret,
		)
		if deleteErr != nil {
			logger.Error("FlightHub model delete initialization failed", "error", deleteErr.Error())
			os.Exit(1)
		}
		consumer.Register(flighthub.FlightHubModelDeleteEventType, modelDeleteHandler.Handler)
		openModelUploadHandler, uploadErr := flighthub.NewOpenModelUploadHandler(
			flighthub.NewSQLOpenModelUploadStore(database), flightHubClient, flightHubTokenResolver,
			flightHubModelProjector, workerConfig.AuthSecret, nil,
		)
		if uploadErr != nil {
			logger.Error("FlightHub open model upload initialization failed", "error", uploadErr.Error())
			os.Exit(1)
		}
		consumer.Register(flighthub.OpenModelUploadCredentialEventType, openModelUploadHandler.Handler)
		consumer.Register(flighthub.OpenModelUploadCallbackEventType, openModelUploadHandler.Handler)
		liveRegistry, liveErr := flighthub.NewDefaultLiveSupplierRegistry(flightHubClient)
		if liveErr != nil {
			logger.Error("FlightHub live supplier initialization failed", "error", liveErr.Error())
			os.Exit(1)
		}
		liveHandler, liveErr := flighthub.NewFlightHubLiveStartHandler(
			flighthub.NewSQLFlightHubLiveSessionStore(database), flightHubClient, liveRegistry,
			flightHubTokenResolver, workerConfig.AuthSecret, nil,
		)
		if liveErr != nil {
			logger.Error("FlightHub live start initialization failed", "error", liveErr.Error())
			os.Exit(1)
		}
		consumer.Register(flighthub.FlightHubLiveStartEventType, liveHandler.Handler)
		flightActionHandler, actionErr := flighthub.NewFlightActionHandler(
			flighthub.NewSQLFlightActionStore(database), flightHubClient, flightHubTokenResolver, workerConfig.AuthSecret,
		)
		if actionErr != nil {
			logger.Error("FlightHub flight action initialization failed", "error", actionErr.Error())
			os.Exit(1)
		}
		consumer.Register(flighthub.FlightActionEventType, flightActionHandler.Handler)
		liveActionHandler, actionErr := flighthub.NewLiveActionHandler(
			flighthub.NewSQLLiveActionStore(database), flightHubClient, flightHubTokenResolver, workerConfig.AuthSecret,
		)
		if actionErr != nil {
			logger.Error("FlightHub live action initialization failed", "error", actionErr.Error())
			os.Exit(1)
		}
		consumer.Register(flighthub.FlightHubLiveActionEventType, liveActionHandler.Handler)
		geospatialActionHandler, actionErr := flighthub.NewGeospatialActionHandler(
			flighthub.NewSQLGeospatialActionStore(database), flightHubClient, flightHubTokenResolver, workerConfig.AuthSecret,
		)
		if actionErr != nil {
			logger.Error("FlightHub geospatial action initialization failed", "error", actionErr.Error())
			os.Exit(1)
		}
		consumer.Register(flighthub.FlightHubGeospatialActionEventType, geospatialActionHandler.Handler)
	}
	assetSigner := algorithm.NewAssetURLSigner(workerConfig.AssetURLSigningSecret, workerConfig.CallbackPublicBaseURL)
	detectionSink := perception.NewSQLDetectionSink()
	consumer.Register("asset.available", func(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
		if err := assetHandler(ctx, tx, event); err != nil {
			return err
		}
		return mission.CompleteCollectionStep(ctx, tx, event)
	})
	algorithmTrigger := algorithm.NewTrigger(assetSigner)
	consumer.Register("task.step.algorithm.requested", mission.WithTaskStepFailurePolicy(algorithmTrigger.TaskStepHandler))
	consumer.Register("task.step.issue.requested", mission.WithTaskStepFailurePolicy(issueworker.NewTaskStepProcessor(nil).Handler))
	consumer.Register("task.step.copilot.requested", mission.WithTaskStepFailurePolicy(agent.TaskStepHandler))
	consumer.Register("task.step.report.requested", mission.WithTaskStepFailurePolicy(reportworker.NewProcessor(nil).Handler))
	algorithmProcessor := algorithm.NewProcessor(
		algorithm.DefaultHTTPClient(), algorithm.NewCircuitBreaker(3, 30*time.Second), rawStore,
		workerConfig.CallbackPublicBaseURL, assetSigner, detectionSink, workerConfig.AuthSecret,
	)
	consumer.Register("algorithm.run.requested", algorithmProcessor.Handler)
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
	runErrors := make(chan error, 12)
	go func() { runErrors <- consumer.RunWithWake(runContext, wake) }()
	go func() { runErrors <- taskTriggerScheduler.Run(runContext) }()
	if flightHubScheduler != nil {
		go func() { runErrors <- flightHubScheduler.Run(runContext) }()
	}
	go func() { runErrors <- heartbeat.NewProjector(database, nil).Run(runContext, 15*time.Second) }()
	go func() {
		runErrors <- (agent.JobProcessor{Database: database, AuthSecret: workerConfig.AuthSecret}).Run(runContext, 2*time.Second)
	}()
	go func() { runErrors <- djiManager.Run(runContext) }()
	go func() { runErrors <- djiCommandDispatcher.RunTimeoutReconciler(runContext, database, time.Second) }()
	if flightHubControlReconciler != nil {
		go func() { runErrors <- flightHubControlReconciler.Run(runContext, time.Second) }()
	}
	if flightHubCommandStatusReconciler != nil {
		go func() {
			runErrors <- flightHubCommandStatusReconciler.Run(runContext, 2*time.Second, func(_ error) {
				logger.Warn("FlightHub command status reconciliation degraded")
			})
		}()
	}
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
