package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
)

const geospatialActionTestSecret = "abcdef0123456789abcdef0123456789"

type memoryGeospatialActionStore struct {
	job           GeospatialActionJob
	targetVersion string
	completed     bool
}

func (store *memoryGeospatialActionStore) Load(context.Context, int, string) (GeospatialActionJob, error) {
	job := store.job
	if job.TargetResourceID.Valid {
		job.TargetRemoteVersion = store.targetVersion
	}
	return job, nil
}
func (store *memoryGeospatialActionStore) Begin(_ context.Context, job GeospatialActionJob) error {
	if store.job.AttemptCount != 0 {
		return errors.New("duplicate unsafe attempt")
	}
	if job.TargetResourceID.Valid && job.ExpectedRemoteVersion.String != store.targetVersion {
		return &APIError{SafeCode: "version_conflict"}
	}
	store.job.AttemptCount, store.job.Status = 1, "executing"
	return nil
}
func (store *memoryGeospatialActionStore) Complete(_ context.Context, job GeospatialActionJob, _ GeospatialActionPayload,
	_ string, _ MapElementDeleteResult,
) error {
	store.job.Status, store.completed = "succeeded", true
	if job.ActionKind == "map-element-update" {
		store.targetVersion = "version-2"
	}
	if job.ActionKind == "map-element-delete" {
		store.job.TargetStatus = "missing"
	}
	return nil
}
func (store *memoryGeospatialActionStore) Fail(_ context.Context, _ GeospatialActionJob, code string) error {
	store.job.Status, store.job.TargetStatus = "failed", code
	return nil
}
func (store *memoryGeospatialActionStore) Block(_ context.Context, _ GeospatialActionJob, code string) error {
	store.job.Status, store.job.TargetStatus = "blocked", code
	return nil
}

type geospatialActionClientFixture struct {
	calls *[]string
	err   error
}

func (client *geospatialActionClientFixture) CreateMapElement(context.Context, string, string, MapElementCreateRequest) (MapElementMutationResult, error) {
	*client.calls = append(*client.calls, "create")
	return MapElementMutationResult{ID: "ELEMENT_REDACTED_NEW"}, client.err
}
func (client *geospatialActionClientFixture) UpdateWorkspaceMapElement(context.Context, string, string, string, MapElementUpdateRequest) (MapElementMutationResult, error) {
	*client.calls = append(*client.calls, "update")
	return MapElementMutationResult{ID: "ELEMENT_REDACTED_OLD"}, client.err
}
func (client *geospatialActionClientFixture) DeleteWorkspaceMapElement(context.Context, string, string, string, string) (MapElementDeleteResult, error) {
	*client.calls = append(*client.calls, "delete")
	return MapElementDeleteResult{AffectedTriStates: []MapElementTriState{{GroupID: "GROUP_REDACTED", TriState: "half"}}}, client.err
}

type geospatialActionTokenResolver struct{}

func (geospatialActionTokenResolver) ResolveToken(context.Context, connector.Instance) (string, error) {
	return "TOKEN_REDACTED", nil
}

func geospatialActionFixtureJob(t *testing.T, id, kind, expectedVersion string) GeospatialActionJob {
	t.Helper()
	point := GeoJSONFeature{Type: "Feature", Properties: json.RawMessage(`{"color":"#00ff00"}`),
		Geometry: GeoJSONGeometry{Type: "Point", Coordinates: json.RawMessage(`[120.5,30.25,15]`)}}
	var request any
	switch kind {
	case "map-element-create":
		request = MapElementCreateRequest{Name: "safety point", Resource: MapElementResource{Type: 0, Content: point}}
	case "map-element-update":
		name := "safety point updated"
		request = MapElementUpdateRequest{Name: &name, Content: &point}
	case "map-element-delete":
		request = map[string]string{"confirmation": "DELETE"}
	default:
		t.Fatalf("unknown kind %s", kind)
	}
	envelope, err := credentials.EncryptJSON(request, geospatialActionTestSecret,
		credentials.AAD("flighthub-geospatial-action", id, 41))
	if err != nil {
		t.Fatal(err)
	}
	envelopeJSON, _ := json.Marshal(envelope)
	scope, _ := json.Marshal(map[string]string{"projectUuid": "11111111-1111-4111-8111-111111111111", "projectName": "test"})
	policy := geospatialActionPolicies[kind]
	job := GeospatialActionJob{ID: id, ProjectID: 41, TeamID: 42, ConnectorInstanceID: 43, ActionKind: kind,
		CapabilityCode: policy.capability, FeatureFlag: policy.featureFlag, RequestEnvelope: envelopeJSON, Status: "queued",
		ConnectorStatus: "connected", Authorized: true, ActionEnabled: true, CapabilityVerified: true,
		Instance: connector.Instance{ID: 43, ProjectID: 41, ConnectorKey: ConnectorKey, Version: ConnectorVersion,
			DiscoveryScope: scope, CredentialEnvelope: json.RawMessage(`{"redacted":true}`)}}
	if kind != "map-element-create" {
		job.TargetResourceID = sql.NullInt64{Int64: 45, Valid: true}
		job.ExpectedRemoteVersion = sql.NullString{String: expectedVersion, Valid: true}
		job.TargetRemoteID, job.TargetRemoteVersion = "ELEMENT_REDACTED_OLD", expectedVersion
		job.TargetKind, job.TargetStatus = "map-element", "active"
	}
	return job
}

