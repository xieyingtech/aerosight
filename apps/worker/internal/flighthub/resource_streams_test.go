package flighthub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
)

type resourceStoreFixture struct {
	mu      sync.Mutex
	states  map[string]connector.ResourceSyncUpdate
	devices []connector.ManagedConnectorDevice
}

func (store *resourceStoreFixture) LoadResourceSyncState(_ context.Context, _ connector.Instance, kind string) (connector.ResourceSyncUpdate, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	state, ok := store.states[kind]
	return state, ok, nil
}

func (store *resourceStoreFixture) SaveResourceSyncState(_ context.Context, _ connector.Instance, update connector.ResourceSyncUpdate) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.states == nil {
		store.states = map[string]connector.ResourceSyncUpdate{}
	}
	store.states[update.Kind] = update
	return nil
}

func (store *resourceStoreFixture) ListManagedDevices(context.Context, connector.Instance) ([]connector.ManagedConnectorDevice, error) {
	return append([]connector.ManagedConnectorDevice(nil), store.devices...), nil
}

func (store *resourceStoreFixture) state(kind string) connector.ResourceSyncUpdate {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.states[kind]
}

type resourceClientFixture struct {
	mu                         sync.Mutex
	stateCalls                 []string
	stateErr                   error
	hmsErr                     error
	autoRecordErr              error
	waylines                   []WaylineSummary
	waylineErr                 error
	taskPages                  map[string][]FlightTaskSummary
	taskErrors                 map[string]error
	tracks                     map[string]FlightTaskTrack
	trackErrors                map[string]error
	operations                 map[string]FlightTaskOperationTimeline
	operationErrors            map[string]error
	media                      map[string][]FlightTaskMedia
	mediaErrors                map[string]error
	exports                    []FlightExportRecord
	exportError                error
	flightAlerts               map[string][]FlightAlertSummary
	flightAlertErrs            map[string]error
	aiAlerts                   []AIAlertRecord
	aiAlertError               error
	projectRecordingCalls      int
	organizationRecordingCalls int
	liveShareCalls             int
	streamConverterCalls       int
}

func (client *resourceClientFixture) GetDeviceState(_ context.Context, _, _, serial string) (DeviceStateSnapshot, error) {
	client.mu.Lock()
	client.stateCalls = append(client.stateCalls, serial)
	client.mu.Unlock()
	if client.stateErr != nil {
		return DeviceStateSnapshot{}, client.stateErr
	}
	model := DeviceModel{Key: "0-91-1", Class: "drone"}
	if serial == "DOCK_REDACTED" {
		model = DeviceModel{Key: "3-2-0", Class: "airport"}
	}
	return DeviceStateSnapshot{SN: serial, Model: model, State: map[string]json.RawMessage{
		"longitude": json.RawMessage(`120`), "latitude": json.RawMessage(`30`), "mode_code": json.RawMessage(`0`),
	}}, nil
}

func (client *resourceClientFixture) ListDeviceHMS(context.Context, string, string, []string) ([]DeviceHMS, error) {
	return []DeviceHMS{}, client.hmsErr
}

func (client *resourceClientFixture) ListHistoricalTopologies(context.Context, string, string) ([]HistoricalTopology, error) {
	return []HistoricalTopology{}, nil
}

func (client *resourceClientFixture) GetAutoRecordingConfig(context.Context, string, string) (AutoRecordingConfig, error) {
	if client.autoRecordErr != nil {
		return AutoRecordingConfig{}, client.autoRecordErr
	}
	return AutoRecordingConfig{ID: 1, ProjectID: "PROJECT_REDACTED", Items: []AutoRecordingItem{}}, nil
}

func (client *resourceClientFixture) ListWaylines(context.Context, string, string) ([]WaylineSummary, error) {
	return append([]WaylineSummary(nil), client.waylines...), client.waylineErr
}

