package flighthub

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"aerosight/worker/internal/connector"
)

type ResourceStreamClient interface {
	GetDeviceState(context.Context, string, string, string) (DeviceStateSnapshot, error)
	ListDeviceHMS(context.Context, string, string, []string) ([]DeviceHMS, error)
	ListHistoricalTopologies(context.Context, string, string) ([]HistoricalTopology, error)
	GetAutoRecordingConfig(context.Context, string, string) (AutoRecordingConfig, error)
	ListWaylines(context.Context, string, string) ([]WaylineSummary, error)
	ListFlightTasks(context.Context, string, string, FlightTaskListOptions) ([]FlightTaskSummary, error)
	GetFlightTaskTrack(context.Context, string, string, string) (FlightTaskTrack, error)
	GetFlightTaskOperationTimeline(context.Context, string, string, string) (FlightTaskOperationTimeline, error)
	ListFlightTaskMedia(context.Context, string, string, string) ([]FlightTaskMedia, error)
	ListFlightTaskExports(context.Context, string, string, FlightExportOptions) (FlightExportPage, error)
	ListFlightAlerts(context.Context, string, string, FlightAlertOptions) (FlightAlertPage, error)
	ListAIAlertRecords(context.Context, string, string, AIAlertOptions) (AIAlertPage, error)
	ListOrganizationRecordingTasks(context.Context, string, string, string) ([]RecordingTask, error)
	ListProjectRecordingTasks(context.Context, string, string, string) ([]RecordingTask, error)
	ListLiveShares(context.Context, string, string, LiveShareListOptions) ([]LiveShare, error)
	ListStreamConverters(context.Context, string, string, StreamConverterListOptions) (PageResult[StreamConverter], error)
}

type ResourceStreamStore interface {
	LoadResourceSyncState(context.Context, connector.Instance, string) (connector.ResourceSyncUpdate, bool, error)
	SaveResourceSyncState(context.Context, connector.Instance, connector.ResourceSyncUpdate) error
	ListManagedDevices(context.Context, connector.Instance) ([]connector.ManagedConnectorDevice, error)
}

type DeviceStatePoll struct {
	Device            connector.ManagedConnectorDevice
	Snapshot          DeviceStateSnapshot
	Mapped            MappedDeviceState
	ReceivedAt        time.Time
	FreshnessInterval time.Duration
}

type HealthPoll struct {
	Devices    []connector.ManagedConnectorDevice
	HMS        []DeviceHMS
	Topologies []HistoricalTopology
	AutoRecord AutoRecordingConfig
	ReceivedAt time.Time
}

type CatalogPoll struct {
	Kind             string
	Waylines         []WaylineSummary
	FlightTasks      []FlightTaskSummary
	CompleteSnapshot bool
	ReceivedAt       time.Time
}

type FlightArtifactTarget struct {
	RemoteTaskID     string
	TaskRunID        int
	RemoteVersion    string
	NeedTrack        bool
	NeedOperation    bool
	NeedMedia        bool
	MediaUploadFinal bool
}

type FlightArtifactPoll struct {
	Target     FlightArtifactTarget
	Track      *FlightTaskTrack
	Operations *FlightTaskOperationTimeline
	Media      *[]FlightTaskMedia
	ReceivedAt time.Time
}

type FlightExportPoll struct {
	Records          []FlightExportRecord
	CompleteSnapshot bool
	ReceivedAt       time.Time
}

type FlightAlertPoll struct {
	Aggregates       []FlightAlertSummary
	Alerts           []AIAlertRecord
	CompleteSnapshot bool
	ReceivedAt       time.Time
}

type ScopedRecordingTask struct {
	Scope  string
	Device connector.ManagedConnectorDevice
	Task   RecordingTask
}

type LiveCatalogPoll struct {
	Devices           []connector.ManagedConnectorDevice
	Recordings        []ScopedRecordingTask
	Shares            []LiveShare
	Converters        []StreamConverter
	RecordingComplete bool
	ShareComplete     bool
	ConverterComplete bool
	ReceivedAt        time.Time
}

type ResourceStreamSink interface {
	ApplyDeviceState(context.Context, connector.Instance, DeviceStatePoll) error
	ApplyHealth(context.Context, connector.Instance, HealthPoll) error
	ApplyCatalog(context.Context, connector.Instance, CatalogPoll) error
	ListFlightArtifactTargets(context.Context, connector.Instance, int) ([]FlightArtifactTarget, error)
	ApplyFlightArtifacts(context.Context, connector.Instance, FlightArtifactPoll) error
	ApplyFlightExports(context.Context, connector.Instance, FlightExportPoll) error
	ApplyFlightAlerts(context.Context, connector.Instance, FlightAlertPoll) error
	ApplyLiveCatalog(context.Context, connector.Instance, LiveCatalogPoll) error
}