func geospatialActionEvent(job GeospatialActionJob) outbox.Event {
	payload, _ := json.Marshal(map[string]string{"jobId": job.ID})
	return outbox.Event{ProjectID: job.ProjectID, TeamID: job.TeamID, Payload: payload}
}

func TestGeospatialActionUnauthorizedOrStaleNeverCallsUpstream(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*GeospatialActionJob, *memoryGeospatialActionStore)
	}{
		{"rbac-denied", func(job *GeospatialActionJob, _ *memoryGeospatialActionStore) { job.Authorized = false }},
		{"feature-disabled", func(job *GeospatialActionJob, _ *memoryGeospatialActionStore) { job.ActionEnabled = false }},
		{"field-write-unverified", func(job *GeospatialActionJob, _ *memoryGeospatialActionStore) { job.CapabilityVerified = false }},
		{"connector-disabled", func(job *GeospatialActionJob, _ *memoryGeospatialActionStore) { job.ConnectorStatus = "disabled" }},
		{"target-out-of-scope", func(job *GeospatialActionJob, _ *memoryGeospatialActionStore) { job.TargetKind = "flight-area" }},
		{"stale-version", func(_ *GeospatialActionJob, store *memoryGeospatialActionStore) { store.targetVersion = "version-2" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			job := geospatialActionFixtureJob(t, "12345678-2222-4333-8444-555555555555", "map-element-update", "version-1")
			store := &memoryGeospatialActionStore{job: job, targetVersion: "version-1"}
			test.mutate(&store.job, store)
			calls := []string{}
			handler, _ := NewGeospatialActionHandler(store, &geospatialActionClientFixture{calls: &calls},
				geospatialActionTokenResolver{}, geospatialActionTestSecret)
			if err := handler.Handler(context.Background(), nil, geospatialActionEvent(store.job)); err != nil {
				t.Fatal(err)
			}
			if len(calls) != 0 || store.job.Status != "failed" {
				t.Fatalf("gate reached upstream: status=%s calls=%v", store.job.Status, calls)
			}
		})
	}
}

func TestGeospatialActionWorkerExecutesEachReleasedWriteExactlyOnce(t *testing.T) {
	want := map[string]string{"map-element-create": "create", "map-element-update": "update", "map-element-delete": "delete"}
	for kind, call := range want {
		t.Run(kind, func(t *testing.T) {
			job := geospatialActionFixtureJob(t, "12345678-2222-4333-8444-555555555555", kind, "version-1")
			store := &memoryGeospatialActionStore{job: job, targetVersion: "version-1"}
			calls := []string{}
			handler, _ := NewGeospatialActionHandler(store, &geospatialActionClientFixture{calls: &calls},
				geospatialActionTokenResolver{}, geospatialActionTestSecret)
			if err := handler.Handler(context.Background(), nil, geospatialActionEvent(job)); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, []string{call}) || !store.completed {
				t.Fatalf("calls=%v completed=%t", calls, store.completed)
			}
		})
	}
}

func TestGeospatialActionConcurrentOldVersionCannotOverwriteNewData(t *testing.T) {
	first := geospatialActionFixtureJob(t, "12345678-2222-4333-8444-555555555551", "map-element-update", "version-1")
	firstStore := &memoryGeospatialActionStore{job: first, targetVersion: "version-1"}
	calls := []string{}
	handler, _ := NewGeospatialActionHandler(firstStore, &geospatialActionClientFixture{calls: &calls},
		geospatialActionTokenResolver{}, geospatialActionTestSecret)
	if err := handler.Handler(context.Background(), nil, geospatialActionEvent(first)); err != nil {
		t.Fatal(err)
	}
	second := geospatialActionFixtureJob(t, "12345678-2222-4333-8444-555555555552", "map-element-update", "version-1")
	secondStore := &memoryGeospatialActionStore{job: second, targetVersion: firstStore.targetVersion}
	handler, _ = NewGeospatialActionHandler(secondStore, &geospatialActionClientFixture{calls: &calls},
		geospatialActionTokenResolver{}, geospatialActionTestSecret)
	if err := handler.Handler(context.Background(), nil, geospatialActionEvent(second)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"update"}) || secondStore.job.Status != "failed" || secondStore.job.TargetStatus != "version_conflict" {
		t.Fatalf("stale concurrent write status=%s/%s calls=%v", secondStore.job.Status, secondStore.job.TargetStatus, calls)
	}
}

func TestGeospatialActionUnknownWriteAndRestartNeverBlindlyRetry(t *testing.T) {
	job := geospatialActionFixtureJob(t, "12345678-2222-4333-8444-555555555555", "map-element-delete", "version-1")
	store := &memoryGeospatialActionStore{job: job, targetVersion: "version-1"}
	calls := []string{}
	handler, _ := NewGeospatialActionHandler(store, &geospatialActionClientFixture{calls: &calls,
		err: &APIError{SafeCode: "request_timeout", Retryable: true}}, geospatialActionTokenResolver{}, geospatialActionTestSecret)
	if err := handler.Handler(context.Background(), nil, geospatialActionEvent(job)); err != nil {
		t.Fatal(err)
	}
	if store.job.Status != "blocked" || !reflect.DeepEqual(calls, []string{"delete"}) {
		t.Fatalf("status=%s calls=%v", store.job.Status, calls)
	}
	store.job.Status, store.job.AttemptCount = "executing", 1
	if err := handler.Handler(context.Background(), nil, geospatialActionEvent(store.job)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"delete"}) {
		t.Fatalf("write was retried: %v", calls)
	}
}
