package runtime

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"aerosight/server/internal/agent"
	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/config"
	"aerosight/server/internal/connector"
	"aerosight/server/internal/dji"
	"aerosight/server/internal/driver"
	"aerosight/server/internal/flighthub"
	"aerosight/server/internal/heartbeat"
	issueworker "aerosight/server/internal/issue"
	"aerosight/server/internal/media"
	"aerosight/server/internal/mission"
	"aerosight/server/internal/observability"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/perception"
	"aerosight/server/internal/wakeup"
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

func New(database *sql.DB, workerConfig config.Config, logger *slog.Logger) (*Runtime, error) {
	runID := observability.CorrelationID("")
	consumer := outbox.NewConsumer(outbox.NewStore(database), runID, "aerosight-worker", logger)
	var flightHubScheduler *connector.Scheduler
	if workerConfig.FlightHubEnabled {
		flightHubClient, flightHubErr := flighthub.NewChinaClient(flighthub.Config{
			Timeout: workerConfig.FlightHubHTTPTimeout, MaxRetries: workerConfig.FlightHubMaxRetries,
			MaxProjectPages: 50, MaxResponseBytes: workerConfig.FlightHubMaxResponseBytes,
			RequestID: func() string { return observability.CorrelationID("") },
		})
		if flightHubErr != nil {
			logger.Error("FlightHub client initialization failed", "error", flightHubErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		connectorRegistry := connector.NewRegistry()
		if flightHubErr = flighthub.RegisterRuntime(
			connectorRegistry, flightHubClient, flighthub.EncryptedTokenResolver{AuthSecret: workerConfig.AuthSecret},
		); flightHubErr != nil {
			logger.Error("FlightHub runtime registration failed", "error", flightHubErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		synchronizer, syncErr := connector.NewSynchronizer(connectorRegistry, connector.NewSQLSyncStore(database))
		if syncErr != nil {
			logger.Error("connector synchronizer initialization failed", "error", syncErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		flightHubScheduler, syncErr = connector.NewScheduler(
			connector.NewSQLLeaseRepository(database), synchronizer, connector.NewSQLSyncOutcomeStore(database),
			connector.SchedulerConfig{
				Owner: workerConfig.WorkerName + ":" + runID, ConnectorKey: flighthub.ConnectorKey, Version: flighthub.ConnectorVersion,
				PollInterval:   workerConfig.FlightHubPollInterval,
				JitterWindow:   min(workerConfig.FlightHubPollInterval/10, 30*time.Second),
				ReconcileEvery: workerConfig.FlightHubReconcileEvery,
				LeaseDuration:  60 * time.Second, RenewEvery: 20 * time.Second, BatchSize: 8, Logger: logger,
				Metrics: observability.DefaultMetrics,
			},
		)
		if syncErr != nil {
			logger.Error("FlightHub scheduler initialization failed", "error", syncErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register("connector.sync.requested", flightHubScheduler.OutboxHandler)
		logger.Info("FlightHub connector enabled", "region", "cn", "poll_interval", workerConfig.FlightHubPollInterval.String())
	}
	driverRegistry := driver.NewRegistry()
	if err := dji.RegisterDriver(driverRegistry, func(context.Context, driver.AdapterConfig) error { return nil }); err != nil {
		logger.Error("DJI driver registration failed", "error", err.Error())
		return nil, errors.New("background component initialization failed")
	}
	djiDeviceTypes := driver.NewDeviceTypeRegistry(driverRegistry)
	for _, register := range []func(*driver.DeviceTypeRegistry) error{
		dji.RegisterUnknownDJIDeviceType, dji.RegisterDock2DeviceTypes, dji.RegisterDock3DeviceTypes,
	} {
		if err := register(djiDeviceTypes); err != nil {
			logger.Error("DJI DeviceType registration failed", "error", err.Error())
			return nil, errors.New("background component initialization failed")
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
		return nil, errors.New("background component initialization failed")
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
			return nil, errors.New("background component initialization failed")
		}
		liveStreamHealth, mediaErr = dji.NewLiveStreamHealthCoordinator(
			mediaInspector, workerConfig.WorkerName+":"+runID, nil,
		)
		if mediaErr != nil {
			logger.Error("live stream health coordinator initialization failed", "error", mediaErr.Error())
			return nil, errors.New("background component initialization failed")
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
			return nil, errors.New("background component initialization failed")
		}
		processor := media.NewProcessor(storage, media.NewSQLRepository())
		assetHandler = processor.Handler
		rawStore = algorithmRawStore{storage: storage}
		assetStore = algorithmRawStore{storage: storage}
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
	consumer.Register("task.step.algorithm.requested", algorithmTrigger.TaskStepHandler)
	consumer.Register("task.step.issue.requested", issueworker.NewTaskStepProcessor(nil).Handler)
	consumer.Register("task.step.copilot.requested", agent.TaskStepHandler)
	algorithmProcessor := algorithm.NewProcessor(
		algorithm.DefaultHTTPClient(), algorithm.NewCircuitBreaker(3, 30*time.Second), rawStore,
		workerConfig.CallbackPublicBaseURL, assetSigner, detectionSink, workerConfig.AuthSecret,
	)
	consumer.Register("algorithm.run.requested", algorithmProcessor.Handler)

	callbacks := http.NewServeMux()
	callbacks.Handle("/callbacks/algorithms/", algorithm.NewCallbackHandler(database, rawStore, detectionSink))
	callbacks.Handle("/algorithm-assets/", algorithm.NewAssetAccessHandler(database, assetStore, assetSigner))
	tasks := []func(context.Context) error{
		func(ctx context.Context) error {
			return consumer.RunWithWake(ctx, wakeup.Postgres(ctx, workerConfig.DatabaseURL, logger))
		},
		func(ctx context.Context) error { return heartbeat.NewProjector(database, nil).Run(ctx, 15*time.Second) },
		func(ctx context.Context) error {
			return (agent.JobProcessor{Database: database, AuthSecret: workerConfig.AuthSecret}).Run(ctx, 2*time.Second)
		},
		djiManager.Run,
		func(ctx context.Context) error {
			return djiCommandDispatcher.RunTimeoutReconciler(ctx, database, time.Second)
		},
	}
	if flightHubScheduler != nil {
		tasks = append(tasks, flightHubScheduler.Run)
	}
	if liveStreamHealth != nil {
		tasks = append(tasks, func(ctx context.Context) error { return liveStreamHealth.Run(ctx, database, 2*time.Second) })
	}
	return &Runtime{Callbacks: callbacks, tasks: tasks}, nil
}

type Runtime struct {
	Callbacks http.Handler
	tasks     []func(context.Context) error
}

// Run cancels peer components on failure and waits for all of them to release resources.
func (r *Runtime) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, len(r.tasks))
	for _, task := range r.tasks {
		go func(run func(context.Context) error) { results <- run(ctx) }(task)
	}
	var first error
	for range r.tasks {
		err := <-results
		if ctx.Err() == nil {
			if err == nil {
				err = errors.New("background component stopped unexpectedly")
			}
			first = err
			cancel()
		}
	}
	return first
}