type ActiveLiveSessionReconciler interface {
	ReconcileLiveSessions(context.Context, connector.Instance) (FlightHubLiveReconcileSummary, error)
}

type ResourceStreamConfig struct {
	OnlineInterval    time.Duration
	OfflineInterval   time.Duration
	HealthInterval    time.Duration
	CatalogInterval   time.Duration
	ArtifactBatchSize int
	MaxBackoff        time.Duration
	LiveReconciler    ActiveLiveSessionReconciler
	Now               func() time.Time
	OnError           func(string, error)
}

type ResourceStreamCoordinator struct {
	client   ResourceStreamClient
	resolver TokenResolver
	store    ResourceStreamStore
	sink     ResourceStreamSink
	config   ResourceStreamConfig
	mu       sync.Mutex
	nextPoll map[string]time.Time
}

func NewResourceStreamCoordinator(client ResourceStreamClient, resolver TokenResolver, store ResourceStreamStore, sink ResourceStreamSink, config ResourceStreamConfig) (*ResourceStreamCoordinator, error) {
	if config.CatalogInterval == 0 {
		config.CatalogInterval = 15 * time.Minute
	}
	if config.ArtifactBatchSize == 0 {
		config.ArtifactBatchSize = 25
	}
	if client == nil || resolver == nil || store == nil || sink == nil || config.OnlineInterval <= 0 || config.OfflineInterval < config.OnlineInterval || config.HealthInterval <= 0 || config.CatalogInterval <= 0 || config.ArtifactBatchSize < 1 || config.ArtifactBatchSize > 100 || config.MaxBackoff <= 0 {
		return nil, errors.New("FlightHub resource stream configuration is invalid")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.OnError == nil {
		config.OnError = func(string, error) {}
	}
	return &ResourceStreamCoordinator{client: client, resolver: resolver, store: store, sink: sink, config: config, nextPoll: map[string]time.Time{}}, nil
}

func (coordinator *ResourceStreamCoordinator) Run(ctx context.Context, instance connector.Instance) {
	var wait sync.WaitGroup
	for _, stream := range []struct {
		kind string
		run  func(context.Context, connector.Instance, string) (map[string]any, time.Duration, error)
	}{
		{kind: "device-state", run: coordinator.pollDeviceStates},
		{kind: "health", run: coordinator.pollHealth},
		{kind: "waylines", run: coordinator.pollWaylines},
		{kind: "flight-tasks", run: coordinator.pollFlightTasks},
		{kind: "flight-artifacts", run: coordinator.pollFlightArtifacts},
		{kind: "active-operations", run: coordinator.pollFlightAlerts},
		{kind: "live", run: coordinator.pollLiveCatalog},
	} {
		stream := stream
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := coordinator.runStream(ctx, instance, stream.kind, stream.run); err != nil {
				coordinator.config.OnError(stream.kind, err)
			}
		}()
	}
	wait.Wait()
}

func (coordinator *ResourceStreamCoordinator) pollLiveCatalog(ctx context.Context, instance connector.Instance, token string) (map[string]any, time.Duration, error) {
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return nil, 0, err
	}
	devices, err := coordinator.store.ListManagedDevices(ctx, instance)
	if err != nil {
		return nil, 0, err
	}
	recordings := make([]ScopedRecordingTask, 0)
	recordingComplete := true
	var pollErrors []error
	for _, device := range devices {
		items, listErr := coordinator.client.ListProjectRecordingTasks(ctx, token, scope.ProjectUUID, device.Serial)
		if listErr != nil {
			recordingComplete = false
			pollErrors = append(pollErrors, listErr)
		} else {
			for _, item := range items {
				recordings = append(recordings, ScopedRecordingTask{Scope: "project", Device: device, Task: item})
			}
		}
		if scope.OrganizationUUID == "" {
			continue
		}
		items, listErr = coordinator.client.ListOrganizationRecordingTasks(ctx, token, scope.OrganizationUUID, device.Serial)
		if listErr != nil {
			recordingComplete = false
			pollErrors = append(pollErrors, listErr)
		} else {
			for _, item := range items {
				recordings = append(recordings, ScopedRecordingTask{Scope: "organization", Device: device, Task: item})
			}
		}
	}
	shares, shareComplete, shareErr := coordinator.listAllLiveShares(ctx, token, scope.ProjectUUID)
	if shareErr != nil {
		pollErrors = append(pollErrors, shareErr)
	}
	converters, converterComplete, converterErr := coordinator.listAllStreamConverters(ctx, token, scope.ProjectUUID)
	if converterErr != nil {
		pollErrors = append(pollErrors, converterErr)
	}
	poll := LiveCatalogPoll{
		Devices: devices, Recordings: recordings, Shares: shares, Converters: converters,
		RecordingComplete: recordingComplete, ShareComplete: shareComplete, ConverterComplete: converterComplete,
		ReceivedAt: coordinator.config.Now().UTC(),
	}
	if applyErr := coordinator.sink.ApplyLiveCatalog(ctx, instance, poll); applyErr != nil {
		pollErrors = append(pollErrors, applyErr)
	}
	cursor := map[string]any{
		"recordings": len(recordings), "recordingComplete": recordingComplete,
		"shares": len(shares), "shareComplete": shareComplete,
		"converters": len(converters), "converterComplete": converterComplete,
		"complete": recordingComplete && shareComplete && converterComplete,
	}
	return cursor, coordinator.config.CatalogInterval, errors.Join(pollErrors...)
}