func (client *resourceClientFixture) ListFlightTasks(_ context.Context, _, _ string, options FlightTaskListOptions) ([]FlightTaskSummary, error) {
	serial := ""
	if len(options.SNs) == 1 {
		serial = options.SNs[0]
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	return append([]FlightTaskSummary(nil), client.taskPages[serial]...), client.taskErrors[serial]
}

func (client *resourceClientFixture) GetFlightTaskTrack(_ context.Context, _, _, taskID string) (FlightTaskTrack, error) {
	return client.tracks[taskID], client.trackErrors[taskID]
}

func (client *resourceClientFixture) GetFlightTaskOperationTimeline(_ context.Context, _, _, taskID string) (FlightTaskOperationTimeline, error) {
	return client.operations[taskID], client.operationErrors[taskID]
}

func (client *resourceClientFixture) ListFlightTaskMedia(_ context.Context, _, _, taskID string) ([]FlightTaskMedia, error) {
	return append([]FlightTaskMedia(nil), client.media[taskID]...), client.mediaErrors[taskID]
}

func (client *resourceClientFixture) ListFlightTaskExports(_ context.Context, _, _ string, options FlightExportOptions) (FlightExportPage, error) {
	if client.exportError != nil {
		return FlightExportPage{}, client.exportError
	}
	if options.Page > 1 {
		return FlightExportPage{Pagination: FlightExportPagination{Page: options.Page, PageSize: options.PageSize, Total: len(client.exports)}, List: []FlightExportRecord{}}, nil
	}
	return FlightExportPage{Pagination: FlightExportPagination{Page: options.Page, PageSize: options.PageSize, Total: len(client.exports)}, List: append([]FlightExportRecord(nil), client.exports...)}, nil
}

func (client *resourceClientFixture) ListFlightAlerts(_ context.Context, _, _ string, options FlightAlertOptions) (FlightAlertPage, error) {
	if err := client.flightAlertErrs[options.DroneSN]; err != nil {
		return FlightAlertPage{}, err
	}
	items := append([]FlightAlertSummary(nil), client.flightAlerts[options.DroneSN]...)
	if options.Page > 1 {
		items = []FlightAlertSummary{}
	}
	return FlightAlertPage{Data: items, Total: len(client.flightAlerts[options.DroneSN]), Page: options.Page, PageSize: options.PageSize, PageCount: boolPageCount(len(client.flightAlerts[options.DroneSN]))}, nil
}

func boolPageCount(total int) int {
	if total > 0 {
		return 1
	}
	return 0
}

func (client *resourceClientFixture) ListAIAlertRecords(_ context.Context, _, _ string, options AIAlertOptions) (AIAlertPage, error) {
	if client.aiAlertError != nil {
		return AIAlertPage{}, client.aiAlertError
	}
	requested := make(map[string]struct{}, len(options.FlightIDs))
	for _, flightID := range options.FlightIDs {
		requested[flightID] = struct{}{}
	}
	data := make(map[string][]AIAlertRecord)
	total := 0
	if options.Page == 1 {
		for _, alert := range client.aiAlerts {
			if _, ok := requested[alert.FlightID]; ok {
				data[alert.FlightID] = append(data[alert.FlightID], alert)
				total++
			}
		}
	}
	return AIAlertPage{Data: data, Total: total, Page: options.Page, PageSize: options.PageSize, PageCount: boolPageCount(total)}, nil
}

func (client *resourceClientFixture) ListOrganizationRecordingTasks(context.Context, string, string, string) ([]RecordingTask, error) {
	client.mu.Lock()
	client.organizationRecordingCalls++
	client.mu.Unlock()
	return []RecordingTask{}, nil
}

func (client *resourceClientFixture) ListProjectRecordingTasks(context.Context, string, string, string) ([]RecordingTask, error) {
	client.mu.Lock()
	client.projectRecordingCalls++
	client.mu.Unlock()
	return []RecordingTask{}, nil
}

func (client *resourceClientFixture) ListLiveShares(context.Context, string, string, LiveShareListOptions) ([]LiveShare, error) {
	client.mu.Lock()
	client.liveShareCalls++
	client.mu.Unlock()
	return []LiveShare{}, nil
}

func (client *resourceClientFixture) ListStreamConverters(_ context.Context, _, _ string, options StreamConverterListOptions) (PageResult[StreamConverter], error) {
	client.mu.Lock()
	client.streamConverterCalls++
	client.mu.Unlock()
	return PageResult[StreamConverter]{
		Pagination: Pagination{Page: options.Page, PageSize: options.PageSize, Total: 0},
		List:       []StreamConverter{},
	}, nil
}

func (client *resourceClientFixture) liveCalls() (int, int, int, int) {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.projectRecordingCalls, client.organizationRecordingCalls, client.liveShareCalls, client.streamConverterCalls
}

func (client *resourceClientFixture) calls() []string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return append([]string(nil), client.stateCalls...)
}

