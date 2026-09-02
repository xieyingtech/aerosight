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

const liveActionTestSecret = "abcdef0123456789abcdef0123456789"

type memoryLiveActionStore struct {
	job       LiveActionJob
	completed bool
}

func (store *memoryLiveActionStore) Load(context.Context, int, string) (LiveActionJob, error) {
	return store.job, nil
}
func (store *memoryLiveActionStore) Begin(context.Context, LiveActionJob) error {
	if store.job.AttemptCount != 0 {
		return errors.New("duplicate unsafe attempt")
	}
	store.job.AttemptCount, store.job.Status = 1, "executing"
	return nil
}
func (store *memoryLiveActionStore) Complete(_ context.Context, _ LiveActionJob, _ LiveActionRequest, _ string) error {
	store.job.Status, store.completed = "succeeded", true
	return nil
}
func (store *memoryLiveActionStore) Fail(_ context.Context, _ LiveActionJob, code string) error {
	store.job.Status = "failed"
	store.job.TargetStatus = code
	return nil
}
func (store *memoryLiveActionStore) Block(_ context.Context, _ LiveActionJob, code string) error {
	store.job.Status = "blocked"
	store.job.TargetStatus = code
	return nil
}

type liveActionClientFixture struct {
	calls *[]string
	err   error
}

func (client *liveActionClientFixture) SetStreamQuality(context.Context, string, string, StreamQualityRequest) error {
	*client.calls = append(*client.calls, "quality")
	return client.err
}
func (client *liveActionClientFixture) CreateStreamConverter(context.Context, string, string, StreamConverterCreateRequest) (StreamConverterCreateResult, error) {
	*client.calls = append(*client.calls, "create")
	return StreamConverterCreateResult{ID: "CONVERTER_REDACTED"}, client.err
}
func (client *liveActionClientFixture) SetStreamConverterEnabled(context.Context, string, string, string, bool) error {
	*client.calls = append(*client.calls, "toggle")
	return client.err
}
func (client *liveActionClientFixture) DeleteStreamConverter(context.Context, string, string, string) error {
	*client.calls = append(*client.calls, "delete")
	return client.err
}

type liveActionTokenResolver struct{}

func (liveActionTokenResolver) ResolveToken(context.Context, connector.Instance) (string, error) {
	return "TOKEN_REDACTED", nil
}

func liveActionFixtureJob(t *testing.T, kind string) LiveActionJob {
	t.Helper()
	id := "12345678-2222-4333-8444-555555555555"
	request := LiveActionRequest{CameraIndex: "0", QualityType: LiveQualityAdaptive, Name: "converter", Schema: "rtmp", Enabled: true}
	request.SchemaOption.URL = "rtmp://media.invalid/live"
	envelope, err := credentials.EncryptJSON(request, liveActionTestSecret, credentials.AAD("flighthub-live-action", id, 41))
	if err != nil {
		t.Fatal(err)
	}
	envelopeJSON, _ := json.Marshal(envelope)
	scope, _ := json.Marshal(map[string]string{"projectUuid": "11111111-1111-4111-8111-111111111111", "projectName": "test"})
	policy := liveActionPolicies[kind]
	job := LiveActionJob{ID: id, ProjectID: 41, TeamID: 42, ConnectorInstanceID: 43, ActionKind: kind,
		CapabilityCode: policy.capability, FeatureFlag: policy.featureFlag, RequestEnvelope: envelopeJSON, Status: "queued",
		ConnectorStatus: "connected", Authorized: true, ActionEnabled: true, CapabilityVerified: true,
		Instance: connector.Instance{ID: 43, ProjectID: 41, ConnectorKey: ConnectorKey, Version: ConnectorVersion,
			DiscoveryScope: scope, CredentialEnvelope: json.RawMessage(`{"redacted":true}`)}}
	if kind == "live-quality-set" || kind == "live-converter-create" {
		job.DeviceID = sql.NullInt64{Int64: 44, Valid: true}
		job.DeviceExternalID = "DEVICE_REDACTED"
	} else {
		job.TargetResourceID = sql.NullInt64{Int64: 45, Valid: true}
		job.TargetRemoteID, job.TargetKind, job.TargetStatus = "CONVERTER_REDACTED", "stream-converter", "active"
	}
	return job
}

