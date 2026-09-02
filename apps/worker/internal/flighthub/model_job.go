package flighthub

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
)

const ModelJobEventType = "flighthub.model_job.requested"

type ModelJobPayload struct {
	Traditional *ModelReconstructionRequest `json:"traditional,omitempty"`
	OpenStart   *OpenModelStartRequest      `json:"openStart,omitempty"`
	ModelUUID   string                      `json:"modelUuid,omitempty"`
}

type ModelJobCreateRequest struct {
	ProjectID           int
	ConnectorInstanceID int64
	RequestedByUserID   int
	ActionKind          string
	IdempotencyKey      string
	Payload             ModelJobPayload
}

type ModelJob struct {
	ID, ActionKind, IdempotencyKey, RequestDigest, ReconciliationName, Status, Stage, LastErrorCode string
	ProjectID, TeamID, RequestedByUserID, Progress, SubmitAttempts, ReconciliationCount             int
	ConnectorInstanceID                                                                             int64
	RequestEnvelope                                                                                 json.RawMessage
	RemoteIDs, AssetIDs                                                                             []string
	Instance                                                                                        connector.Instance
}

type ModelJobStore interface {
	Create(context.Context, ModelJobCreateRequest, json.RawMessage, string, string) (ModelJob, error)
	Load(context.Context, int, string) (ModelJob, error)
	BeginSubmit(context.Context, ModelJob) error
	BindRemoteIDs(context.Context, ModelJob, []string) error
	RecordProgress(context.Context, ModelJob, int, string, string) error
	Complete(context.Context, ModelJob, []string) error
	Fail(context.Context, ModelJob, string, bool) error
}

type SQLModelJobStore struct{ db *sql.DB }

func NewSQLModelJobStore(database *sql.DB) *SQLModelJobStore { return &SQLModelJobStore{db: database} }