type resourceSinkFixture struct {
	stateApplied    chan DeviceStatePoll
	mu              sync.Mutex
	health          int
	states          int
	catalogs        []CatalogPoll
	artifactTargets []FlightArtifactTarget
	artifacts       []FlightArtifactPoll
	exportPolls     []FlightExportPoll
	alertPolls      []FlightAlertPoll
	liveCatalogs    []LiveCatalogPoll
}

func (sink *resourceSinkFixture) ApplyDeviceState(_ context.Context, _ connector.Instance, poll DeviceStatePoll) error {
	sink.mu.Lock()
	sink.states++
	sink.mu.Unlock()
	if sink.stateApplied != nil {
		sink.stateApplied <- poll
	}
	return nil
}

func (sink *resourceSinkFixture) stateCount() int {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return sink.states
}

func (sink *resourceSinkFixture) ApplyHealth(context.Context, connector.Instance, HealthPoll) error {
	sink.mu.Lock()
	sink.health++
	sink.mu.Unlock()
	return nil
}

func (sink *resourceSinkFixture) ApplyCatalog(_ context.Context, _ connector.Instance, poll CatalogPoll) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	poll.Waylines = append([]WaylineSummary(nil), poll.Waylines...)
	poll.FlightTasks = append([]FlightTaskSummary(nil), poll.FlightTasks...)
	sink.catalogs = append(sink.catalogs, poll)
	return nil
}

func (sink *resourceSinkFixture) ListFlightArtifactTargets(context.Context, connector.Instance, int) ([]FlightArtifactTarget, error) {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return append([]FlightArtifactTarget(nil), sink.artifactTargets...), nil
}

func (sink *resourceSinkFixture) ApplyFlightArtifacts(_ context.Context, _ connector.Instance, poll FlightArtifactPoll) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.artifacts = append(sink.artifacts, poll)
	return nil
}

func (sink *resourceSinkFixture) ApplyFlightExports(_ context.Context, _ connector.Instance, poll FlightExportPoll) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	poll.Records = append([]FlightExportRecord(nil), poll.Records...)
	sink.exportPolls = append(sink.exportPolls, poll)
	return nil
}

func (sink *resourceSinkFixture) ApplyFlightAlerts(_ context.Context, _ connector.Instance, poll FlightAlertPoll) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	poll.Aggregates = append([]FlightAlertSummary(nil), poll.Aggregates...)
	poll.Alerts = append([]AIAlertRecord(nil), poll.Alerts...)
	sink.alertPolls = append(sink.alertPolls, poll)
	return nil
}

func (sink *resourceSinkFixture) ApplyLiveCatalog(_ context.Context, _ connector.Instance, poll LiveCatalogPoll) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	poll.Devices = append([]connector.ManagedConnectorDevice(nil), poll.Devices...)
	poll.Recordings = append([]ScopedRecordingTask(nil), poll.Recordings...)
	poll.Shares = append([]LiveShare(nil), poll.Shares...)
	poll.Converters = append([]StreamConverter(nil), poll.Converters...)
	sink.liveCatalogs = append(sink.liveCatalogs, poll)
	return nil
}

func (sink *resourceSinkFixture) catalog(kind string) CatalogPoll {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	for index := len(sink.catalogs) - 1; index >= 0; index-- {
		if sink.catalogs[index].Kind == kind {
			return sink.catalogs[index]
		}
	}
	return CatalogPoll{}
}

func (sink *resourceSinkFixture) appliedArtifacts() []FlightArtifactPoll {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return append([]FlightArtifactPoll(nil), sink.artifacts...)
}

type blockingInventoryRunner struct {
	started chan struct{}
	release chan struct{}
}

type countingInventoryRunner struct{ calls int }

type liveReconcilerFixture struct {
	calls   int
	summary FlightHubLiveReconcileSummary
	err     error
}

func (fixture *liveReconcilerFixture) ReconcileLiveSessions(context.Context, connector.Instance) (FlightHubLiveReconcileSummary, error) {
	fixture.calls++
	return fixture.summary, fixture.err
}