func (coordinator *ResourceStreamCoordinator) listAllLiveShares(ctx context.Context, token, projectUUID string) ([]LiveShare, bool, error) {
	const pageSize, maxPages = 100, 100
	items := make([]LiveShare, 0)
	seen := make(map[string]struct{})
	for page := 1; page <= maxPages; page++ {
		result, err := coordinator.client.ListLiveShares(ctx, token, projectUUID, LiveShareListOptions{
			PageOptions: PageOptions{Page: page, PageSize: pageSize}, Status: 0,
		})
		if err != nil {
			return items, false, err
		}
		for _, item := range result {
			identity, identityErr := opaqueResourceIdentity(item)
			if identityErr != nil {
				return items, false, identityErr
			}
			if _, duplicate := seen[identity]; duplicate {
				return items, false, schemaError()
			}
			seen[identity] = struct{}{}
			items = append(items, item)
		}
		if len(result) < pageSize {
			return items, true, nil
		}
	}
	return items, false, &APIError{SafeCode: "snapshot_incomplete", Retryable: true}
}

func (coordinator *ResourceStreamCoordinator) listAllStreamConverters(ctx context.Context, token, projectUUID string) ([]StreamConverter, bool, error) {
	const pageSize, maxPages = 100, 100
	items := make([]StreamConverter, 0)
	seen := make(map[string]struct{})
	total := -1
	for page := 1; page <= maxPages; page++ {
		result, err := coordinator.client.ListStreamConverters(ctx, token, projectUUID, StreamConverterListOptions{
			PageOptions: PageOptions{Page: page, PageSize: pageSize},
		})
		if err != nil {
			return items, false, err
		}
		if result.Pagination.Page != page || result.Pagination.PageSize != pageSize {
			return items, false, schemaError()
		}
		if total < 0 {
			total = result.Pagination.Total
		} else if total != result.Pagination.Total {
			return items, false, schemaError()
		}
		for _, item := range result.List {
			if _, duplicate := seen[item.ID]; duplicate {
				return items, false, schemaError()
			}
			seen[item.ID] = struct{}{}
			items = append(items, item)
		}
		if len(items) == total {
			return items, true, nil
		}
		if len(items) > total || len(result.List) == 0 || len(result.List) < pageSize {
			return items, false, schemaError()
		}
	}
	return items, false, &APIError{SafeCode: "snapshot_incomplete", Retryable: true}
}