func (store *SQLModelJobStore) Create(ctx context.Context, request ModelJobCreateRequest, envelope json.RawMessage, digest, reconciliationName string) (job ModelJob, returnedErr error) {
	if store == nil || store.db == nil {
		return job, errors.New("FlightHub model job store is unavailable")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return job, err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	var teamID int
	err = tx.QueryRowContext(ctx, `select adapter.team_id from device_adapters adapter
		join connector_definitions definition on definition.id=adapter.connector_definition_id
		join team_members member on member.team_id=adapter.team_id and member.user_id=$3
		join project_feature_flags flags on flags.project_id=adapter.project_id
		where adapter.id=$1 and adapter.project_id=$2 and definition.connector_key=$4 and definition.version=$5
		and adapter.status in('connecting','connected','degraded')
		and flags.flighthub_action_flags_json @> '{"model.write":true}'::jsonb`, request.ConnectorInstanceID, request.ProjectID,
		request.RequestedByUserID, ConnectorKey, ConnectorVersion).Scan(&teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return job, &APIError{SafeCode: "scope_forbidden"}
	}
	if err != nil {
		return job, err
	}
	err = tx.QueryRowContext(ctx, `insert into connector_model_jobs(project_id,team_id,connector_instance_id,requested_by_user_id,
		action_kind,idempotency_key,request_digest,request_envelope_json,reconciliation_name)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9)
		on conflict(project_id,connector_instance_id,action_kind,idempotency_key) do nothing returning id::text`,
		request.ProjectID, teamID, request.ConnectorInstanceID, request.RequestedByUserID, request.ActionKind,
		request.IdempotencyKey, digest, envelope, nullableJobText(reconciliationName)).Scan(&job.ID)
	if errors.Is(err, sql.ErrNoRows) {
		var existingDigest string
		err = tx.QueryRowContext(ctx, `select id::text,request_digest from connector_model_jobs
			where project_id=$1 and connector_instance_id=$2 and action_kind=$3 and idempotency_key=$4`, request.ProjectID,
			request.ConnectorInstanceID, request.ActionKind, request.IdempotencyKey).Scan(&job.ID, &existingDigest)
		if err != nil {
			return job, err
		}
		if existingDigest != digest {
			return job, &APIError{SafeCode: "idempotency_conflict"}
		}
	} else if err != nil {
		return job, err
	}
	payload, _ := json.Marshal(map[string]string{"jobId": job.ID})
	_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
		values($1,$2,$3,$4,'flighthub_model_job',$5,$6,32) on conflict(event_id) do nothing`, request.ProjectID, teamID,
		"flighthub-model-job:"+job.ID, ModelJobEventType, job.ID, payload)
	if err != nil {
		return job, err
	}
	if err = tx.Commit(); err != nil {
		return job, err
	}
	return store.Load(ctx, request.ProjectID, job.ID)
}

func nullableJobText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (store *SQLModelJobStore) Load(ctx context.Context, projectID int, jobID string) (job ModelJob, err error) {
	var envelope, remoteRaw, assetRaw, credentialRaw, scopeRaw []byte
	err = store.db.QueryRowContext(ctx, `select job.id::text,job.project_id,job.team_id,job.connector_instance_id,job.requested_by_user_id,
		job.action_kind,job.idempotency_key,job.request_digest,job.request_envelope_json,coalesce(job.reconciliation_name,''),job.status,
		job.remote_ids_json,job.asset_ids_json,job.progress,job.stage,job.submit_attempt_count,job.reconciliation_count,coalesce(job.last_error_code,''),
		definition.connector_key,definition.version,adapter.credential_envelope_json,adapter.discovery_scope_json
		from connector_model_jobs job join device_adapters adapter on adapter.id=job.connector_instance_id and adapter.project_id=job.project_id
		join connector_definitions definition on definition.id=adapter.connector_definition_id where job.id=$1::uuid and job.project_id=$2`, jobID, projectID).
		Scan(&job.ID, &job.ProjectID, &job.TeamID, &job.ConnectorInstanceID, &job.RequestedByUserID, &job.ActionKind,
			&job.IdempotencyKey, &job.RequestDigest, &envelope, &job.ReconciliationName, &job.Status, &remoteRaw, &assetRaw,
			&job.Progress, &job.Stage, &job.SubmitAttempts, &job.ReconciliationCount, &job.LastErrorCode,
			&job.Instance.ConnectorKey, &job.Instance.Version, &credentialRaw, &scopeRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return job, &APIError{SafeCode: "scope_forbidden"}
	}
	job.RequestEnvelope, job.Instance.CredentialEnvelope, job.Instance.DiscoveryScope = envelope, credentialRaw, scopeRaw
	job.Instance.ID, job.Instance.ProjectID = job.ConnectorInstanceID, job.ProjectID
	if err == nil {
		err = json.Unmarshal(remoteRaw, &job.RemoteIDs)
	}
	if err == nil {
		err = json.Unmarshal(assetRaw, &job.AssetIDs)
	}
	return job, err
}

func (store *SQLModelJobStore) execOne(ctx context.Context, query string, args ...any) error {
	result, err := store.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("FlightHub model job state changed concurrently")
	}
	return nil
}

func (store *SQLModelJobStore) BeginSubmit(ctx context.Context, job ModelJob) error {
	return store.execOne(ctx, `update connector_model_jobs set status='reconciling',stage='submitted',submit_attempt_count=1,submitted_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status='queued' and submit_attempt_count=0`, job.ID, job.ProjectID)
}

func (store *SQLModelJobStore) BindRemoteIDs(ctx context.Context, job ModelJob, ids []string) error {
	raw, _ := json.Marshal(ids)
	return store.execOne(ctx, `update connector_model_jobs set remote_ids_json=$3,stage='reconciling',last_error_code=null,updated_at=now()
		where id=$1::uuid and project_id=$2 and status='reconciling'`, job.ID, job.ProjectID, raw)
}

func (store *SQLModelJobStore) RecordProgress(ctx context.Context, job ModelJob, progress int, stage, code string) error {
	return store.execOne(ctx, `update connector_model_jobs set progress=$3,stage=$4,last_error_code=$5,
		reconciliation_count=reconciliation_count+1,reconciled_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status='reconciling' and reconciliation_count<32`, job.ID, job.ProjectID,
		progress, stage, nullableJobText(code))
}

func (store *SQLModelJobStore) Complete(ctx context.Context, job ModelJob, assetIDs []string) error {
	resolved := make([]string, 0, len(assetIDs))
	for _, remoteID := range assetIDs {
		kind, lookupID := "model", remoteID
		if job.ActionKind != "traditional-create" {
			kind, lookupID = "model-resource", "model:"+remoteID
		}
		var targetType, targetID string
		err := store.db.QueryRowContext(ctx, `select coalesce(canonical_target_type,''),coalesce(canonical_target_id,'')
			from connector_remote_resources where project_id=$1 and connector_instance_id=$2 and resource_kind=$3 and remote_id=$4 and status='active'`,
			job.ProjectID, job.ConnectorInstanceID, kind, lookupID).Scan(&targetType, &targetID)
		if err != nil || targetType != "asset" || targetID == "" {
			return connector.ErrRemoteResourceUnavailable
		}
		resolved = append(resolved, targetID)
	}
	raw, _ := json.Marshal(resolved)
	return store.execOne(ctx, `update connector_model_jobs set status='succeeded',progress=100,stage='completed',asset_ids_json=$3,
		last_error_code=null,reconciled_at=now(),completed_at=now(),updated_at=now() where id=$1::uuid and project_id=$2 and status='reconciling'`,
		job.ID, job.ProjectID, raw)
}

func (store *SQLModelJobStore) Fail(ctx context.Context, job ModelJob, code string, blocked bool) error {
	status := "failed"
	if blocked {
		status = "blocked"
	}
	return store.execOne(ctx, `update connector_model_jobs set status=$3,stage=$3,last_error_code=$4,reconciled_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status in('queued','reconciling')`, job.ID, job.ProjectID, status, code)
}

type ModelJobClient interface {
	CreateModelReconstruction(context.Context, string, string, ModelReconstructionRequest) (ModelReconstructionResult, error)
	ListModels(context.Context, string, string) ([]ModelSummary, error)
	StartOpenModelReconstruction(context.Context, string, string, OpenModelStartRequest) (OpenModelStartResult, error)
	StopOpenModelReconstruction(context.Context, string, string, string) error
	ListRunningOpenModels(context.Context, string, string) ([]OpenModel, error)
	GetOpenModel(context.Context, string, string, string) (OpenModel, error)
	GetOpenModelResource(context.Context, string, string, string) (OpenModelResource, error)
}

type ModelJobProjector interface {
	ApplyModelCatalog(context.Context, connector.Instance, ModelCatalogPoll) error
}

type ModelJobHandler struct {
	store      ModelJobStore
	client     ModelJobClient
	resolver   TokenResolver
	projector  ModelJobProjector
	authSecret string
	now        func() time.Time
}

func NewModelJobHandler(store ModelJobStore, client ModelJobClient, resolver TokenResolver, projector ModelJobProjector, authSecret string, now func() time.Time) (*ModelJobHandler, error) {
	if store == nil || client == nil || resolver == nil || projector == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub model job dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	return &ModelJobHandler{store: store, client: client, resolver: resolver, projector: projector, authSecret: authSecret, now: now}, nil
}

func modelJobDigest(payload ModelJobPayload) (string, []byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), raw, nil
}

func normalizeModelJobRequest(input ModelJobCreateRequest) (ModelJobCreateRequest, string, error) {
	input.ActionKind, input.IdempotencyKey = strings.TrimSpace(input.ActionKind), strings.TrimSpace(input.IdempotencyKey)
	if input.ProjectID <= 0 || input.ConnectorInstanceID <= 0 || input.RequestedByUserID <= 0 || len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 {
		return input, "", &APIError{SafeCode: "request_invalid"}
	}
	name := ""
	switch input.ActionKind {
	case "traditional-create":
		if input.Payload.Traditional == nil || input.Payload.OpenStart != nil || input.Payload.ModelUUID != "" || validateModelReconstruction(input.Payload.Traditional) != nil {
			return input, "", &APIError{SafeCode: "request_invalid"}
		}
		traditional := *input.Payload.Traditional
		traditional.ReconstructionTypes = append([]ModelFileType(nil), input.Payload.Traditional.ReconstructionTypes...)
		traditional.GenerateModelFormats = append([]string(nil), input.Payload.Traditional.GenerateModelFormats...)
		input.Payload.Traditional = &traditional
		sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", input.ProjectID, input.ConnectorInstanceID, input.IdempotencyKey)))
		name = fmt.Sprintf("%s · AeroSight-%s", input.Payload.Traditional.Name, hex.EncodeToString(sum[:6]))
		input.Payload.Traditional.Name = name
	case "open-start":
		if input.Payload.OpenStart == nil || input.Payload.Traditional != nil || input.Payload.ModelUUID != "" || validateOpenModelStart(input.Payload.OpenStart) != nil {
			return input, "", &APIError{SafeCode: "request_invalid"}
		}
	case "open-stop":
		if input.Payload.Traditional != nil || input.Payload.OpenStart != nil || !validModelString(input.Payload.ModelUUID, 256, false) {
			return input, "", &APIError{SafeCode: "request_invalid"}
		}
	default:
		return input, "", &APIError{SafeCode: "request_invalid"}
	}
	return input, name, nil
}

func (handler *ModelJobHandler) Enqueue(ctx context.Context, input ModelJobCreateRequest) (ModelJob, error) {
	input, name, err := normalizeModelJobRequest(input)
	if err != nil {
		return ModelJob{}, err
	}
	digest, raw, err := modelJobDigest(input.Payload)
	if err != nil {
		return ModelJob{}, err
	}
	envelope, err := credentials.EncryptJSON(json.RawMessage(raw), handler.authSecret, credentials.AAD("flighthub-model-job", input.ConnectorInstanceID, input.ProjectID))
	if err != nil {
		return ModelJob{}, err
	}
	envelopeRaw, _ := json.Marshal(envelope)
	return handler.store.Create(ctx, input, envelopeRaw, digest, name)
}

func parseModelJobEvent(event outbox.Event) (string, error) {
	var payload struct {
		JobID string `json:"jobId"`
	}
	if event.ProjectID <= 0 || json.Unmarshal(event.Payload, &payload) != nil || payload.JobID == "" {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return payload.JobID, nil
}

func modelJobRetry(code string) error {
	if code == "" {
		code = "model_job_pending"
	}
	return &APIError{SafeCode: code, Retryable: true}
}

func (handler *ModelJobHandler) Handle(ctx context.Context, event outbox.Event) (returnedErr error) {
	jobID, err := parseModelJobEvent(event)
	if err != nil {
		return err
	}
	job, err := handler.store.Load(ctx, event.ProjectID, jobID)
	if err != nil {
		return err
	}
	if job.Status == "succeeded" || job.Status == "failed" || job.Status == "blocked" {
		return nil
	}
	finalAttempt := event.MaxAttempts > 0 && event.Attempts >= event.MaxAttempts
	defer func() {
		if returnedErr == nil || !finalAttempt {
			return
		}
		if failErr := handler.store.Fail(ctx, job, "model_reconciliation_exhausted", true); failErr != nil {
			returnedErr = errors.Join(returnedErr, failErr)
			return
		}
		returnedErr = nil
	}()
	if job.ReconciliationCount >= 32 {
		return handler.store.Fail(ctx, job, "model_reconciliation_exhausted", true)
	}
	envelope, err := credentials.ParseEnvelope(job.RequestEnvelope)
	if err != nil {
		return handler.store.Fail(ctx, job, "request_unavailable", true)
	}
	var payload ModelJobPayload
	if err := credentials.DecryptJSON(envelope, handler.authSecret, credentials.AAD("flighthub-model-job", job.ConnectorInstanceID, job.ProjectID), &payload); err != nil {
		return handler.store.Fail(ctx, job, "request_unavailable", true)
	}
	scope, err := parseScope(job.Instance.DiscoveryScope)
	if err != nil {
		return err
	}
	token, err := handler.resolver.ResolveToken(ctx, job.Instance)
	if err != nil {
		return err
	}
	defer func() { token = "" }()
	if job.Status == "queued" {
		if err := handler.store.BeginSubmit(ctx, job); err != nil {
			return err
		}
		job.Status, job.SubmitAttempts = "reconciling", 1
		if job.ActionKind == "open-stop" {
			job.RemoteIDs = []string{payload.ModelUUID}
			if err := handler.store.BindRemoteIDs(ctx, job, job.RemoteIDs); err != nil {
				return err
			}
		}
		ids, submitErr := handler.submit(ctx, job, payload, token, scope.ProjectUUID)
		if submitErr != nil {
			if err := handler.store.RecordProgress(ctx, job, 0, "submit-unknown", SafeCode(submitErr)); err != nil {
				return err
			}
			return modelJobRetry(SafeCode(submitErr))
		}
		if err := handler.store.BindRemoteIDs(ctx, job, ids); err != nil {
			return err
		}
		job.RemoteIDs = ids
	}
	return handler.reconcile(ctx, job, payload, token, scope.ProjectUUID)
}

func (handler *ModelJobHandler) Handler(ctx context.Context, _ *sql.Tx, event outbox.Event) error {
	return handler.Handle(ctx, event)
}

func (handler *ModelJobHandler) submit(ctx context.Context, job ModelJob, payload ModelJobPayload, token, projectUUID string) ([]string, error) {
	switch job.ActionKind {
	case "traditional-create":
		result, err := handler.client.CreateModelReconstruction(ctx, token, projectUUID, *payload.Traditional)
		if err != nil {
			return nil, err
		}
		return []string{strconv.FormatInt(result.ID, 10)}, nil
	case "open-start":
		result, err := handler.client.StartOpenModelReconstruction(ctx, token, projectUUID, *payload.OpenStart)
		if err != nil {
			return nil, err
		}
		ids := []string{}
		for _, item := range []*OpenModelTaskResult{result.Model2D, result.Model3D, result.Model3DGS, result.ModelLidar} {
			if item != nil {
				ids = append(ids, item.UUID)
			}
		}
		return ids, nil
	case "open-stop":
		return []string{payload.ModelUUID}, handler.client.StopOpenModelReconstruction(ctx, token, projectUUID, payload.ModelUUID)
	default:
		return nil, &APIError{SafeCode: "request_invalid"}
	}
}

func requestedOpenModelTypes(input *OpenModelStartRequest) []OpenModelType {
	if input == nil {
		return nil
	}
	result := make([]OpenModelType, 0, 4)
	for _, item := range []struct {
		selected bool
		kind     OpenModelType
	}{
		{input.Parameter2D != "", OpenModel2D},
		{input.Parameter3D != "", OpenModel3D},
		{input.Parameter3DGS != "", OpenModel3DGS},
		{input.ParameterLidar != "", OpenModelLidar},
	} {
		if item.selected {
			result = append(result, item.kind)
		}
	}
	return result
}

func (handler *ModelJobHandler) reconcile(ctx context.Context, job ModelJob, payload ModelJobPayload, token, projectUUID string) error {
	if job.ActionKind == "traditional-create" {
		models, err := handler.client.ListModels(ctx, token, projectUUID)
		if err != nil {
			return modelJobRetry(SafeCode(err))
		}
		var found *ModelSummary
		for index := range models {
			if (len(job.RemoteIDs) > 0 && strconv.FormatInt(models[index].ID, 10) == job.RemoteIDs[0]) || (len(job.RemoteIDs) == 0 && models[index].Name == job.ReconciliationName) {
				if found != nil {
					return handler.store.Fail(ctx, job, "model_reconciliation_ambiguous", true)
				}
				found = &models[index]
			}
		}
		if found == nil {
			if err := handler.store.RecordProgress(ctx, job, 0, "waiting-catalog", "model_not_visible"); err != nil {
				return err
			}
			return modelJobRetry("model_not_visible")
		}
		ids := []string{strconv.FormatInt(found.ID, 10)}
		if len(job.RemoteIDs) == 0 {
			if err := handler.store.BindRemoteIDs(ctx, job, ids); err != nil {
				return err
			}
		}
		if err := handler.projector.ApplyModelCatalog(ctx, job.Instance, ModelCatalogPoll{Models: []ModelSummary{*found}, ReceivedAt: handler.now().UTC()}); err != nil {
			return err
		}
		return handler.store.Complete(ctx, job, ids)
	}
	if len(job.RemoteIDs) == 0 && job.ActionKind == "open-start" {
		running, err := handler.client.ListRunningOpenModels(ctx, token, projectUUID)
		if err != nil {
			return modelJobRetry(SafeCode(err))
		}
		for _, requestedType := range requestedOpenModelTypes(payload.OpenStart) {
			matchedID := ""
			for _, item := range running {
				if item.ResourceUUID != payload.OpenStart.ResourceUUID || item.ModelType != requestedType {
					continue
				}
				if matchedID != "" {
					return handler.store.Fail(ctx, job, "model_reconciliation_ambiguous", true)
				}
				matchedID = item.ModelUUID
			}
			if matchedID == "" {
				job.RemoteIDs = nil
				break
			}
			job.RemoteIDs = append(job.RemoteIDs, matchedID)
		}
		if len(job.RemoteIDs) == 0 {
			if err := handler.store.RecordProgress(ctx, job, 0, "waiting-remote-id", "model_not_visible"); err != nil {
				return err
			}
			return modelJobRetry("model_not_visible")
		}
		if err := handler.store.BindRemoteIDs(ctx, job, job.RemoteIDs); err != nil {
			return err
		}
	}
	models := []OpenModel{}
	resources := []OpenModelResource{}
	resourceSeen := map[string]struct{}{}
	progress := 100
	for _, id := range job.RemoteIDs {
		item, err := handler.client.GetOpenModel(ctx, token, projectUUID, id)
		if err != nil {
			return modelJobRetry(SafeCode(err))
		}
		models = append(models, item)
		if item.ReconstructionProgress < progress {
			progress = item.ReconstructionProgress
		}
		if item.ModelStatus == OpenModelReconstructionFailed || item.ModelStatus == OpenModelRequestingResourceFailed {
			return handler.store.Fail(ctx, job, "model_reconstruction_failed", false)
		}
		if item.ModelStatus == OpenModelReconstructionCanceled && job.ActionKind != "open-stop" {
			return handler.store.Fail(ctx, job, "model_reconstruction_canceled", false)
		}
		if job.ActionKind == "open-stop" && (item.ModelStatus == OpenModelReconstructionSucceeded || item.ModelStatus == OpenModelMapReconstructionSucceeded) {
			return handler.store.Fail(ctx, job, "model_stop_not_effective", false)
		}
		if item.ModelStatus != OpenModelReconstructionSucceeded && item.ModelStatus != OpenModelMapReconstructionSucceeded && item.ModelStatus != OpenModelReconstructionCanceled {
			if err := handler.store.RecordProgress(ctx, job, progress, "reconstructing", ""); err != nil {
				return err
			}
			return modelJobRetry("model_job_pending")
		}
		if _, exists := resourceSeen[item.ResourceUUID]; !exists {
			resource, resourceErr := handler.client.GetOpenModelResource(ctx, token, projectUUID, item.ResourceUUID)
			if resourceErr == nil {
				resources = append(resources, resource)
				resourceSeen[item.ResourceUUID] = struct{}{}
			}
		}
	}
	if err := handler.projector.ApplyModelCatalog(ctx, job.Instance, ModelCatalogPoll{OpenModels: models, Resources: resources, ReceivedAt: handler.now().UTC()}); err != nil {
		return err
	}
	return handler.store.Complete(ctx, job, job.RemoteIDs)
}