func (runner *countingInventoryRunner) Run(context.Context, connector.Instance, connector.DiscoveryMode) (connector.SyncApplyResult, error) {
	runner.calls++
	return connector.SyncApplyResult{RunID: int64(runner.calls), Discovered: 2}, nil
}

func (runner blockingInventoryRunner) Run(context.Context, connector.Instance, connector.DiscoveryMode) (connector.SyncApplyResult, error) {
	close(runner.started)
	<-runner.release
	return connector.SyncApplyResult{Discovered: 2}, nil
}

func resourceStreamInstance() connector.Instance {
	return connector.Instance{ID: 7, ProjectID: 3, ConnectorKey: ConnectorKey, Version: ConnectorVersion, DiscoveryScope: json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"测试项目","organizationUuid":"00000000-0000-4000-8000-000000000010"}`)}
}

func TestLiveCatalogEmptySnapshotsRemainHealthyAndComplete(t *testing.T) {
	now := time.Date(2026, 9, 2, 5, 0, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}, devices: []connector.ManagedConnectorDevice{
		{DeviceID: 1, TeamID: 2, Serial: "DOCK_REDACTED", Class: "airport"},
		{DeviceID: 2, TeamID: 2, Serial: "AIRCRAFT_REDACTED", Class: "drone"},
	}}
	client := &resourceClientFixture{}
	sink := &resourceSinkFixture{}
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute,
		CatalogInterval: 15 * time.Minute, MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.runStream(context.Background(), resourceStreamInstance(), "live", coordinator.pollLiveCatalog); err != nil {
		t.Fatal(err)
	}
	state := store.state("live")
	if state.Status != "idle" || state.LastErrorCode != "" || state.Cursor["complete"] != true ||
		state.Cursor["recordings"] != 0 || state.Cursor["shares"] != 0 || state.Cursor["converters"] != 0 {
		t.Fatalf("empty live stream state=%#v", state)
	}
	projectCalls, organizationCalls, shareCalls, converterCalls := client.liveCalls()
	if projectCalls != 2 || organizationCalls != 2 || shareCalls != 1 || converterCalls != 1 {
		t.Fatalf("live calls project=%d organization=%d shares=%d converters=%d", projectCalls, organizationCalls, shareCalls, converterCalls)
	}
	if len(sink.liveCatalogs) != 1 || !sink.liveCatalogs[0].RecordingComplete ||
		!sink.liveCatalogs[0].ShareComplete || !sink.liveCatalogs[0].ConverterComplete {
		t.Fatalf("empty live catalog=%#v", sink.liveCatalogs)
	}
}

func TestLiveCatalogLegacyScopeKeepsProjectRecordingSnapshotComplete(t *testing.T) {
	now := time.Date(2026, 9, 2, 5, 30, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}, devices: []connector.ManagedConnectorDevice{
		{DeviceID: 2, TeamID: 2, Serial: "AIRCRAFT_REDACTED", Class: "drone"},
	}}
	client := &resourceClientFixture{}
	sink := &resourceSinkFixture{}
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute,
		CatalogInterval: 15 * time.Minute, MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	instance := resourceStreamInstance()
	instance.DiscoveryScope = json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"测试项目"}`)
	if err := coordinator.runStream(context.Background(), instance, "live", coordinator.pollLiveCatalog); err != nil {
		t.Fatal(err)
	}
	projectCalls, organizationCalls, _, _ := client.liveCalls()
	if projectCalls != 1 || organizationCalls != 0 || store.state("live").Cursor["complete"] != true {
		t.Fatalf("legacy live state=%#v project calls=%d organization calls=%d", store.state("live"), projectCalls, organizationCalls)
	}
}