func (coordinator *ResourceStreamCoordinator) pollFlightAlerts(ctx context.Context, instance connector.Instance, token string) (map[string]any, time.Duration, error) {
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return nil, 0, err
	}
	devices, err := coordinator.store.ListManagedDevices(ctx, instance)
	if err != nil {
		return nil, 0, err
	}
	drones := make([]connector.ManagedConnectorDevice, 0, len(devices))
	for _, device := range devices {
		if device.Class == "drone" || device.Class == "aircraft" || device.Class == "uav" {
			drones = append(drones, device)
		}
	}
	if len(drones) == 0 {
		return map[string]any{"drones": 0, "flights": 0, "alerts": 0, "complete": false}, coordinator.config.CatalogInterval, nil
	}
	aggregates := make(map[string]FlightAlertSummary)
	for _, drone := range drones {
		items, listErr := coordinator.listAllFlightAlerts(ctx, token, scope.ProjectUUID, drone.Serial)
		if listErr != nil {
			return map[string]any{"drones": len(drones), "flights": len(aggregates), "alerts": 0, "complete": false}, 0, listErr
		}
		for _, item := range items {
			if previous, duplicate := aggregates[item.FlightID]; duplicate {
				if previous.Count != item.Count || previous.TaskName != item.TaskName || previous.StartTime != item.StartTime {
					return map[string]any{"drones": len(drones), "flights": len(aggregates), "alerts": 0, "complete": false}, 0, schemaError()
				}
				continue
			}
			aggregates[item.FlightID] = item
		}
	}
	flightIDs := make([]string, 0, len(aggregates))
	for flightID := range aggregates {
		flightIDs = append(flightIDs, flightID)
	}
	sort.Strings(flightIDs)
	alerts := make([]AIAlertRecord, 0)
	for start := 0; start < len(flightIDs); start += maxFlightAlertBatch {
		end := start + maxFlightAlertBatch
		if end > len(flightIDs) {
			end = len(flightIDs)
		}
		items, listErr := coordinator.listAllAIAlerts(ctx, token, scope.ProjectUUID, flightIDs[start:end])
		if listErr != nil {
			return map[string]any{"drones": len(drones), "flights": len(aggregates), "alerts": len(alerts), "complete": false}, 0, listErr
		}
		alerts = append(alerts, items...)
	}
	orderedAggregates := make([]FlightAlertSummary, 0, len(flightIDs))
	for _, flightID := range flightIDs {
		orderedAggregates = append(orderedAggregates, aggregates[flightID])
	}
	poll := FlightAlertPoll{Aggregates: orderedAggregates, Alerts: alerts, CompleteSnapshot: true, ReceivedAt: coordinator.config.Now().UTC()}
	if err := coordinator.sink.ApplyFlightAlerts(ctx, instance, poll); err != nil {
		return map[string]any{"drones": len(drones), "flights": len(aggregates), "alerts": len(alerts), "complete": false}, 0, err
	}
	return map[string]any{"drones": len(drones), "flights": len(aggregates), "alerts": len(alerts), "complete": true}, coordinator.config.CatalogInterval, nil
}

func (coordinator *ResourceStreamCoordinator) listAllFlightAlerts(ctx context.Context, token, projectUUID, droneSN string) ([]FlightAlertSummary, error) {
	const pageSize, maxPages = 100, 100
	items := make([]FlightAlertSummary, 0)
	seen := make(map[string]struct{})
	total := -1
	for page := 1; page <= maxPages; page++ {
		result, err := coordinator.client.ListFlightAlerts(ctx, token, projectUUID, FlightAlertOptions{DroneSN: droneSN, Page: page, PageSize: pageSize})
		if err != nil {
			return nil, err
		}
		if total < 0 {
			total = result.Total
		} else if total != result.Total {
			return nil, schemaError()
		}
		for _, item := range result.Data {
			if _, duplicate := seen[item.FlightID]; duplicate {
				return nil, schemaError()
			}
			seen[item.FlightID] = struct{}{}
			items = append(items, item)
		}
		if len(items) == total {
			return items, nil
		}
		if len(items) > total || len(result.Data) == 0 || len(result.Data) < pageSize {
			return nil, schemaError()
		}
	}
	return nil, &APIError{SafeCode: "snapshot_incomplete", Retryable: true}
}

func (coordinator *ResourceStreamCoordinator) listAllAIAlerts(ctx context.Context, token, projectUUID string, flightIDs []string) ([]AIAlertRecord, error) {
	const pageSize, maxPages = 100, 100
	items := make([]AIAlertRecord, 0)
	seen := make(map[string]struct{})
	total := -1
	for page := 1; page <= maxPages; page++ {
		result, err := coordinator.client.ListAIAlertRecords(ctx, token, projectUUID, AIAlertOptions{FlightIDs: flightIDs, Page: page, PageSize: pageSize})
		if err != nil {
			return nil, err
		}
		if total < 0 {
			total = result.Total
		} else if total != result.Total {
			return nil, schemaError()
		}
		keys := make([]string, 0, len(result.Data))
		for flightID := range result.Data {
			keys = append(keys, flightID)
		}
		sort.Strings(keys)
		pageItems := 0
		for _, flightID := range keys {
			for _, item := range result.Data[flightID] {
				if _, duplicate := seen[item.AlertUUID]; duplicate {
					return nil, schemaError()
				}
				seen[item.AlertUUID] = struct{}{}
				items = append(items, item)
				pageItems++
			}
		}
		if len(items) == total {
			return items, nil
		}
		if len(items) > total || pageItems == 0 || pageItems < pageSize {
			return nil, schemaError()
		}
	}
	return nil, &APIError{SafeCode: "snapshot_incomplete", Retryable: true}
}

