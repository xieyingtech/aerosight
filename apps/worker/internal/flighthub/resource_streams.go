package flighthub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"aerosight/worker/internal/connector"
)

type ResourceStreamClient interface {
	GetDeviceState(context.Context, string, string, string) (DeviceStateSnapshot, error)
	ListDeviceHMS(context.Context, string, string, []string) ([]DeviceHMS, error)
	ListHistoricalTopologies(context.Context, string, string) ([]HistoricalTopology, error)
	GetAutoRecordingConfig(context.Context, string, string) (AutoRecordingConfig, error)
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

type ResourceStreamSink interface {
	ApplyDeviceState(context.Context, connector.Instance, DeviceStatePoll) error
	ApplyHealth(context.Context, connector.Instance, HealthPoll) error
}

type ResourceStreamConfig struct {
	OnlineInterval  time.Duration
	OfflineInterval time.Duration
	HealthInterval  time.Duration
	MaxBackoff      time.Duration
	Now             func() time.Time
	OnError         func(string, error)
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
	if client == nil || resolver == nil || store == nil || sink == nil || config.OnlineInterval <= 0 || config.OfflineInterval < config.OnlineInterval || config.HealthInterval <= 0 || config.MaxBackoff <= 0 {
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
	token, err := coordinator.resolver.ResolveToken(ctx, instance)
	if err != nil {
		return coordinator.failStream(ctx, instance, kind, previous.Cursor, attempt, now, err)
	}
	defer func() { token = "" }()
	cursor, nextInterval, pollErr := poll(ctx, instance, token)
	if pollErr != nil {
		return coordinator.failStream(ctx, instance, kind, cursor, attempt, now, pollErr)
	}
	if nextInterval <= 0 {
		nextInterval = coordinator.config.OfflineInterval
	}
	next := now.Add(nextInterval)
	return coordinator.store.SaveResourceSyncState(ctx, instance, connector.ResourceSyncUpdate{
		Kind: kind, Status: "idle", Cursor: cursor, AttemptCount: 0, SucceededAt: &now, NextAttemptAt: &next,
	})
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