func TestActiveOperationsReconcilesLiveBeforeTokenResolutionAndPreservesCursor(t *testing.T) {
	now := time.Date(2026, 9, 2, 4, 0, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{
		"active-operations": {Kind: "active-operations", Cursor: map[string]any{"previousAlerts": 3}},
	}}
	live := &liveReconcilerFixture{summary: FlightHubLiveReconcileSummary{Candidates: 2, Updated: 1}}
	coordinator, err := NewResourceStreamCoordinator(
		&resourceClientFixture{}, tokenResolverFixture{err: &APIError{SafeCode: "credential_invalid"}}, store,
		&resourceSinkFixture{}, ResourceStreamConfig{
			OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute,
			CatalogInterval: 15 * time.Minute, MaxBackoff: 5 * time.Minute, LiveReconciler: live,
			Now: func() time.Time { return now },
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	pollCalled := false
	err = coordinator.runStream(context.Background(), resourceStreamInstance(), "active-operations",
		func(context.Context, connector.Instance, string) (map[string]any, time.Duration, error) {
			pollCalled = true
			return nil, 0, nil
		})
	if !IsSafeCode(err, "credential_invalid") || live.calls != 1 || pollCalled {
		t.Fatalf("err=%v live calls=%d poll called=%v", err, live.calls, pollCalled)
	}
	state := store.state("active-operations")
	if state.Status != "backoff" || state.Cursor["previousAlerts"] != 3 ||
		state.Cursor["liveCandidates"] != 2 || state.Cursor["liveUpdated"] != 1 {
		t.Fatalf("active operations state=%#v", state)
	}
}

func TestActiveLiveCandidatesTightenIntervalAndPollCursorWins(t *testing.T) {
	now := time.Date(2026, 9, 2, 4, 30, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{
		"active-operations": {Kind: "active-operations", Cursor: map[string]any{"complete": false, "previousAlerts": 3}},
	}}
	live := &liveReconcilerFixture{summary: FlightHubLiveReconcileSummary{Candidates: 1}}
	coordinator, err := NewResourceStreamCoordinator(
		&resourceClientFixture{}, tokenResolverFixture{token: "TOKEN_REDACTED"}, store,
		&resourceSinkFixture{}, ResourceStreamConfig{
			OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute,
			CatalogInterval: 15 * time.Minute, MaxBackoff: 5 * time.Minute, LiveReconciler: live,
			Now: func() time.Time { return now },
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = coordinator.runStream(context.Background(), resourceStreamInstance(), "active-operations",
		func(context.Context, connector.Instance, string) (map[string]any, time.Duration, error) {
			return map[string]any{"complete": true, "alerts": 4}, 15 * time.Minute, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	state := store.state("active-operations")
	if state.Status != "idle" || state.Cursor["complete"] != true || state.Cursor["previousAlerts"] != 3 ||
		state.Cursor["alerts"] != 4 || state.Cursor["liveCandidates"] != 1 || state.NextAttemptAt == nil ||
		!state.NextAttemptAt.Equal(now.Add(15*time.Second)) {
		t.Fatalf("active operations state=%#v", state)
	}
}

func TestResourceStreamsRunInsideLeaseWithoutSlowInventoryOrHMSBlockingPosition(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}, devices: []connector.ManagedConnectorDevice{{DeviceID: 1, Serial: "AIRCRAFT_REDACTED", Online: true}}}
	client := &resourceClientFixture{hmsErr: errors.New("HMS temporarily unavailable")}
	sink := &resourceSinkFixture{stateApplied: make(chan DeviceStatePoll, 1)}
	streamErrors := make(chan string, 2)
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute,
		MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now }, OnError: func(kind string, _ error) { streamErrors <- kind },
	})
	if err != nil {
		t.Fatal(err)
	}
	inventory := blockingInventoryRunner{started: make(chan struct{}), release: make(chan struct{})}
	runner, err := NewConcurrentResourceRunner(inventory, coordinator)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, runErr := runner.Run(context.Background(), resourceStreamInstance(), connector.DiscoveryPoll)
		done <- runErr
	}()
	<-inventory.started
	select {
	case poll := <-sink.stateApplied:
		if poll.Mapped.Position == nil || poll.Mapped.Position.Validity != "valid" {
			t.Fatalf("position stream mapped invalid state: %#v", poll)
		}
	case <-time.After(time.Second):
		t.Fatal("slow inventory or HMS blocked the position stream")
	}
	close(inventory.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if state := store.state("device-state"); state.Status != "idle" || state.AttemptCount != 0 {
		t.Fatalf("device state stream=%#v", state)
	}
	if health := store.state("health"); health.Status != "backoff" || health.AttemptCount != 1 || health.LastErrorCode != "stream_failed" {
		t.Fatalf("health stream=%#v", health)
	}
	select {
	case kind := <-streamErrors:
		if kind != "health" {
			t.Fatalf("unexpected stream error %q", kind)
		}
	default:
		t.Fatal("health failure did not reach safe diagnostics")
	}
}

func TestDeviceStateStreamUsesAdaptiveIntervalsAndRecoversPersistedBackoff(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}, devices: []connector.ManagedConnectorDevice{
		{DeviceID: 1, Serial: "AIRCRAFT_REDACTED", Online: true},
		{DeviceID: 2, Serial: "DOCK_REDACTED", Online: false},
	}}
	client := &resourceClientFixture{}
	sink := &resourceSinkFixture{}
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute,
		MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	instance := resourceStreamInstance()
	coordinator.Run(context.Background(), instance)
	if calls := client.calls(); len(calls) != 2 {
		t.Fatalf("initial adaptive poll calls=%v", calls)
	}
	now = now.Add(15 * time.Second)
	coordinator.Run(context.Background(), instance)
	if calls := client.calls(); len(calls) != 3 || calls[2] != "AIRCRAFT_REDACTED" {
		t.Fatalf("online/offline adaptive poll calls=%v", calls)
	}

	client.stateErr = &APIError{SafeCode: "rate_limited", Retryable: true}
	now = now.Add(15 * time.Second)
	appliedBeforeFailure := sink.stateCount()
	coordinator.Run(context.Background(), instance)
	failed := store.state("device-state")
	if failed.Status != "backoff" || failed.AttemptCount != 1 || failed.LastErrorCode != "rate_limited" || failed.NextAttemptAt == nil {
		t.Fatalf("persisted backoff=%#v", failed)
	}
	if sink.stateCount() != appliedBeforeFailure {
		t.Fatal("failed poll refreshed the previous device state")
	}
	callCount := len(client.calls())
	now = now.Add(500 * time.Millisecond)
	coordinator.Run(context.Background(), instance)
	if len(client.calls()) != callCount {
		t.Fatal("persisted backoff was ignored")
	}
	client.stateErr = nil
	now = failed.NextAttemptAt.Add(time.Millisecond)
	coordinator.Run(context.Background(), instance)
	if recovered := store.state("device-state"); recovered.Status != "idle" || recovered.AttemptCount != 0 {
		t.Fatalf("stream did not recover from backoff: %#v", recovered)
	}
}