func (coordinator *ResourceStreamCoordinator) pollFlightArtifacts(ctx context.Context, instance connector.Instance, token string) (map[string]any, time.Duration, error) {
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return nil, 0, err
	}
	targets, err := coordinator.sink.ListFlightArtifactTargets(ctx, instance, coordinator.config.ArtifactBatchSize)
	if err != nil {
		return nil, 0, err
	}
	now := coordinator.config.Now().UTC()
	tracks, operations, mediaTasks, exports := 0, 0, 0, 0
	var pollErrors []error
	for _, target := range targets {
		poll := FlightArtifactPoll{Target: target, ReceivedAt: now}
		if target.NeedTrack {
			track, trackErr := coordinator.client.GetFlightTaskTrack(ctx, token, scope.ProjectUUID, target.RemoteTaskID)
			if trackErr != nil {
				pollErrors = append(pollErrors, trackErr)
			} else {
				poll.Track = &track
			}
		}
		if target.NeedOperation {
			timeline, operationErr := coordinator.client.GetFlightTaskOperationTimeline(ctx, token, scope.ProjectUUID, target.RemoteTaskID)
			if operationErr != nil {
				pollErrors = append(pollErrors, operationErr)
			} else {
				poll.Operations = &timeline
			}
		}
		if target.NeedMedia {
			media, mediaErr := coordinator.client.ListFlightTaskMedia(ctx, token, scope.ProjectUUID, target.RemoteTaskID)
			if mediaErr != nil {
				pollErrors = append(pollErrors, mediaErr)
			} else {
				poll.Media = &media
			}
		}
		if poll.Track == nil && poll.Operations == nil && poll.Media == nil {
			continue
		}
		if applyErr := coordinator.sink.ApplyFlightArtifacts(ctx, instance, poll); applyErr != nil {
			pollErrors = append(pollErrors, applyErr)
			continue
		}
		if poll.Track != nil {
			tracks++
		}
		if poll.Operations != nil {
			operations++
		}
		if poll.Media != nil {
			mediaTasks++
		}
	}
	exportRecords, exportComplete, exportErr := coordinator.listAllFlightExports(ctx, token, scope.ProjectUUID)
	if exportErr != nil {
		pollErrors = append(pollErrors, exportErr)
	} else if applyErr := coordinator.sink.ApplyFlightExports(ctx, instance, FlightExportPoll{
		Records: exportRecords, CompleteSnapshot: exportComplete, ReceivedAt: now,
	}); applyErr != nil {
		pollErrors = append(pollErrors, applyErr)
	} else {
		exports = len(exportRecords)
	}
	cursor := map[string]any{
		"targets": len(targets), "tracks": tracks, "operations": operations, "mediaTasks": mediaTasks, "exports": exports,
		"complete": len(targets) < coordinator.config.ArtifactBatchSize && len(pollErrors) == 0,
	}
	return cursor, coordinator.config.CatalogInterval, errors.Join(pollErrors...)
}

func (coordinator *ResourceStreamCoordinator) listAllFlightExports(ctx context.Context, token, projectUUID string) ([]FlightExportRecord, bool, error) {
	const pageSize, maxPages = 100, 100
	items := make([]FlightExportRecord, 0)
	seen := make(map[string]struct{})
	total := -1
	for page := 1; page <= maxPages; page++ {
		result, err := coordinator.client.ListFlightTaskExports(ctx, token, projectUUID, FlightExportOptions{Page: page, PageSize: pageSize})
		if err != nil {
			return items, false, err
		}
		if total < 0 {
			total = result.Pagination.Total
		} else if total != result.Pagination.Total {
			return items, false, schemaError()
		}
		for _, item := range result.List {
			if _, duplicate := seen[item.UUID]; duplicate {
				return items, false, schemaError()
			}
			seen[item.UUID] = struct{}{}
			items = append(items, item)
		}
		if len(items) == total {
			return items, true, nil
		}
		if len(items) > total || len(result.List) == 0 || len(result.List) < pageSize {
			return items, false, schemaError()
		}
	}
	return items, false, &APIError{SafeCode: "snapshot_incomplete", Retryable: true}
}

func (coordinator *ResourceStreamCoordinator) pollWaylines(ctx context.Context, instance connector.Instance, token string) (map[string]any, time.Duration, error) {
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return nil, 0, err
	}
	waylines, err := coordinator.client.ListWaylines(ctx, token, scope.ProjectUUID)
	if err != nil {
		return map[string]any{"resources": 0, "complete": false}, 0, err
	}
	poll := CatalogPoll{Kind: "wayline", Waylines: waylines, CompleteSnapshot: true, ReceivedAt: coordinator.config.Now().UTC()}
	if err := coordinator.sink.ApplyCatalog(ctx, instance, poll); err != nil {
		return map[string]any{"resources": len(waylines), "complete": false}, 0, err
	}
	return map[string]any{"resources": len(waylines), "pages": 1, "completePages": 1, "complete": true}, coordinator.config.CatalogInterval, nil
}

