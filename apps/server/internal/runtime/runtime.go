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
	"aerosight/server/internal/inspection"
	issueworker "aerosight/server/internal/issue"
	"aerosight/server/internal/media"
	"aerosight/server/internal/mission"
	"aerosight/server/internal/observability"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/perception"
	reportworker "aerosight/server/internal/report"
	"aerosight/server/internal/tasktrigger"
	"aerosight/server/internal/telemetry"
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

func (store algorithmRawStore) ReadWaylineSource(ctx context.Context, key string) (flighthub.WaylineSourceObject, error) {
	object, err := store.storage.GetObject(ctx, key)
	if err != nil {
		return flighthub.WaylineSourceObject{}, err
	}
	return flighthub.WaylineSourceObject{Body: object.Body, ContentType: object.ContentType}, nil
}

func New(database *sql.DB, workerConfig config.Config, logger *slog.Logger) (*Runtime, error) {
	runID := observability.CorrelationID("")
	consumer := outbox.NewConsumer(outbox.NewStore(database), runID, "aerosight-worker", logger)
	var flightHubScheduler *connector.Scheduler
	var flightHubClient *flighthub.Client
	var flightHubModelProjector flighthub.ModelJobProjector
	var flightHubControlReconciler *flighthub.SQLControlCommandReconciler
	var flightHubCommandStatusReconciler *flighthub.ControlCommandStatusReconciler
	var flightHubControlSessionHandler *flighthub.ControlSessionHandler
	flightHubTokenResolver := flighthub.EncryptedTokenResolver{AuthSecret: workerConfig.AuthSecret}
	{
		createdFlightHubClient, flightHubErr := flighthub.NewChinaClient(flighthub.Config{
			Timeout: workerConfig.FlightHubHTTPTimeout, MaxRetries: workerConfig.FlightHubMaxRetries,
			MaxProjectPages: 50, MaxResponseBytes: workerConfig.FlightHubMaxResponseBytes,
			RequestID:        func() string { return observability.CorrelationID("") },
			AllowedLinkHosts: workerConfig.FlightHubAllowedLinkHosts,
		})
		if flightHubErr != nil {
			logger.Error("FlightHub client initialization failed", "error", flightHubErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		flightHubClient = createdFlightHubClient
		connectorRegistry := connector.NewRegistry()
		if flightHubErr = flighthub.RegisterRuntime(
			connectorRegistry, flightHubClient, flightHubTokenResolver,
		); flightHubErr != nil {
			logger.Error("FlightHub runtime registration failed", "error", flightHubErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		synchronizer, syncErr := connector.NewSynchronizer(connectorRegistry, connector.NewSQLSyncStore(database))
		if syncErr != nil {
			logger.Error("connector synchronizer initialization failed", "error", syncErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		resourceRepository := connector.NewSQLResourceRepository(database)
		telemetryIngestor := telemetry.NewIngestor(database)
		flightHubControlReconciler, flightHubErr = flighthub.NewSQLControlCommandReconciler(database, nil)
		if flightHubErr != nil {
			logger.Error("FlightHub control reconciliation initialization failed", "error", flightHubErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		flightHubCommandStatusReconciler, flightHubErr = flighthub.NewControlCommandStatusReconciler(
			database, flightHubClient, flightHubTokenResolver, nil,
		)
		if flightHubErr != nil {
			logger.Error("FlightHub command status reconciliation initialization failed", "error", flightHubErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		flightHubControlSessionHandler, flightHubErr = flighthub.NewControlSessionHandler(
			flighthub.NewSQLControlSessionStore(database), flightHubClient, flightHubTokenResolver, nil,
		)
		if flightHubErr != nil {
			logger.Error("FlightHub control session initialization failed", "error", flightHubErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register(flighthub.FlightHubControlSessionEventType, flightHubControlSessionHandler.Handler)
		liveReconciler, liveErr := flighthub.NewFlightHubLiveReconciler(database, nil)
		if liveErr != nil {
			logger.Error("FlightHub live reconciliation initialization failed", "error", liveErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		resourceSink, resourceErr := flighthub.NewSQLResourceStreamSink(
			telemetryIngestor, resourceRepository, heartbeat.NewProjector(database, nil), flighthub.NewSQLDeviceHealthProjector(database),
			flighthub.NewSQLFlightCatalogProjector(database, telemetryIngestor, nil, 30*time.Minute, workerConfig.AuthSecret),
			flightHubControlReconciler,
		)
		if resourceErr != nil {
			logger.Error("FlightHub resource sink initialization failed", "error", resourceErr.Error())
			return nil, errors.New("background component initialization failed")
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
			return nil, errors.New("background component initialization failed")
		}
		inventoryRunner, resourceErr := flighthub.NewScheduledInventoryRunner(
			synchronizer, resourceRepository, workerConfig.FlightHubPollInterval, 5*time.Minute, nil,
		)
		if resourceErr != nil {
			logger.Error("FlightHub inventory schedule initialization failed", "error", resourceErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		resourceRunner, resourceErr := flighthub.NewConcurrentResourceRunner(inventoryRunner, resourceStreams)
		if resourceErr != nil {
			logger.Error("FlightHub concurrent resource runner initialization failed", "error", resourceErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		capabilityRunner, resourceErr := flighthub.NewCapabilityProbeRunner(
			resourceRunner, flightHubClient, flightHubTokenResolver,
			resourceRepository, 15*time.Minute, nil,
		)
		if resourceErr != nil {
			logger.Error("FlightHub capability probe initialization failed", "error", resourceErr.Error())
			return nil, errors.New("background component initialization failed")
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
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register("connector.sync.requested", flightHubScheduler.OutboxHandler)
		logger.Info("FlightHub connector runtime registered", "region", "cn", "poll_interval", workerConfig.FlightHubPollInterval.String())
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
	deviceCommandHandler := outbox.Handler(djiCommandDispatcher.DispatchHandler)
	if flightHubClient != nil {
		flightHubCommandDispatcher, commandErr := flighthub.NewControlCommandDispatcher(flightHubClient, flightHubTokenResolver, nil)
		if commandErr != nil {
			logger.Error("FlightHub command dispatcher initialization failed", "error", commandErr.Error())
			return nil, errors.New("background component initialization failed")
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
	taskTriggerScheduler := tasktrigger.NewScheduler(database, nil, 30*time.Second, logger)
	consumer.Register("task_run.triggered", missionProcessor.Handler)
	consumer.Register("task_run.transitioned", func(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
		if err := missionProcessor.Handler(ctx, tx, event); err != nil {
			return err
		}
		return taskTriggerScheduler.UpstreamHandler(ctx, tx, event)
	})
	consumer.Register("mission.control", func(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
		if err := missionProcessor.Handler(ctx, tx, event); err != nil {
			return err
		}
		return algorithm.ResumeInspectionChildren(ctx, tx, event)
	})
	consumer.Register("command.ack", missionProcessor.Handler)
	var rawStore algorithm.RawResultStore
	var assetStore algorithm.AlgorithmAssetStore
	var assetHandler outbox.Handler
	var waylineSource flighthub.WaylineSourceReader
	if workerConfig.ObjectStorageLocalRoot == "" {
		assetHandler = func(context.Context, *sql.Tx, outbox.Event) error {
			return errors.New("DATA_DIR is not configured")
		}
		logger.Warn("media derivative processing unavailable", "reason", "DATA_DIR is not configured")
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
		waylineSource = algorithmRawStore{storage: storage}
	}
	if flightHubClient != nil && waylineSource != nil {
		waylineUploadHandler, uploadErr := flighthub.NewWaylineUploadHandler(
			flighthub.NewSQLWaylineUploadStore(database), flightHubClient, flightHubTokenResolver,
			waylineSource, flighthub.NewMinIOWaylineObjectUploader(), workerConfig.AuthSecret,
		)
		if uploadErr != nil {
			logger.Error("FlightHub wayline upload initialization failed", "error", uploadErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register(flighthub.WaylineUploadEventType, waylineUploadHandler.Handler)
	} else {
		logger.Warn("FlightHub wayline upload unavailable", "reason", "DATA_DIR is not configured")
	}
	if flightHubClient != nil {
		modelJobHandler, modelJobErr := flighthub.NewModelJobHandler(
			flighthub.NewSQLModelJobStore(database), flightHubClient, flightHubTokenResolver,
			flightHubModelProjector, workerConfig.AuthSecret, nil,
		)
		if modelJobErr != nil {
			logger.Error("FlightHub model job initialization failed", "error", modelJobErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register(flighthub.ModelJobEventType, modelJobHandler.Handler)
		modelDeleteHandler, deleteErr := flighthub.NewModelDeleteHandler(
			flighthub.NewSQLModelDeleteStore(database), flightHubClient, flightHubTokenResolver, workerConfig.AuthSecret,
		)
		if deleteErr != nil {
			logger.Error("FlightHub model delete initialization failed", "error", deleteErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register(flighthub.FlightHubModelDeleteEventType, modelDeleteHandler.Handler)
		openModelUploadHandler, uploadErr := flighthub.NewOpenModelUploadHandler(
			flighthub.NewSQLOpenModelUploadStore(database), flightHubClient, flightHubTokenResolver,
			flightHubModelProjector, workerConfig.AuthSecret, nil,
		)
		if uploadErr != nil {
			logger.Error("FlightHub open model upload initialization failed", "error", uploadErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register(flighthub.OpenModelUploadCredentialEventType, openModelUploadHandler.Handler)
		consumer.Register(flighthub.OpenModelUploadCallbackEventType, openModelUploadHandler.Handler)
		liveRegistry, liveErr := flighthub.NewDefaultLiveSupplierRegistry(flightHubClient)
		if liveErr != nil {
			logger.Error("FlightHub live supplier initialization failed", "error", liveErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		liveHandler, liveErr := flighthub.NewFlightHubLiveStartHandler(
			flighthub.NewSQLFlightHubLiveSessionStore(database), flightHubClient, liveRegistry,
			flightHubTokenResolver, workerConfig.AuthSecret, nil,
		)
		if liveErr != nil {
			logger.Error("FlightHub live start initialization failed", "error", liveErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register(flighthub.FlightHubLiveStartEventType, liveHandler.Handler)
		flightActionHandler, actionErr := flighthub.NewFlightActionHandler(
			flighthub.NewSQLFlightActionStore(database), flightHubClient, flightHubTokenResolver, workerConfig.AuthSecret,
		)
		if actionErr != nil {
			logger.Error("FlightHub flight action initialization failed", "error", actionErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register(flighthub.FlightActionEventType, flightActionHandler.Handler)
		liveActionHandler, actionErr := flighthub.NewLiveActionHandler(
			flighthub.NewSQLLiveActionStore(database), flightHubClient, flightHubTokenResolver, workerConfig.AuthSecret,
		)
		if actionErr != nil {
			logger.Error("FlightHub live action initialization failed", "error", actionErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register(flighthub.FlightHubLiveActionEventType, liveActionHandler.Handler)
		deviceAdminHandler, actionErr := flighthub.NewDeviceAdminActionHandler(database, flightHubClient, flightHubTokenResolver, workerConfig.AuthSecret)
		if actionErr != nil {
			logger.Error("FlightHub device admin action initialization failed", "error", actionErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register(flighthub.FlightHubDeviceAdminEventType, deviceAdminHandler.Handler)
		managementWriteHandler, actionErr := flighthub.NewManagementWriteHandler(database, flightHubClient, flightHubTokenResolver, workerConfig.AuthSecret)
		if actionErr != nil {
			logger.Error("FlightHub management write initialization failed", "error", actionErr.Error())
			return nil, errors.New("background component initialization failed")
		}
		consumer.Register(flighthub.FlightHubManagementWriteEventType, managementWriteHandler.Handler)
		geospatialActionHandler, actionErr := flighthub.NewGeospatialActionHandler(
			flighthub.NewSQLGeospatialActionStore(database), flightHubClient, flightHubTokenResolver, workerConfig.AuthSecret,
		)
		if actionErr != nil {
			logger.Error("FlightHub geospatial action initialization failed", "error", actionErr.Error())
			return nil, errors.New("background component initialization failed")
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
	var flightObserve inspection.FlightObservationReader
	var remoteAsset inspection.RemoteAssetReader
	var algorithmRemoteAsset algorithm.RemoteAlgorithmAssetReader
	if workerConfig.AuthSecret != "" {
		access, err := flighthub.NewFlightAssetAccessService(database, flightHubClient, flightHubTokenResolver, workerConfig.AuthSecret, nil)
		if err != nil {
			return nil, err
		}
		observer := flighthub.NewInspectionFlightObserver(flightHubClient, access, flightHubTokenResolver)
		flightObserve = observer.Observe
		remoteAsset = observer.ReadAsset
		algorithmRemoteAsset = access.ReadAlgorithmAsset
	}
	observe := inspection.NewObserveProcessor(func(ctx context.Context, key string) ([]byte, error) {
		if assetStore == nil {
			return nil, errors.New("ASSET_STORAGE_UNAVAILABLE")
		}
		asset, err := assetStore.ReadAlgorithmAsset(ctx, key)
		return asset.Body, err
	}, flightObserve).WithRemoteAssetReader(remoteAsset)
	consumer.Register("task.step.inspection.observe.requested", mission.WithTaskStepFailurePolicy(observe.Handler))
	algorithmTrigger := algorithm.NewTrigger(assetSigner)
	detect := inspection.NewDetectProcessor(algorithmTrigger)
	consumer.Register("task.step.inspection.detect.requested", mission.WithTaskStepFailurePolicy(detect.Handler))
	consumer.Register("inspection.algorithm.completed", mission.WithTaskStepFailurePolicy(detect.Handler))
	consumer.Register("task.step.algorithm.requested", mission.WithTaskStepFailurePolicy(algorithmTrigger.TaskStepHandler))
	consumer.Register("task.step.issue.requested", mission.WithTaskStepFailurePolicy(issueworker.NewTaskStepProcessor(nil).Handler))
	consumer.Register("task.step.copilot.requested", mission.WithTaskStepFailurePolicy(agent.TaskStepHandler))
	consumer.Register("task.step.report.requested", mission.WithTaskStepFailurePolicy(reportworker.NewProcessor(nil).Handler))
	algorithmClient, err := algorithm.HTTPClientWithCA(workerConfig.AlgorithmCAFile)
	if err != nil {
		return nil, err
	}
	algorithmProcessor := algorithm.NewProcessor(
		algorithmClient, algorithm.NewCircuitBreaker(3, 30*time.Second), rawStore,
		workerConfig.CallbackPublicBaseURL, assetSigner, detectionSink, workerConfig.AuthSecret,
	)
	algorithmProcessor.WithInspectionWorker(database)
	consumer.Register("algorithm.run.requested", algorithmProcessor.Handler)

	callbacks := http.NewServeMux()
	callbacks.Handle("/callbacks/algorithms/", algorithm.NewCallbackHandler(database, rawStore, detectionSink))
	callbacks.Handle("/algorithm-assets/", algorithm.NewAssetAccessHandler(database, assetStore, assetSigner).WithRemoteReader(algorithmRemoteAsset))
	tasks := []func(context.Context) error{
		func(ctx context.Context) error { return algorithmProcessor.RunInspection(ctx, time.Second) },
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
	tasks = append(tasks, taskTriggerScheduler.Run)
	if flightHubControlReconciler != nil {
		tasks = append(tasks, func(ctx context.Context) error { return flightHubControlReconciler.Run(ctx, time.Second) })
	}
	if flightHubCommandStatusReconciler != nil {
		tasks = append(tasks, func(ctx context.Context) error {
			return flightHubCommandStatusReconciler.Run(ctx, 2*time.Second, func(_ error) { logger.Warn("FlightHub command status reconciliation degraded") })
		})
	}
	if flightHubControlSessionHandler != nil {
		tasks = append(tasks, func(ctx context.Context) error {
			return flightHubControlSessionHandler.Run(ctx, time.Second, func(_ error) { logger.Warn("FlightHub control session reconciliation degraded") })
		})
	}
	if liveStreamHealth != nil {
		tasks = append(tasks, func(ctx context.Context) error { return liveStreamHealth.Run(ctx, database, 2*time.Second) })
	}
	return &Runtime{Callbacks: callbacks, InspectionMedia: algorithmRemoteAsset, tasks: tasks}, nil
}

type Runtime struct {
	InspectionMedia algorithm.RemoteAlgorithmAssetReader
	Callbacks       http.Handler
	tasks           []func(context.Context) error
}

// Run cancels peer components on failure and waits for all of them to release resources.
func (r *Runtime) Run(ctx context.Context) error {
	return r.RunWithFailure(ctx, nil)
}

// RunWithFailure reports a component failure before waiting for peers to drain.
// onFailure must not block; it can revoke readiness and cancel the application.
func (r *Runtime) RunWithFailure(ctx context.Context, onFailure func(error)) error {
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
			if onFailure != nil {
				onFailure(err)
			}
			cancel()
		} else if first == nil && err != nil && !errors.Is(err, ctx.Err()) {
			// Preserve cleanup failures after cancellation; cancellation itself
			// is normal, but a failed drain must not become a successful exit.
			first = err
		}
	}
	return first
}