func liveActionEvent(job LiveActionJob) outbox.Event {
	payload, _ := json.Marshal(map[string]string{"jobId": job.ID})
	return outbox.Event{ProjectID: job.ProjectID, TeamID: job.TeamID, Payload: payload}
}

func TestLiveActionUnauthorizedOrUnacceptedNeverCallsUpstream(t *testing.T) {
	t.Run("rbac-denied-by-store", func(t *testing.T) {
		job := liveActionFixtureJob(t, "live-quality-set")
		job.Authorized = false
		store := &memoryLiveActionStore{job: job}
		calls := []string{}
		handler, _ := NewLiveActionHandler(store, &liveActionClientFixture{calls: &calls}, liveActionTokenResolver{}, liveActionTestSecret)
		if err := handler.Handler(context.Background(), nil, liveActionEvent(job)); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 0 {
			t.Fatalf("upstream called after RBAC denial: %v", calls)
		}
	})
	for _, test := range []struct {
		name   string
		mutate func(*LiveActionJob)
	}{
		{"feature-disabled", func(job *LiveActionJob) { job.ActionEnabled = false }},
		{"field-write-unverified", func(job *LiveActionJob) { job.CapabilityVerified = false }},
		{"connector-disabled", func(job *LiveActionJob) { job.ConnectorStatus = "disabled" }},
		{"device-out-of-scope", func(job *LiveActionJob) { job.DeviceExternalID = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			job := liveActionFixtureJob(t, "live-quality-set")
			test.mutate(&job)
			store := &memoryLiveActionStore{job: job}
			calls := []string{}
			handler, _ := NewLiveActionHandler(store, &liveActionClientFixture{calls: &calls}, liveActionTokenResolver{}, liveActionTestSecret)
			if err := handler.Handler(context.Background(), nil, liveActionEvent(job)); err != nil {
				t.Fatal(err)
			}
			if len(calls) != 0 {
				t.Fatalf("upstream called behind failed gate: %v", calls)
			}
			if store.job.Status != "failed" {
				t.Fatalf("status=%s, want failed", store.job.Status)
			}
		})
	}
}

func TestLiveActionWorkerExecutesEachReleasedWriteExactlyOnce(t *testing.T) {
	want := map[string]string{"live-quality-set": "quality", "live-converter-create": "create",
		"live-converter-toggle": "toggle", "live-converter-delete": "delete"}
	for kind, call := range want {
		t.Run(kind, func(t *testing.T) {
			job := liveActionFixtureJob(t, kind)
			store := &memoryLiveActionStore{job: job}
			calls := []string{}
			handler, _ := NewLiveActionHandler(store, &liveActionClientFixture{calls: &calls}, liveActionTokenResolver{}, liveActionTestSecret)
			if err := handler.Handler(context.Background(), nil, liveActionEvent(job)); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, []string{call}) {
				t.Fatalf("calls=%v", calls)
			}
			if !store.completed {
				t.Fatal("action not completed")
			}
		})
	}
}

func TestLiveActionUnknownWriteAndRestartNeverBlindlyRetry(t *testing.T) {
	job := liveActionFixtureJob(t, "live-converter-toggle")
	store := &memoryLiveActionStore{job: job}
	calls := []string{}
	handler, _ := NewLiveActionHandler(store, &liveActionClientFixture{calls: &calls, err: &APIError{SafeCode: "request_timeout", Retryable: true}},
		liveActionTokenResolver{}, liveActionTestSecret)
	if err := handler.Handler(context.Background(), nil, liveActionEvent(job)); err != nil {
		t.Fatal(err)
	}
	if store.job.Status != "blocked" || !reflect.DeepEqual(calls, []string{"toggle"}) {
		t.Fatalf("status=%s calls=%v", store.job.Status, calls)
	}
	store.job.Status, store.job.AttemptCount = "executing", 1
	if err := handler.Handler(context.Background(), nil, liveActionEvent(job)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"toggle"}) {
		t.Fatalf("write was retried: %v", calls)
	}
}