func (coordinator *ResourceStreamCoordinator) pollFlightTasks(ctx context.Context, instance connector.Instance, token string) (map[string]any, time.Duration, error) {
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return nil, 0, err
	}
	devices, err := coordinator.store.ListManagedDevices(ctx, instance)
	if err != nil {
		return nil, 0, err
	}
	docks := make([]connector.ManagedConnectorDevice, 0, len(devices))
	for _, device := range devices {
		if device.Class == "airport" {
			docks = append(docks, device)
		}
	}
	if len(docks) == 0 {
		if err := coordinator.sink.ApplyCatalog(ctx, instance, CatalogPoll{Kind: "flight-task", FlightTasks: []FlightTaskSummary{}, CompleteSnapshot: false, ReceivedAt: coordinator.config.Now().UTC()}); err != nil {
			return map[string]any{"resources": 0, "pages": 0, "completePages": 0, "complete": false}, 0, err
		}
		return map[string]any{"resources": 0, "pages": 0, "completePages": 0, "complete": false}, coordinator.config.CatalogInterval, nil
	}

	byID := make(map[string]FlightTaskSummary)
	completedPages := 0
	var pageErrors []error
	for _, dock := range docks {
		page, pageErr := coordinator.client.ListFlightTasks(ctx, token, scope.ProjectUUID, FlightTaskListOptions{SNs: []string{dock.Serial}})
		if pageErr != nil {
			pageErrors = append(pageErrors, pageErr)
			continue
		}
		completedPages++
		for _, task := range page {
			if _, exists := byID[task.UUID]; !exists {
				byID[task.UUID] = task
			}
		}
		if len(page) == maxFlightTaskBatch {
			pageErrors = append(pageErrors, &APIError{SafeCode: "snapshot_incomplete", Retryable: true})
		}
	}
	tasks := make([]FlightTaskSummary, 0, len(byID))
	for _, task := range byID {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(left, right int) bool { return tasks[left].UUID < tasks[right].UUID })
	complete := len(pageErrors) == 0 && completedPages == len(docks)
	poll := CatalogPoll{Kind: "flight-task", FlightTasks: tasks, CompleteSnapshot: complete, ReceivedAt: coordinator.config.Now().UTC()}
	if sinkErr := coordinator.sink.ApplyCatalog(ctx, instance, poll); sinkErr != nil {
		pageErrors = append(pageErrors, sinkErr)
		complete = false
	}
	cursor := map[string]any{"resources": len(tasks), "pages": len(docks), "completePages": completedPages, "complete": complete}
	return cursor, coordinator.config.CatalogInterval, errors.Join(pageErrors...)
}

func (coordinator *ResourceStreamCoordinator) runStream(
	ctx context.Context,
	instance connector.Instance,
	kind string,
	poll func(context.Context, connector.Instance, string) (map[string]any, time.Duration, error),
) error {
	now := coordinator.config.Now().UTC()
	previous, exists, err := coordinator.store.LoadResourceSyncState(ctx, instance, kind)
	if err != nil {
		return err
	}
	if exists && previous.NextAttemptAt != nil && previous.NextAttemptAt.After(now) {
		return nil
	}
	attempt := previous.AttemptCount
	if err := coordinator.store.SaveResourceSyncState(ctx, instance, connector.ResourceSyncUpdate{
		Kind: kind, Status: "running", Cursor: previous.Cursor, AttemptCount: attempt, StartedAt: &now,
	}); err != nil {
		return err
	}
	preflightCursor := mergeResourceCursor(map[string]any{}, previous.Cursor)
	liveCandidates := 0
	if kind == "active-operations" && coordinator.config.LiveReconciler != nil {
		liveSummary, liveErr := coordinator.config.LiveReconciler.ReconcileLiveSessions(ctx, instance)
		preflightCursor = mergeResourceCursor(preflightCursor, liveSummary.Cursor())
		liveCandidates = liveSummary.Candidates
		if liveErr != nil {
			return coordinator.failStream(ctx, instance, kind, preflightCursor, attempt, now, liveErr)
		}
	}
	token, err := coordinator.resolver.ResolveToken(ctx, instance)
	if err != nil {
		return coordinator.failStream(ctx, instance, kind, preflightCursor, attempt, now, err)
	}
	defer func() { token = "" }()
	cursor, nextInterval, pollErr := poll(ctx, instance, token)
	cursor = mergeResourceCursor(preflightCursor, cursor)
	if pollErr != nil {
		return coordinator.failStream(ctx, instance, kind, cursor, attempt, now, pollErr)
	}
	if nextInterval <= 0 {
		nextInterval = coordinator.config.OfflineInterval
	}
	if liveCandidates > 0 && nextInterval > coordinator.config.OnlineInterval {
		nextInterval = coordinator.config.OnlineInterval
	}
	next := now.Add(nextInterval)
	return coordinator.store.SaveResourceSyncState(ctx, instance, connector.ResourceSyncUpdate{
		Kind: kind, Status: "idle", Cursor: cursor, AttemptCount: 0, SucceededAt: &now, NextAttemptAt: &next,
	})
}

