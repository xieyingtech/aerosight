package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
)

type memoryModelDeleteStore struct {
	job       ModelDeleteJob
	completed bool
}

func (store *memoryModelDeleteStore) Load(context.Context, int, string) (ModelDeleteJob, error) {
	return store.job, nil
}
func (store *memoryModelDeleteStore) Begin(context.Context, ModelDeleteJob) error {
	store.job.Status, store.job.AttemptCount = "executing", 1
	return nil
}
func (store *memoryModelDeleteStore) Complete(context.Context, ModelDeleteJob) error {
	store.job.Status, store.completed = "succeeded", true
	return nil
}
func (store *memoryModelDeleteStore) Fail(_ context.Context, _ ModelDeleteJob, code string) error {
	store.job.Status, store.job.LastErrorCode = "failed", code
	return nil
}
func (store *memoryModelDeleteStore) Block(_ context.Context, _ ModelDeleteJob, code string) error {
	store.job.Status, store.job.LastErrorCode = "blocked", code
	return nil
}

type modelDeleteClientFixture struct {
	deleteModelCalls, deleteResourceCalls, getModelCalls, getResourceCalls int
	deleteErr, getErr                                                      error
}

func (client *modelDeleteClientFixture) DeleteOpenModel(context.Context, string, string, string) error {
	client.deleteModelCalls++
	return client.deleteErr
}
func (client *modelDeleteClientFixture) DeleteOpenModelResource(context.Context, string, string, string) error {
	client.deleteResourceCalls++
	return client.deleteErr
}
func (client *modelDeleteClientFixture) GetOpenModel(context.Context, string, string, string) (OpenModel, error) {
	client.getModelCalls++
	return OpenModel{ModelUUID: "MODEL_REDACTED"}, client.getErr
}
func (client *modelDeleteClientFixture) GetOpenModelResource(context.Context, string, string, string) (OpenModelResource, error) {
	client.getResourceCalls++
	return OpenModelResource{ResourceUUID: "RESOURCE_REDACTED", Status: 1}, client.getErr
}