func catalogTask(uuid, status string) FlightTaskSummary {
	return FlightTaskSummary{
		UUID: uuid, Name: "脱敏任务", TaskType: "immediate", Status: status,
		SN: "DOCK_REDACTED", WaylineUUID: "WAYLINE_REDACTED", BeginAt: "2026-09-01T10:00:00Z",
	}
}

func TestCatalogStreamsDeduplicateRepeatedPagesAndProtectPartialSnapshots(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}, devices: []connector.ManagedConnectorDevice{
		{DeviceID: 1, Serial: "DOCK_REDACTED_01", Class: "airport"},
		{DeviceID: 2, Serial: "DOCK_REDACTED_02", Class: "airport"},
		{DeviceID: 3, Serial: "AIRCRAFT_REDACTED", Class: "drone"},
	}}
	client := &resourceClientFixture{
		waylines: []WaylineSummary{{
			ID: "WAYLINE_REDACTED", Name: "脱敏航线", DeviceModelKey: "0-91-1", TemplateTypes: []string{"waypoint"},
			PayloadInformation: []WaylinePayload{{Domain: "1", Type: "98", LensType: "wide"}}, UpdatedAt: now.UnixMilli(), SizeBytes: 1024,
		}},
		taskPages: map[string][]FlightTaskSummary{
			"DOCK_REDACTED_01": {catalogTask("TASK_REDACTED_01", "executing"), catalogTask("TASK_REDACTED_02", "waiting")},
			"DOCK_REDACTED_02": {catalogTask("TASK_REDACTED_01", "executing")},
		},
		taskErrors: map[string]error{},
	}
	sink := &resourceSinkFixture{}
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute, CatalogInterval: 15 * time.Minute,
		MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	instance := resourceStreamInstance()
	for iteration := 0; iteration < 2; iteration++ {
		waylineCursor, _, err := coordinator.pollWaylines(context.Background(), instance, "TOKEN_REDACTED")
		if err != nil || waylineCursor["complete"] != true {
			t.Fatalf("wayline cursor=%#v err=%v", waylineCursor, err)
		}
		taskCursor, _, err := coordinator.pollFlightTasks(context.Background(), instance, "TOKEN_REDACTED")
		if err != nil || taskCursor["complete"] != true || taskCursor["resources"] != 2 {
			t.Fatalf("task cursor=%#v err=%v", taskCursor, err)
		}
		projected := sink.catalog("flight-task")
		if !projected.CompleteSnapshot || len(projected.FlightTasks) != 2 || projected.FlightTasks[0].UUID != "TASK_REDACTED_01" || projected.FlightTasks[1].UUID != "TASK_REDACTED_02" {
			t.Fatalf("deduplicated task projection=%#v", projected)
		}
	}

	client.taskErrors["DOCK_REDACTED_02"] = &APIError{SafeCode: "upstream_unavailable", Retryable: true}
	cursor, _, err := coordinator.pollFlightTasks(context.Background(), instance, "TOKEN_REDACTED")
	partial := sink.catalog("flight-task")
	if !IsSafeCode(err, "upstream_unavailable") || cursor["complete"] != false || partial.CompleteSnapshot || len(partial.FlightTasks) != 2 {
		t.Fatalf("partial cursor=%#v poll=%#v err=%v", cursor, partial, err)
	}
}