func mergeResourceCursor(target, source map[string]any) map[string]any {
	if target == nil {
		target = map[string]any{}
	}
	for key, value := range source {
		target[key] = value
	}
	return target
}

func (coordinator *ResourceStreamCoordinator) failStream(ctx context.Context, instance connector.Instance, kind string, cursor map[string]any, attempt int, now time.Time, cause error) error {
	attempt++
	delay := time.Second * time.Duration(1<<min(attempt-1, 8))
	delay = min(delay, coordinator.config.MaxBackoff)
	next := now.Add(delay)
	if cursor == nil {
		cursor = map[string]any{}
	}
	saveErr := coordinator.store.SaveResourceSyncState(ctx, instance, connector.ResourceSyncUpdate{
		Kind: kind, Status: "backoff", Cursor: cursor, AttemptCount: attempt,
		LastErrorCode: resourceStreamErrorCode(cause), StartedAt: &now, NextAttemptAt: &next,
	})
	return errors.Join(cause, saveErr)
}

func resourceStreamErrorCode(err error) string {
	if err == nil {
		return "stream_failed"
	}
	if code := SafeCode(err); code != "" && code[0] != '*' {
		return code
	}
	return "stream_failed"
}

func (coordinator *ResourceStreamCoordinator) pollDeviceStates(ctx context.Context, instance connector.Instance, token string) (map[string]any, time.Duration, error) {
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return nil, 0, err
	}
	devices, err := coordinator.store.ListManagedDevices(ctx, instance)
	if err != nil {
		return nil, 0, err
	}
	now := coordinator.config.Now().UTC()
	processed := 0
	var pollErrors []error
	nextInterval := coordinator.config.OfflineInterval
	for _, device := range devices {
		interval := coordinator.config.OfflineInterval
		if device.Online {
			interval = coordinator.config.OnlineInterval
			nextInterval = coordinator.config.OnlineInterval
		}
		key := fmt.Sprintf("%d/%d", instance.ID, device.DeviceID)
		coordinator.mu.Lock()
		dueAt := coordinator.nextPoll[key]
		coordinator.mu.Unlock()
		if dueAt.After(now) {
			continue
		}
		snapshot, stateErr := coordinator.client.GetDeviceState(ctx, token, scope.ProjectUUID, device.Serial)
		coordinator.mu.Lock()
		coordinator.nextPoll[key] = now.Add(interval)
		coordinator.mu.Unlock()
		if stateErr != nil {
			pollErrors = append(pollErrors, stateErr)
			continue
		}
		poll := DeviceStatePoll{Device: device, Snapshot: snapshot, Mapped: MapDeviceState(snapshot), ReceivedAt: now, FreshnessInterval: interval}
		if sinkErr := coordinator.sink.ApplyDeviceState(ctx, instance, poll); sinkErr != nil {
			pollErrors = append(pollErrors, sinkErr)
			continue
		}
		processed++
	}
	cursor := map[string]any{"processed": processed, "managed": len(devices), "mapperVersion": StateMapperVersion}
	return cursor, nextInterval, errors.Join(pollErrors...)
}

