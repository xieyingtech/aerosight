package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/outbox"
)

type memoryModelJobStore struct{ job ModelJob }

func (store *memoryModelJobStore) Create(_ context.Context, request ModelJobCreateRequest, envelope json.RawMessage, digest, name string) (ModelJob, error) {
	if store.job.ID != "" {
		if store.job.RequestDigest != digest {
			return ModelJob{}, &APIError{SafeCode: "idempotency_conflict"}
		}
		return store.job, nil
	}
	store.job = ModelJob{ID: "JOB_REDACTED", ProjectID: request.ProjectID, TeamID: 2, ConnectorInstanceID: request.ConnectorInstanceID,
		RequestedByUserID: request.RequestedByUserID, ActionKind: request.ActionKind, IdempotencyKey: request.IdempotencyKey,
		RequestDigest: digest, RequestEnvelope: envelope, ReconciliationName: name, Status: "queued", Stage: "queued",
		Instance: connector.Instance{ID: request.ConnectorInstanceID, ProjectID: request.ProjectID, ConnectorKey: ConnectorKey, Version: ConnectorVersion,
			DiscoveryScope: json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"脱敏项目"}`)}}
	return store.job, nil
}
func (store *memoryModelJobStore) Load(context.Context, int, string) (ModelJob, error) {
	return store.job, nil
}
func (store *memoryModelJobStore) BeginSubmit(context.Context, ModelJob) error {
	store.job.Status = "reconciling"
	store.job.SubmitAttempts = 1
	store.job.Stage = "submitted"
	return nil
}
func (store *memoryModelJobStore) BindRemoteIDs(_ context.Context, _ ModelJob, ids []string) error {
	store.job.RemoteIDs = append([]string(nil), ids...)
	store.job.Stage = "reconciling"
	return nil
}
func (store *memoryModelJobStore) RecordProgress(_ context.Context, _ ModelJob, progress int, stage, code string) error {
	store.job.Progress = progress
	store.job.Stage = stage
	store.job.LastErrorCode = code
	store.job.ReconciliationCount++
	return nil
}
func (store *memoryModelJobStore) Complete(_ context.Context, _ ModelJob, ids []string) error {
	store.job.Status = "succeeded"
	store.job.Progress = 100
	store.job.Stage = "completed"
	store.job.AssetIDs = append([]string(nil), ids...)
	return nil
}
func (store *memoryModelJobStore) Fail(_ context.Context, _ ModelJob, code string, blocked bool) error {
	store.job.Status = "failed"
	if blocked {
		store.job.Status = "blocked"
	}
	store.job.LastErrorCode = code
	return nil
}

type modelJobClientFixture struct {
	createCalls, startCalls, stopCalls int
	createResult                       ModelReconstructionResult
	createErr                          error
	models                             []ModelSummary
	startResult                        OpenModelStartResult
	startErr                           error
	stopErr                            error
	running                            []OpenModel
	details                            map[string]OpenModel
	resources                          map[string]OpenModelResource
}

func (client *modelJobClientFixture) CreateModelReconstruction(context.Context, string, string, ModelReconstructionRequest) (ModelReconstructionResult, error) {
	client.createCalls++
	return client.createResult, client.createErr
}
func (client *modelJobClientFixture) ListModels(context.Context, string, string) ([]ModelSummary, error) {
	return client.models, nil
}
func (client *modelJobClientFixture) StartOpenModelReconstruction(context.Context, string, string, OpenModelStartRequest) (OpenModelStartResult, error) {
	client.startCalls++
	return client.startResult, client.startErr
}
func (client *modelJobClientFixture) StopOpenModelReconstruction(context.Context, string, string, string) error {
	client.stopCalls++
	return client.stopErr
}
func (client *modelJobClientFixture) ListRunningOpenModels(context.Context, string, string) ([]OpenModel, error) {
	return client.running, nil
}
func (client *modelJobClientFixture) GetOpenModel(_ context.Context, _, _, id string) (OpenModel, error) {
	item, ok := client.details[id]
	if !ok {
		return OpenModel{}, errors.New("missing")
	}
	return item, nil
}
func (client *modelJobClientFixture) GetOpenModelResource(_ context.Context, _, _, id string) (OpenModelResource, error) {
	item, ok := client.resources[id]
	if !ok {
		return OpenModelResource{}, errors.New("missing")
	}
	return item, nil
}

type modelJobProjectorFixture struct{ polls []ModelCatalogPoll }

func (fixture *modelJobProjectorFixture) ApplyModelCatalog(_ context.Context, _ connector.Instance, poll ModelCatalogPoll) error {
	fixture.polls = append(fixture.polls, poll)
	return nil
}

func modelJobEvent() outbox.Event {
	return outbox.Event{ProjectID: 3, Payload: json.RawMessage(`{"jobId":"JOB_REDACTED"}`)}
}
func traditionalModelJobRequest() ModelJobCreateRequest {
	return ModelJobCreateRequest{ProjectID: 3, ConnectorInstanceID: 7, RequestedByUserID: 5, ActionKind: "traditional-create", IdempotencyKey: "model-job-key", Payload: ModelJobPayload{Traditional: &ModelReconstructionRequest{Name: "脱敏模型", ReconstructionTypes: []ModelFileType{ModelFile3D}, SimplifiedFactor: 0.2, TaskFolderID: 9, WKT: "EPSG:4326", QualityLevel: "medium", ReconstructionMode: "normal", GenerateModelFormats: []string{"b3dm"}}}}
}

func TestModelJobUnknownCreateResponseRecoversWithoutResubmissionAfterRestart(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	store := &memoryModelJobStore{}
	client := &modelJobClientFixture{createErr: &APIError{SafeCode: "upstream_unavailable", Retryable: true}}
	projector := &modelJobProjectorFixture{}
	handler, err := NewModelJobHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, projector, flightProjectorTestSecret, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	job, err := handler.Enqueue(context.Background(), traditionalModelJobRequest())
	if err != nil {
		t.Fatal(err)
	}
	event := modelJobEvent()
	event.Payload = json.RawMessage(`{"jobId":"` + job.ID + `"}`)
	if err := handler.Handle(context.Background(), event); !IsSafeCode(err, "upstream_unavailable") || client.createCalls != 1 || store.job.Status != "reconciling" {
		t.Fatalf("err=%v calls=%d job=%#v", err, client.createCalls, store.job)
	}
	client.createErr = nil
	client.models = []ModelSummary{{ID: 71, Name: store.job.ReconciliationName, FileType: ModelFile3D, Size: 10, CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli()}}
	restarted, _ := NewModelJobHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, projector, flightProjectorTestSecret, func() time.Time { return now })
	if err := restarted.Handle(context.Background(), event); err != nil || client.createCalls != 1 || store.job.Status != "succeeded" || len(projector.polls) != 1 || store.job.AssetIDs[0] != strconv.FormatInt(71, 10) {
		t.Fatalf("err=%v calls=%d job=%#v polls=%d", err, client.createCalls, store.job, len(projector.polls))
	}
}

func TestOpenModelJobResumesProgressAndAssociatesCompletedAsset(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC)
	store := &memoryModelJobStore{}
	task := &OpenModelTaskResult{UUID: "MODEL_REDACTED", Status: OpenModelRequestingResource}
	client := &modelJobClientFixture{startResult: OpenModelStartResult{Model3D: task}, details: map[string]OpenModel{}, resources: map[string]OpenModelResource{}}
	projector := &modelJobProjectorFixture{}
	handler, _ := NewModelJobHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, projector, flightProjectorTestSecret, func() time.Time { return now })
	request := ModelJobCreateRequest{ProjectID: 3, ConnectorInstanceID: 7, RequestedByUserID: 5, ActionKind: "open-start", IdempotencyKey: "open-model-key", Payload: ModelJobPayload{OpenStart: &OpenModelStartRequest{ResourceUUID: "RESOURCE_REDACTED", Parameter3D: `{"quality":"standard"}`}}}
	job, err := handler.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	event := modelJobEvent()
	event.Payload = json.RawMessage(`{"jobId":"` + job.ID + `"}`)
	client.details[task.UUID] = OpenModel{ResourceUUID: "RESOURCE_REDACTED", ModelUUID: task.UUID, ModelType: OpenModel3D, ModelStatus: OpenModelReconstructionExecuting, ReconstructionProgress: 40, ZipStatus: OpenModelZipRunning, ZipProgress: 10}
	if err := handler.Handle(context.Background(), event); !IsSafeCode(err, "model_job_pending") || client.startCalls != 1 || store.job.Progress != 40 {
		t.Fatalf("err=%v job=%#v", err, store.job)
	}
	client.details[task.UUID] = OpenModel{ResourceUUID: "RESOURCE_REDACTED", ModelUUID: task.UUID, ModelType: OpenModel3D, ModelStatus: OpenModelReconstructionSucceeded, ModelSize: 20, ReconstructionProgress: 100, ZipStatus: OpenModelZipFinished, ZipProgress: 100}
	client.resources["RESOURCE_REDACTED"] = OpenModelResource{ResourceUUID: "RESOURCE_REDACTED", Status: 1, Size: 30, FileNames: []string{"synthetic.jpg"}}
	restarted, _ := NewModelJobHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, projector, flightProjectorTestSecret, func() time.Time { return now })
	if err := restarted.Handle(context.Background(), event); err != nil || client.startCalls != 1 || store.job.Status != "succeeded" || len(projector.polls) != 1 {
		t.Fatalf("err=%v calls=%d job=%#v", err, client.startCalls, store.job)
	}
}

func TestOpenModelStartUnknownResponseReconcilesEveryRequestedTypeWithoutResubmission(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 45, 0, 0, time.UTC)
	store := &memoryModelJobStore{}
	client := &modelJobClientFixture{
		startErr:  &APIError{SafeCode: "upstream_unavailable", Retryable: true},
		details:   map[string]OpenModel{},
		resources: map[string]OpenModelResource{},
	}
	projector := &modelJobProjectorFixture{}
	handler, _ := NewModelJobHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, projector, flightProjectorTestSecret, func() time.Time { return now })
	request := ModelJobCreateRequest{
		ProjectID: 3, ConnectorInstanceID: 7, RequestedByUserID: 5, ActionKind: "open-start", IdempotencyKey: "unknown-open-key",
		Payload: ModelJobPayload{OpenStart: &OpenModelStartRequest{
			ResourceUUID: "RESOURCE_MULTI_REDACTED", Parameter2D: `{"quality":"standard"}`, Parameter3D: `{"quality":"standard"}`,
		}},
	}
	job, err := handler.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	event := modelJobEvent()
	event.Payload = json.RawMessage(`{"jobId":"` + job.ID + `"}`)
	if err := handler.Handle(context.Background(), event); !IsSafeCode(err, "upstream_unavailable") || client.startCalls != 1 || len(store.job.RemoteIDs) != 0 {
		t.Fatalf("err=%v calls=%d job=%#v", err, client.startCalls, store.job)
	}
	client.startErr = nil
	client.running = []OpenModel{
		{ResourceUUID: "RESOURCE_MULTI_REDACTED", ModelUUID: "MODEL_2D_REDACTED", ModelType: OpenModel2D, ModelStatus: OpenModelReconstructionSucceeded, ReconstructionProgress: 100, ZipStatus: OpenModelZipFinished, ZipProgress: 100},
		{ResourceUUID: "RESOURCE_MULTI_REDACTED", ModelUUID: "MODEL_3D_REDACTED", ModelType: OpenModel3D, ModelStatus: OpenModelReconstructionSucceeded, ReconstructionProgress: 100, ZipStatus: OpenModelZipFinished, ZipProgress: 100},
		{ResourceUUID: "RESOURCE_OTHER_REDACTED", ModelUUID: "MODEL_OTHER_REDACTED", ModelType: OpenModel3DGS, ModelStatus: OpenModelReconstructionSucceeded, ReconstructionProgress: 100, ZipStatus: OpenModelZipFinished, ZipProgress: 100},
	}
	for _, item := range client.running[:2] {
		client.details[item.ModelUUID] = item
		client.resources[item.ResourceUUID] = OpenModelResource{ResourceUUID: item.ResourceUUID, Status: 1, Size: 30, FileNames: []string{"synthetic.jpg"}}
	}
	restarted, _ := NewModelJobHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, projector, flightProjectorTestSecret, func() time.Time { return now })
	if err := restarted.Handle(context.Background(), event); err != nil || client.startCalls != 1 || store.job.Status != "succeeded" || len(store.job.RemoteIDs) != 2 || len(projector.polls) != 1 {
		t.Fatalf("err=%v calls=%d job=%#v polls=%d", err, client.startCalls, store.job, len(projector.polls))
	}
}

func TestOpenModelJobPersistsTerminalFailureWithoutRetry(t *testing.T) {
	now := time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)
	store := &memoryModelJobStore{}
	task := &OpenModelTaskResult{UUID: "MODEL_FAILED_REDACTED", Status: OpenModelRequestingResource}
	client := &modelJobClientFixture{startResult: OpenModelStartResult{Model3D: task}, details: map[string]OpenModel{task.UUID: {ResourceUUID: "RESOURCE_REDACTED", ModelUUID: task.UUID, ModelType: OpenModel3D, ModelStatus: OpenModelReconstructionFailed, ReconstructionProgress: 55, ErrorCode: 1, ZipStatus: OpenModelZipInitial}}, resources: map[string]OpenModelResource{}}
	handler, _ := NewModelJobHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, &modelJobProjectorFixture{}, flightProjectorTestSecret, func() time.Time { return now })
	request := ModelJobCreateRequest{ProjectID: 3, ConnectorInstanceID: 7, RequestedByUserID: 5, ActionKind: "open-start", IdempotencyKey: "failed-model-key", Payload: ModelJobPayload{OpenStart: &OpenModelStartRequest{ResourceUUID: "RESOURCE_REDACTED", Parameter3D: `{"quality":"standard"}`}}}
	job, err := handler.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	event := modelJobEvent()
	event.Payload = json.RawMessage(`{"jobId":"` + job.ID + `"}`)
	if err := handler.Handle(context.Background(), event); err != nil || store.job.Status != "failed" || client.startCalls != 1 {
		t.Fatalf("err=%v job=%#v", err, store.job)
	}
	if err := handler.Handle(context.Background(), event); err != nil || client.startCalls != 1 {
		t.Fatalf("terminal retry err=%v calls=%d", err, client.startCalls)
	}
}

func TestOpenModelStopUnknownResponseRecoversWithoutResubmission(t *testing.T) {
	now := time.Date(2026, 9, 2, 11, 30, 0, 0, time.UTC)
	store := &memoryModelJobStore{}
	modelID := "MODEL_STOP_REDACTED"
	client := &modelJobClientFixture{
		stopErr:   &APIError{SafeCode: "upstream_unavailable", Retryable: true},
		details:   map[string]OpenModel{},
		resources: map[string]OpenModelResource{},
	}
	projector := &modelJobProjectorFixture{}
	handler, _ := NewModelJobHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, projector, flightProjectorTestSecret, func() time.Time { return now })
	request := ModelJobCreateRequest{ProjectID: 3, ConnectorInstanceID: 7, RequestedByUserID: 5, ActionKind: "open-stop", IdempotencyKey: "stop-model-key", Payload: ModelJobPayload{ModelUUID: modelID}}
	job, err := handler.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	event := modelJobEvent()
	event.Payload = json.RawMessage(`{"jobId":"` + job.ID + `"}`)
	if err := handler.Handle(context.Background(), event); !IsSafeCode(err, "upstream_unavailable") || client.stopCalls != 1 || len(store.job.RemoteIDs) != 1 || store.job.RemoteIDs[0] != modelID {
		t.Fatalf("err=%v calls=%d job=%#v", err, client.stopCalls, store.job)
	}
	client.stopErr = nil
	client.details[modelID] = OpenModel{ResourceUUID: "RESOURCE_STOP_REDACTED", ModelUUID: modelID, ModelType: OpenModel3D, ModelStatus: OpenModelReconstructionCanceled, ReconstructionProgress: 45, ZipStatus: OpenModelZipInitial}
	restarted, _ := NewModelJobHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, projector, flightProjectorTestSecret, func() time.Time { return now })
	if err := restarted.Handle(context.Background(), event); err != nil || client.stopCalls != 1 || store.job.Status != "succeeded" || len(projector.polls) != 1 {
		t.Fatalf("err=%v calls=%d job=%#v polls=%d", err, client.stopCalls, store.job, len(projector.polls))
	}
}

func TestOpenModelStartCancellationIsTerminalFailure(t *testing.T) {
	now := time.Date(2026, 9, 2, 11, 45, 0, 0, time.UTC)
	store := &memoryModelJobStore{}
	modelID := "MODEL_CANCELED_REDACTED"
	client := &modelJobClientFixture{
		startResult: OpenModelStartResult{Model3D: &OpenModelTaskResult{UUID: modelID, Status: OpenModelRequestingResource}},
		details: map[string]OpenModel{modelID: {
			ResourceUUID: "RESOURCE_CANCELED_REDACTED", ModelUUID: modelID, ModelType: OpenModel3D,
			ModelStatus: OpenModelReconstructionCanceled, ReconstructionProgress: 45, ZipStatus: OpenModelZipInitial,
		}},
		resources: map[string]OpenModelResource{},
	}
	handler, _ := NewModelJobHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, &modelJobProjectorFixture{}, flightProjectorTestSecret, func() time.Time { return now })
	request := ModelJobCreateRequest{ProjectID: 3, ConnectorInstanceID: 7, RequestedByUserID: 5, ActionKind: "open-start", IdempotencyKey: "cancel-model-key", Payload: ModelJobPayload{OpenStart: &OpenModelStartRequest{ResourceUUID: "RESOURCE_CANCELED_REDACTED", Parameter3D: `{"quality":"standard"}`}}}
	job, err := handler.Enqueue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	event := modelJobEvent()
	event.Payload = json.RawMessage(`{"jobId":"` + job.ID + `"}`)
	if err := handler.Handle(context.Background(), event); err != nil || store.job.Status != "failed" || store.job.LastErrorCode != "model_reconstruction_canceled" {
		t.Fatalf("err=%v job=%#v", err, store.job)
	}
}

func TestModelJobFinalOutboxAttemptPersistsBlockedState(t *testing.T) {
	store := &memoryModelJobStore{}
	client := &modelJobClientFixture{createErr: &APIError{SafeCode: "upstream_unavailable", Retryable: true}}
	handler, _ := NewModelJobHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, &modelJobProjectorFixture{}, flightProjectorTestSecret, nil)
	job, err := handler.Enqueue(context.Background(), traditionalModelJobRequest())
	if err != nil {
		t.Fatal(err)
	}
	event := modelJobEvent()
	event.Payload = json.RawMessage(`{"jobId":"` + job.ID + `"}`)
	event.Attempts, event.MaxAttempts = 32, 32
	if err := handler.Handle(context.Background(), event); err != nil || store.job.Status != "blocked" || store.job.LastErrorCode != "model_reconciliation_exhausted" || client.createCalls != 1 {
		t.Fatalf("err=%v job=%#v calls=%d", err, store.job, client.createCalls)
	}
}

func TestSQLModelJobEnqueueIsIdempotentOutboxedAndSecretSafe(t *testing.T) {
	databaseURL := os.Getenv("AEROSIGHT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AEROSIGHT_TEST_DATABASE_URL is not configured")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var userID, teamID, projectID int
	var adapterID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into users(name,email) values($1,$2) returning id`, "model-job-user", fmt.Sprintf("model-job-%d@example.test", suffix)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("model-job-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID)
		_, _ = database.ExecContext(context.Background(), `delete from users where id=$1`, userID)
	})
	if _, err := database.ExecContext(ctx, `insert into team_members(team_id,user_id,role) values($1,$2,'owner')`, teamID, userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name,created_by_user_id) values($1,$2,$3) returning id`, teamID, fmt.Sprintf("model-job-%d", suffix), userID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `insert into project_feature_flags(project_id,flighthub_action_flags_json) values($1,'{"model.write":true}'::jsonb)`, projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key=$1 and version=$2`, ConnectorKey, ConnectorVersion).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,discovery_scope_json,credential_envelope_json)
		values($1,$2,$3,'dji-flighthub2',$4,'2','connected',$5,'{}'::jsonb) returning id`, projectID, teamID, fmt.Sprintf("model-job-%d", suffix), definitionID,
		fmt.Sprintf(`{"projectUuid":"%s","projectName":"脱敏项目"}`, runtimeProjectUUID)).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	store := NewSQLModelJobStore(database)
	handler, _ := NewModelJobHandler(store, &modelJobClientFixture{}, tokenResolverFixture{token: "TOKEN_REDACTED"}, &modelJobProjectorFixture{}, flightProjectorTestSecret, nil)
	request := traditionalModelJobRequest()
	request.ProjectID, request.ConnectorInstanceID, request.RequestedByUserID = projectID, adapterID, userID
	first, err := handler.Enqueue(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := handler.Enqueue(ctx, request)
	if err != nil || second.ID != first.ID {
		t.Fatalf("first=%#v second=%#v err=%v", first, second, err)
	}
	changed := request
	changed.Payload.Traditional = &ModelReconstructionRequest{}
	*changed.Payload.Traditional = *request.Payload.Traditional
	changed.Payload.Traditional.Name = "changed"
	if _, err := handler.Enqueue(ctx, changed); !IsSafeCode(err, "idempotency_conflict") {
		t.Fatalf("conflict err=%v", err)
	}
	var events int
	var persisted string
	if err := database.QueryRowContext(ctx, `select count(*) from outbox_events where event_id=$1`, "flighthub-model-job:"+first.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select request_envelope_json::text from connector_model_jobs where id=$1::uuid and project_id=$2`, first.ID, projectID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if events != 1 || strings.Contains(persisted, "EPSG") || strings.Contains(persisted, "脱敏模型") {
		t.Fatalf("events=%d secret persisted=%t", events, strings.Contains(persisted, "EPSG"))
	}
	clock := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	projector := NewSQLFlightCatalogProjector(database, &telemetryIngestorFixture{}, func() time.Time { return clock }, 30*time.Minute, flightProjectorTestSecret)
	sink, err := NewSQLResourceStreamSink(&telemetryIngestorFixture{}, connector.NewSQLResourceRepository(database), &freshnessProjectorFixture{}, &healthProjectorFixture{}, projector)
	if err != nil {
		t.Fatal(err)
	}
	model := ModelSummary{ID: 71, Name: first.ReconciliationName, FileType: ModelFile3D, Size: 20, CreatedAt: clock.UnixMilli(), UpdatedAt: clock.UnixMilli()}
	if err := sink.ApplyModelCatalog(ctx, connector.Instance{ID: adapterID, ProjectID: projectID}, ModelCatalogPoll{Models: []ModelSummary{model}, ReceivedAt: clock}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `update connector_model_jobs set status='reconciling',submit_attempt_count=1,remote_ids_json='["71"]'::jsonb where id=$1::uuid`, first.ID); err != nil {
		t.Fatal(err)
	}
	first, err = store.Load(ctx, projectID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, first, []string{"71"}); err != nil {
		t.Fatal(err)
	}
	var linked bool
	if err := database.QueryRowContext(ctx, `select (job.asset_ids_json->>0)=resource.canonical_target_id from connector_model_jobs job
		join connector_remote_resources resource on resource.project_id=job.project_id and resource.connector_instance_id=job.connector_instance_id
		and resource.resource_kind='model' and resource.remote_id='71' where job.id=$1::uuid and job.status='succeeded'`, first.ID).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if !linked {
		t.Fatal("completed model job did not retain its canonical asset")
	}
	if _, err := database.ExecContext(ctx, `update project_feature_flags set flighthub_action_flags_json='{}'::jsonb where project_id=$1`, projectID); err != nil {
		t.Fatal(err)
	}
	request.IdempotencyKey = "disabled-model-key"
	if _, err := handler.Enqueue(ctx, request); !IsSafeCode(err, "scope_forbidden") {
		t.Fatalf("disabled enqueue err=%v", err)
	}
}