func TestFlightTaskCatalogAtVendorLimitFailsSnapshotClosed(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	page := make([]FlightTaskSummary, maxFlightTaskBatch)
	for index := range page {
		page[index] = catalogTask(fmt.Sprintf("TASK_REDACTED_%03d", index), "success")
	}
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}, devices: []connector.ManagedConnectorDevice{{DeviceID: 1, Serial: "DOCK_REDACTED", Class: "airport"}}}
	client := &resourceClientFixture{taskPages: map[string][]FlightTaskSummary{"DOCK_REDACTED": page}, taskErrors: map[string]error{}}
	sink := &resourceSinkFixture{}
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute, CatalogInterval: 15 * time.Minute,
		MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _, err := coordinator.pollFlightTasks(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
	if !IsSafeCode(err, "snapshot_incomplete") || cursor["complete"] != false || sink.catalog("flight-task").CompleteSnapshot {
		t.Fatalf("vendor limit cursor=%#v err=%v", cursor, err)
	}
}

func TestFlightArtifactStreamAppliesIndependentTrackOperationMediaAndExportResults(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	target := FlightArtifactTarget{RemoteTaskID: "TASK_REDACTED_01", TaskRunID: 17, NeedTrack: true, NeedOperation: true, NeedMedia: true}
	track := FlightTaskTrack{Track: FlightTrack{
		ID: "TRACK_REDACTED", DroneSN: "AIRCRAFT_REDACTED", Points: []FlightTrackPoint{{Timestamp: now.UnixMilli(), Latitude: 30, Longitude: 120, Height: 10}},
	}}
	timeline := FlightTaskOperationTimeline{
		ControlChanges: []FlightControlChange{}, PayloadChanges: []FlightControlChange{},
		OperationLogs: []FlightOperationLog{{Method: "pause_task", Time: now.UnixMilli()}}, RelatedUsers: []FlightOperationUser{},
	}
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}}
	client := &resourceClientFixture{
		tracks: map[string]FlightTaskTrack{target.RemoteTaskID: track}, trackErrors: map[string]error{},
		operations: map[string]FlightTaskOperationTimeline{target.RemoteTaskID: timeline}, operationErrors: map[string]error{},
		media: map[string][]FlightTaskMedia{target.RemoteTaskID: {{UUID: "MEDIA_REDACTED", Name: "脱敏照片", FileType: "image"}}}, mediaErrors: map[string]error{},
		exports: []FlightExportRecord{{UUID: "EXPORT_REDACTED", Status: "export_complete", Progress: 100}},
	}
	sink := &resourceSinkFixture{artifactTargets: []FlightArtifactTarget{target}}
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute, CatalogInterval: 15 * time.Minute,
		ArtifactBatchSize: 25, MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _, err := coordinator.pollFlightArtifacts(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
	if err != nil || cursor["tracks"] != 1 || cursor["operations"] != 1 || cursor["mediaTasks"] != 1 || cursor["exports"] != 1 || cursor["complete"] != true {
		t.Fatalf("artifact cursor=%#v err=%v", cursor, err)
	}
	applied := sink.appliedArtifacts()
	if len(applied) != 1 || applied[0].Track == nil || applied[0].Operations == nil || applied[0].Media == nil || applied[0].Target.TaskRunID != target.TaskRunID {
		t.Fatalf("artifact projection=%#v", applied)
	}

	client.trackErrors[target.RemoteTaskID] = &APIError{SafeCode: "upstream_unavailable", Retryable: true}
	sink.artifacts = nil
	cursor, _, err = coordinator.pollFlightArtifacts(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
	applied = sink.appliedArtifacts()
	if !IsSafeCode(err, "upstream_unavailable") || cursor["tracks"] != 0 || cursor["operations"] != 1 || cursor["mediaTasks"] != 1 || cursor["exports"] != 1 || cursor["complete"] != false || len(applied) != 1 || applied[0].Track != nil || applied[0].Operations == nil || applied[0].Media == nil {
		t.Fatalf("partial artifact cursor=%#v projection=%#v err=%v", cursor, applied, err)
	}
}

func TestFlightAlertStreamPagesDronesAndAppliesOnlyCompleteSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}, devices: []connector.ManagedConnectorDevice{
		{DeviceID: 1, Serial: "AIRCRAFT_REDACTED_01", Class: "drone"},
		{DeviceID: 2, Serial: "DOCK_REDACTED_01", Class: "airport"},
	}}
	client := &resourceClientFixture{
		flightAlerts: map[string][]FlightAlertSummary{
			"AIRCRAFT_REDACTED_01": {{FlightID: "FLIGHT_REDACTED_01", Count: 1, TaskName: "脱敏任务", TaskType: 1, StartTime: now.Unix(), Status: 1}},
		},
		flightAlertErrs: map[string]error{},
		aiAlerts: []AIAlertRecord{{AlertUUID: "ALERT_REDACTED_01", FlightID: "FLIGHT_REDACTED_01", ProjectID: runtimeProjectUUID,
			DroneSN: "AIRCRAFT_REDACTED_01", Status: 2, Timestamp: now.UnixMilli()}},
	}
	sink := &resourceSinkFixture{}
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute, CatalogInterval: 15 * time.Minute,
		MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _, err := coordinator.pollFlightAlerts(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
	if err != nil || cursor["complete"] != true || cursor["drones"] != 1 || cursor["flights"] != 1 || cursor["alerts"] != 1 {
		t.Fatalf("alert cursor=%#v err=%v", cursor, err)
	}
	if len(sink.alertPolls) != 1 || !sink.alertPolls[0].CompleteSnapshot || len(sink.alertPolls[0].Aggregates) != 1 || len(sink.alertPolls[0].Alerts) != 1 {
		t.Fatalf("alert poll=%#v", sink.alertPolls)
	}
	client.aiAlertError = &APIError{SafeCode: "upstream_unavailable", Retryable: true}
	cursor, _, err = coordinator.pollFlightAlerts(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
	if !IsSafeCode(err, "upstream_unavailable") || cursor["complete"] != false || len(sink.alertPolls) != 1 {
		t.Fatalf("partial alert cursor=%#v polls=%d err=%v", cursor, len(sink.alertPolls), err)
	}
}

func TestInventoryUsesIndependentPersistentCadence(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}}
	inventory := &countingInventoryRunner{}
	runner, err := NewScheduledInventoryRunner(inventory, store, 10*time.Minute, 5*time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	instance := resourceStreamInstance()
	first, err := runner.Run(context.Background(), instance, connector.DiscoveryPoll)
	if err != nil || inventory.calls != 1 || first.RunID != 1 {
		t.Fatalf("first inventory=%#v calls=%d err=%v", first, inventory.calls, err)
	}
	now = now.Add(15 * time.Second)
	skipped, err := runner.Run(context.Background(), instance, connector.DiscoveryPoll)
	if err != nil || inventory.calls != 1 || skipped.RunID != 0 {
		t.Fatalf("inventory cadence did not skip=%#v calls=%d err=%v", skipped, inventory.calls, err)
	}
	now = now.Add(10 * time.Minute)
	second, err := runner.Run(context.Background(), instance, connector.DiscoveryPoll)
	if err != nil || inventory.calls != 2 || second.RunID != 2 {
		t.Fatalf("inventory did not resume=%#v calls=%d err=%v", second, inventory.calls, err)
	}
}