func (coordinator *ResourceStreamCoordinator) pollHealth(ctx context.Context, instance connector.Instance, token string) (map[string]any, time.Duration, error) {
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return nil, 0, err
	}
	devices, err := coordinator.store.ListManagedDevices(ctx, instance)
	if err != nil {
		return nil, 0, err
	}
	serials := make([]string, 0, len(devices))
	for _, device := range devices {
		serials = append(serials, device.Serial)
	}
	hms := make([]DeviceHMS, 0)
	var pollErrors []error
	for start := 0; start < len(serials); start += maxHMSDevices {
		end := min(start+maxHMSDevices, len(serials))
		batch, batchErr := coordinator.client.ListDeviceHMS(ctx, token, scope.ProjectUUID, serials[start:end])
		if batchErr != nil {
			pollErrors = append(pollErrors, batchErr)
			continue
		}
		hms = append(hms, batch...)
	}
	autoRecord, autoRecordErr := coordinator.client.GetAutoRecordingConfig(ctx, token, scope.ProjectUUID)
	if autoRecordErr != nil {
		pollErrors = append(pollErrors, autoRecordErr)
	}
	topologies, topologyErr := coordinator.client.ListHistoricalTopologies(ctx, token, scope.ProjectUUID)
	if topologyErr != nil {
		pollErrors = append(pollErrors, topologyErr)
	}
	if len(pollErrors) == 0 {
		if sinkErr := coordinator.sink.ApplyHealth(ctx, instance, HealthPoll{Devices: devices, HMS: hms, Topologies: topologies, AutoRecord: autoRecord, ReceivedAt: coordinator.config.Now().UTC()}); sinkErr != nil {
			pollErrors = append(pollErrors, sinkErr)
		}
	}
	return map[string]any{"managed": len(devices), "hmsDevices": len(hms), "topologies": len(topologies), "autoRecord": autoRecordErr == nil}, coordinator.config.HealthInterval, errors.Join(pollErrors...)
}

type ConcurrentResourceRunner struct {
	inventory connector.SyncRunner
	streams   *ResourceStreamCoordinator
}

type ScheduledInventoryRunner struct {
	runner     connector.SyncRunner
	store      ResourceStreamStore
	interval   time.Duration
	maxBackoff time.Duration
	now        func() time.Time
}

func NewScheduledInventoryRunner(runner connector.SyncRunner, store ResourceStreamStore, interval, maxBackoff time.Duration, now func() time.Time) (*ScheduledInventoryRunner, error) {
	if runner == nil || store == nil || interval <= 0 || maxBackoff <= 0 {
		return nil, errors.New("FlightHub inventory schedule is invalid")
	}
	if now == nil {
		now = time.Now
	}
	return &ScheduledInventoryRunner{runner: runner, store: store, interval: interval, maxBackoff: maxBackoff, now: now}, nil
}

func (runner *ScheduledInventoryRunner) Run(ctx context.Context, instance connector.Instance, mode connector.DiscoveryMode) (connector.SyncApplyResult, error) {
	now := runner.now().UTC()
	previous, exists, err := runner.store.LoadResourceSyncState(ctx, instance, "inventory")
	if err != nil {
		return connector.SyncApplyResult{}, err
	}
	if exists && previous.NextAttemptAt != nil && previous.NextAttemptAt.After(now) {
		return connector.SyncApplyResult{}, nil
	}
	if err := runner.store.SaveResourceSyncState(ctx, instance, connector.ResourceSyncUpdate{
		Kind: "inventory", Status: "running", Cursor: previous.Cursor, AttemptCount: previous.AttemptCount, StartedAt: &now,
	}); err != nil {
		return connector.SyncApplyResult{}, err
	}
	result, runErr := runner.runner.Run(ctx, instance, mode)
	if runErr != nil {
		attempt := previous.AttemptCount + 1
		delay := min(time.Second*time.Duration(1<<min(attempt-1, 8)), runner.maxBackoff)
		next := now.Add(delay)
		saveErr := runner.store.SaveResourceSyncState(ctx, instance, connector.ResourceSyncUpdate{
			Kind: "inventory", Status: "backoff", Cursor: previous.Cursor, AttemptCount: attempt,
			LastErrorCode: resourceStreamErrorCode(runErr), StartedAt: &now, NextAttemptAt: &next,
		})
		return result, errors.Join(runErr, saveErr)
	}
	next := now.Add(runner.interval)
	saveErr := runner.store.SaveResourceSyncState(ctx, instance, connector.ResourceSyncUpdate{
		Kind: "inventory", Status: "idle", Cursor: map[string]any{
			"runId": result.RunID, "discovered": result.Discovered, "managed": result.Managed, "missing": result.Missing,
		}, AttemptCount: 0, SucceededAt: &now, NextAttemptAt: &next,
	})
	return result, saveErr
}

func NewConcurrentResourceRunner(inventory connector.SyncRunner, streams *ResourceStreamCoordinator) (*ConcurrentResourceRunner, error) {
	if inventory == nil || streams == nil {
		return nil, errors.New("FlightHub inventory and resource streams are required")
	}
	return &ConcurrentResourceRunner{inventory: inventory, streams: streams}, nil
}

func (runner *ConcurrentResourceRunner) Run(ctx context.Context, instance connector.Instance, mode connector.DiscoveryMode) (connector.SyncApplyResult, error) {
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		runner.streams.Run(ctx, instance)
	}()
	result, err := runner.inventory.Run(ctx, instance, mode)
	wait.Wait()
	return result, err
}