func readyModelDeleteJob(t *testing.T, action string) ModelDeleteJob {
	t.Helper()
	policy := modelDeletePolicies[action]
	job := ModelDeleteJob{ID: "00000000-0000-4000-8000-000000000001", ProjectID: 3, TeamID: 2,
		ConnectorInstanceID: 7, TargetResourceID: 91, RequestedByUserID: 5, ActionKind: action,
		CapabilityCode: policy.capability, FeatureFlag: policy.featureFlag, ExpectedRemoteVersion: "version-2",
		Status: "queued", TargetRemoteID: "REMOTE_REDACTED", TargetRemoteVersion: "version-2",
		TargetKind: policy.targetKind, TargetStatus: "active", ConnectorStatus: "connected",
		Authorized: true, ActionEnabled: true, CapabilityVerified: true, ApprovalValid: true,
		AssetID: sql.NullString{String: "31", Valid: true}, AssetStatus: sql.NullString{String: "available", Valid: true},
		DependentReferenceCount: 1, Instance: connector.Instance{ID: 7, ProjectID: 3, ConnectorKey: ConnectorKey,
			Version: ConnectorVersion, DiscoveryScope: json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"脱敏项目"}`)},
	}
	job.PreviewDigest = modelDeletePreviewHash(job)
	envelope, err := credentials.EncryptJSON(map[string]string{"confirmation": "DELETE"}, modelDeleteTestSecret,
		credentials.AAD("flighthub-model-delete", job.ID, job.ProjectID))
	if err != nil {
		t.Fatal(err)
	}
	job.RequestEnvelope, _ = json.Marshal(envelope)
	return job
}

const modelDeleteTestSecret = "abcdef0123456789abcdef0123456789"

func deleteEvent() outbox.Event {
	return outbox.Event{ProjectID: 3, Payload: json.RawMessage(`{"jobId":"00000000-0000-4000-8000-000000000001"}`)}
}

func TestModelDeleteGatesUnconfirmedAndCrossTenantJobsBeforeUpstream(t *testing.T) {
	for _, mutate := range []func(*ModelDeleteJob){
		func(job *ModelDeleteJob) { job.Authorized = false },
		func(job *ModelDeleteJob) { job.ActionEnabled = false },
		func(job *ModelDeleteJob) { job.CapabilityVerified = false },
		func(job *ModelDeleteJob) { job.ApprovalValid = false },
		func(job *ModelDeleteJob) { job.TargetKind = "" },
		func(job *ModelDeleteJob) { job.PreviewDigest = "0" + job.PreviewDigest[1:] },
	} {
		job := readyModelDeleteJob(t, "model-delete")
		mutate(&job)
		store := &memoryModelDeleteStore{job: job}
		client := &modelDeleteClientFixture{}
		handler, _ := NewModelDeleteHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, modelDeleteTestSecret)
		if err := handler.Handler(context.Background(), nil, deleteEvent()); err != nil {
			t.Fatal(err)
		}
		if client.deleteModelCalls+client.deleteResourceCalls+client.getModelCalls+client.getResourceCalls != 0 {
			t.Fatalf("gate failure called upstream: %#v", client)
		}
	}
}

func TestModelDeleteCallsDeleteOnceAndConfirmsMissingBeforeProjection(t *testing.T) {
	store := &memoryModelDeleteStore{job: readyModelDeleteJob(t, "model-delete")}
	client := &modelDeleteClientFixture{getErr: &APIError{SafeCode: "scope_not_found"}}
	handler, _ := NewModelDeleteHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, modelDeleteTestSecret)
	if err := handler.Handler(context.Background(), nil, deleteEvent()); err != nil {
		t.Fatal(err)
	}
	if client.deleteModelCalls != 1 || client.getModelCalls != 1 || !store.completed {
		t.Fatalf("client=%#v store=%#v", client, store)
	}
}

func TestModelResourceDeleteUsesDedicatedEndpoint(t *testing.T) {
	store := &memoryModelDeleteStore{job: readyModelDeleteJob(t, "model-resource-delete")}
	client := &modelDeleteClientFixture{getErr: &APIError{SafeCode: "scope_not_found"}}
	handler, _ := NewModelDeleteHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, modelDeleteTestSecret)
	if err := handler.Handler(context.Background(), nil, deleteEvent()); err != nil {
		t.Fatal(err)
	}
	if client.deleteResourceCalls != 1 || client.deleteModelCalls != 0 || client.getResourceCalls != 1 {
		t.Fatalf("client=%#v", client)
	}
}

func TestModelDeleteRestartAfterUnknownAttemptOnlyReadsRemoteState(t *testing.T) {
	job := readyModelDeleteJob(t, "model-delete")
	job.Status, job.AttemptCount = "executing", 1
	store := &memoryModelDeleteStore{job: job}
	client := &modelDeleteClientFixture{deleteErr: errors.New("must not be called"), getErr: &APIError{SafeCode: "scope_not_found"}}
	handler, _ := NewModelDeleteHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, modelDeleteTestSecret)
	if err := handler.Handler(context.Background(), nil, deleteEvent()); err != nil {
		t.Fatal(err)
	}
	if client.deleteModelCalls != 0 || client.getModelCalls != 1 || !store.completed {
		t.Fatalf("client=%#v store=%#v", client, store)
	}
}

func TestModelDeleteRestartWithCrossTenantTargetStillMakesZeroUpstreamCalls(t *testing.T) {
	job := readyModelDeleteJob(t, "model-delete")
	job.Status, job.AttemptCount, job.TargetKind, job.TargetRemoteID = "executing", 1, "", ""
	store := &memoryModelDeleteStore{job: job}
	client := &modelDeleteClientFixture{}
	handler, _ := NewModelDeleteHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, modelDeleteTestSecret)
	if err := handler.Handler(context.Background(), nil, deleteEvent()); err != nil {
		t.Fatal(err)
	}
	if client.deleteModelCalls+client.getModelCalls != 0 || store.job.Status != "blocked" {
		t.Fatalf("client=%#v store=%#v", client, store)
	}
}

func TestModelDeleteUnknownReconciliationBlocksWithoutSecondDelete(t *testing.T) {
	store := &memoryModelDeleteStore{job: readyModelDeleteJob(t, "model-delete")}
	client := &modelDeleteClientFixture{deleteErr: &APIError{SafeCode: "upstream_unavailable", Retryable: true},
		getErr: &APIError{SafeCode: "upstream_unavailable", Retryable: true}}
	handler, _ := NewModelDeleteHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, modelDeleteTestSecret)
	if err := handler.Handler(context.Background(), nil, deleteEvent()); err != nil {
		t.Fatal(err)
	}
	if client.deleteModelCalls != 1 || client.getModelCalls != 1 || store.job.Status != "blocked" {
		t.Fatalf("client=%#v store=%#v", client, store)
	}
}
