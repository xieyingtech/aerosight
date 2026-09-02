package flighthub

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
)

const FlightHubModelDeleteEventType = "flighthub.model_delete.requested"

type ModelDeleteJob struct {
	ID, ActionKind, CapabilityCode, FeatureFlag, ExpectedRemoteVersion, PreviewDigest string
	ProjectID, TeamID, RequestedByUserID                                              int
	ConnectorInstanceID, TargetResourceID                                             int64
	ApprovalRequestID, Status, LastErrorCode                                          string
	AttemptCount, ReconciliationCount                                                 int
	RequestEnvelope                                                                   json.RawMessage
	TargetRemoteID, TargetRemoteVersion, TargetKind, TargetStatus, ConnectorStatus    string
	AssetID, AssetStatus                                                              sql.NullString
	DependentReferenceCount                                                           int
	Authorized, ActionEnabled, CapabilityVerified, ApprovalValid                      bool
	Instance                                                                          connector.Instance
}

type ModelDeleteStore interface {
	Load(context.Context, int, string) (ModelDeleteJob, error)
	Begin(context.Context, ModelDeleteJob) error
	Complete(context.Context, ModelDeleteJob) error
	Fail(context.Context, ModelDeleteJob, string) error
	Block(context.Context, ModelDeleteJob, string) error
}

type SQLModelDeleteStore struct{ db *sql.DB }

func NewSQLModelDeleteStore(database *sql.DB) *SQLModelDeleteStore {
	return &SQLModelDeleteStore{db: database}
}

func (store *SQLModelDeleteStore) Load(ctx context.Context, projectID int, jobID string) (job ModelDeleteJob, err error) {
	if store == nil || store.db == nil || projectID <= 0 || strings.TrimSpace(jobID) == "" {
		return job, &APIError{SafeCode: "request_invalid"}
	}
	var envelope []byte
	err = store.db.QueryRowContext(ctx, `select job.id::text,job.project_id,job.team_id,job.connector_instance_id,
		job.target_resource_id,job.approval_request_id::text,job.requested_by_user_id,job.action_kind,
		job.capability_code,job.feature_flag,job.expected_remote_version,job.preview_digest,
		job.request_envelope_json,job.status,job.attempt_count,job.reconciliation_count,coalesce(job.last_error_code,''),
		coalesce(target.remote_id,''),coalesce(target.remote_version,''),coalesce(target.resource_kind,''),
		coalesce(target.status,''),adapter.status,
		case when target.canonical_target_type='asset' then target.canonical_target_id end,asset.status,
		coalesce((select count(*)::int from connector_asset_access_refs ref where ref.project_id=job.project_id
		  and ref.connector_instance_id=job.connector_instance_id and ref.remote_resource_id=target.id),0),
		member.role in('owner','admin'),
		coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(job.feature_flag,true),false),
		exists(select 1 from connector_capability_snapshots capability where capability.project_id=job.project_id
		  and capability.connector_instance_id=job.connector_instance_id and capability.capability_code=job.capability_code
		  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		  and capability.region='cn' and capability.deployment='cn-public-cloud'
		  and capability.status='supported' and capability.evidence_level='field-write'
		  and (capability.expires_at is null or capability.expires_at>now())
		  and capability.device_model is null and capability.firmware_version is null),
		exists(select 1 from approval_requests approval where approval.id=job.approval_request_id
		  and approval.project_id=job.project_id and approval.team_id=job.team_id
		  and approval.resource_type='connector_remote_resource' and approval.resource_id=job.target_resource_id::text
		  and approval.action=case job.action_kind when 'model-delete' then 'flighthub.model.delete'
		    else 'flighthub.model-resource.delete' end and approval.status='approved' and approval.expires_at>now()
		  and approval.context_json->>'previewDigest'=job.preview_digest
		  and approval.context_json->>'expectedRemoteVersion'=job.expected_remote_version),
		definition.connector_key,definition.version,adapter.credential_envelope_json,adapter.discovery_scope_json
	 from connector_model_delete_jobs job
	 join device_adapters adapter on adapter.id=job.connector_instance_id and adapter.project_id=job.project_id
	 join connector_definitions definition on definition.id=adapter.connector_definition_id
	 join team_members member on member.team_id=job.team_id and member.user_id=job.requested_by_user_id
	 left join project_feature_flags flags on flags.project_id=job.project_id
	 left join connector_remote_resources target on target.id=job.target_resource_id and target.project_id=job.project_id
	   and target.connector_instance_id=job.connector_instance_id
	 left join assets asset on target.canonical_target_type='asset' and asset.project_id=target.project_id
	   and asset.id::text=target.canonical_target_id
	 where job.id=$1::uuid and job.project_id=$2`, jobID, projectID).Scan(
		&job.ID, &job.ProjectID, &job.TeamID, &job.ConnectorInstanceID, &job.TargetResourceID,
		&job.ApprovalRequestID, &job.RequestedByUserID, &job.ActionKind, &job.CapabilityCode, &job.FeatureFlag,
		&job.ExpectedRemoteVersion, &job.PreviewDigest, &envelope, &job.Status, &job.AttemptCount,
		&job.ReconciliationCount, &job.LastErrorCode, &job.TargetRemoteID, &job.TargetRemoteVersion,
		&job.TargetKind, &job.TargetStatus, &job.ConnectorStatus, &job.AssetID, &job.AssetStatus,
		&job.DependentReferenceCount, &job.Authorized, &job.ActionEnabled, &job.CapabilityVerified,
		&job.ApprovalValid, &job.Instance.ConnectorKey, &job.Instance.Version, &job.Instance.CredentialEnvelope,
		&job.Instance.DiscoveryScope,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return job, &APIError{SafeCode: "scope_forbidden"}
	}
	job.RequestEnvelope = json.RawMessage(envelope)
	job.Instance.ID, job.Instance.ProjectID = job.ConnectorInstanceID, job.ProjectID
	return job, err
}

func (store *SQLModelDeleteStore) update(ctx context.Context, query string, args ...any) error {
	result, err := store.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("FlightHub model delete state changed concurrently")
	}
	return nil
}

func (store *SQLModelDeleteStore) Begin(ctx context.Context, job ModelDeleteJob) error {
	result, err := store.db.ExecContext(ctx, `update connector_model_delete_jobs job
	 set status='executing',attempt_count=1,attempted_at=now(),updated_at=now()
	 where job.id=$1::uuid and job.project_id=$2 and job.status='queued' and job.attempt_count=0
	 and exists(select 1 from connector_remote_resources target where target.id=job.target_resource_id
	   and target.project_id=job.project_id and target.connector_instance_id=job.connector_instance_id
	   and target.resource_kind=case job.action_kind when 'model-delete' then 'model' else 'model-resource' end
	   and target.status='active' and target.remote_version=job.expected_remote_version)
	 and exists(select 1 from approval_requests approval where approval.id=job.approval_request_id
	   and approval.project_id=job.project_id and approval.team_id=job.team_id and approval.status='approved'
	   and approval.expires_at>now() and approval.resource_type='connector_remote_resource'
	   and approval.resource_id=job.target_resource_id::text
	   and approval.action=case job.action_kind when 'model-delete' then 'flighthub.model.delete'
	     else 'flighthub.model-resource.delete' end
	   and approval.context_json->>'previewDigest'=job.preview_digest
	   and approval.context_json->>'expectedRemoteVersion'=job.expected_remote_version)`, job.ID, job.ProjectID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return &APIError{SafeCode: "approval_or_version_conflict"}
	}
	return nil
}

func (store *SQLModelDeleteStore) Complete(ctx context.Context, job ModelDeleteJob) (returnedErr error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	changed, err := tx.ExecContext(ctx, `update connector_remote_resources set status='missing',missing_at=now(),updated_at=now()
	 where id=$1 and project_id=$2 and connector_instance_id=$3 and status='active'
	   and resource_kind=$4 and remote_version=$5`, job.TargetResourceID, job.ProjectID, job.ConnectorInstanceID,
		modelDeletePolicies[job.ActionKind].targetKind, job.ExpectedRemoteVersion)
	if err != nil {
		return err
	}
	if count, _ := changed.RowsAffected(); count != 1 {
		return &APIError{SafeCode: "version_conflict"}
	}
	assetDeleted := false
	if job.AssetID.Valid {
		result, assetErr := tx.ExecContext(ctx, `update assets set status='deleted',deleted_at=coalesce(deleted_at,now()),
		  failed_at=null,failure_code=null where project_id=$1 and id::text=$2 and status<>'deleted'`,
			job.ProjectID, job.AssetID.String)
		if assetErr != nil {
			return assetErr
		}
		count, _ := result.RowsAffected()
		assetDeleted = count == 1
	}
	result := map[string]any{"confirmed": true, "remoteResourceId": job.TargetResourceID,
		"assetMarkedDeleted": assetDeleted}
	changed, err = tx.ExecContext(ctx, `update connector_model_delete_jobs set status='succeeded',result_json=$3,
	 last_error_code=null,reconciliation_count=reconciliation_count+1,completed_at=now(),updated_at=now()
	 where id=$1::uuid and project_id=$2 and status='executing'`, job.ID, job.ProjectID, result)
	if err != nil {
		return err
	}
	if count, _ := changed.RowsAffected(); count != 1 {
		return errors.New("FlightHub model delete completion state changed")
	}
	return tx.Commit()
}

func (store *SQLModelDeleteStore) Fail(ctx context.Context, job ModelDeleteJob, code string) error {
	return store.update(ctx, `update connector_model_delete_jobs set status='failed',last_error_code=$3,
	 reconciliation_count=reconciliation_count+case when attempt_count=1 then 1 else 0 end,
	 completed_at=now(),updated_at=now() where id=$1::uuid and project_id=$2 and status in('queued','executing')`,
		job.ID, job.ProjectID, code)
}

func (store *SQLModelDeleteStore) Block(ctx context.Context, job ModelDeleteJob, code string) error {
	return store.update(ctx, `update connector_model_delete_jobs set status='blocked',last_error_code=$3,
	 reconciliation_count=reconciliation_count+1,unknown_at=now(),completed_at=now(),updated_at=now()
	 where id=$1::uuid and project_id=$2 and status in('queued','executing')`, job.ID, job.ProjectID, code)
}

type ModelDeleteClient interface {
	DeleteOpenModel(context.Context, string, string, string) error
	DeleteOpenModelResource(context.Context, string, string, string) error
	GetOpenModel(context.Context, string, string, string) (OpenModel, error)
	GetOpenModelResource(context.Context, string, string, string) (OpenModelResource, error)
}

type modelDeletePolicy struct{ capability, featureFlag, targetKind string }

var modelDeletePolicies = map[string]modelDeletePolicy{
	"model-delete":          {"model.delete", FlightHubModelDeleteFeatureFlag, "model"},
	"model-resource-delete": {"model.resource.delete", FlightHubModelResourceDeleteFeatureFlag, "model-resource"},
}

type ModelDeleteHandler struct {
	store      ModelDeleteStore
	client     ModelDeleteClient
	resolver   TokenResolver
	authSecret string
}

func NewModelDeleteHandler(store ModelDeleteStore, client ModelDeleteClient, resolver TokenResolver,
	authSecret string) (*ModelDeleteHandler, error) {
	if store == nil || client == nil || resolver == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub model delete dependencies are required")
	}
	return &ModelDeleteHandler{store: store, client: client, resolver: resolver, authSecret: authSecret}, nil
}

func modelDeleteEvent(event outbox.Event) (string, error) {
	var payload struct {
		JobID string `json:"jobId"`
	}
	if event.ProjectID <= 0 || json.Unmarshal(event.Payload, &payload) != nil || strings.TrimSpace(payload.JobID) == "" {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return payload.JobID, nil
}

func modelDeletePreviewHash(job ModelDeleteJob) string {
	preview := map[string]any{
		"targetResourceId": job.TargetResourceID, "resourceKind": job.TargetKind,
		"remoteVersion": job.TargetRemoteVersion, "assetId": nil, "assetStatus": nil,
		"dependentReferenceCount": job.DependentReferenceCount,
		"effect":                  "remote-delete-and-local-mark-missing",
	}
	if job.AssetID.Valid {
		preview["assetId"] = job.AssetID.String
	}
	if job.AssetStatus.Valid {
		preview["assetStatus"] = job.AssetStatus.String
	}
	raw, _ := json.Marshal(preview)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func (handler *ModelDeleteHandler) confirmation(job ModelDeleteJob) error {
	var envelope credentials.Envelope
	if json.Unmarshal(job.RequestEnvelope, &envelope) != nil {
		return &APIError{SafeCode: "request_invalid"}
	}
	var request struct {
		Confirmation string `json:"confirmation"`
	}
	if credentials.DecryptJSON(envelope, handler.authSecret,
		credentials.AAD("flighthub-model-delete", job.ID, job.ProjectID), &request) != nil || request.Confirmation != "DELETE" {
		return &APIError{SafeCode: "request_invalid"}
	}
	return nil
}

func definitiveModelDeleteError(code string) bool {
	switch code {
	case "request_invalid", "scope_forbidden", "credential_invalid", "configuration_required":
		return true
	default:
		return false
	}
}

func (handler *ModelDeleteHandler) reconcile(ctx context.Context, job ModelDeleteJob, token, projectUUID string,
	deleteErr error) error {
	var err error
	if job.ActionKind == "model-delete" {
		_, err = handler.client.GetOpenModel(ctx, token, projectUUID, job.TargetRemoteID)
	} else {
		_, err = handler.client.GetOpenModelResource(ctx, token, projectUUID, job.TargetRemoteID)
	}
	if IsSafeCode(err, "scope_not_found") {
		return handler.store.Complete(ctx, job)
	}
	if err != nil {
		return handler.store.Block(ctx, job, "delete_result_unknown")
	}
	if deleteErr != nil && definitiveModelDeleteError(safeWorkflowCode(deleteErr)) {
		return handler.store.Fail(ctx, job, safeWorkflowCode(deleteErr))
	}
	return handler.store.Block(ctx, job, "delete_not_confirmed")
}

func (handler *ModelDeleteHandler) Handler(ctx context.Context, _ *sql.Tx, event outbox.Event) error {
	jobID, err := modelDeleteEvent(event)
	if err != nil {
		return err
	}
	job, err := handler.store.Load(ctx, event.ProjectID, jobID)
	if err != nil {
		return err
	}
	policy, known := modelDeletePolicies[job.ActionKind]
	if !known || policy.capability != job.CapabilityCode || policy.featureFlag != job.FeatureFlag {
		return handler.store.Fail(ctx, job, "request_invalid")
	}
	if job.Status != "queued" && job.Status != "executing" {
		return nil
	}
	if job.TargetKind != policy.targetKind || job.TargetStatus != "active" || job.TargetRemoteID == "" {
		if job.Status == "executing" {
			return handler.store.Block(ctx, job, "scope_forbidden")
		}
		return handler.store.Fail(ctx, job, "scope_forbidden")
	}
	if job.Status == "queued" {
		if !job.Authorized {
			return handler.store.Fail(ctx, job, "permission_denied")
		}
		switch job.ConnectorStatus {
		case "connecting", "connected", "degraded":
		default:
			return handler.store.Fail(ctx, job, "connector_disabled")
		}
		if !job.ActionEnabled || !job.CapabilityVerified {
			return handler.store.Fail(ctx, job, "action_disabled")
		}
		if !job.ApprovalValid {
			return handler.store.Fail(ctx, job, "approval_required")
		}
		if job.TargetRemoteVersion != job.ExpectedRemoteVersion || modelDeletePreviewHash(job) != job.PreviewDigest {
			return handler.store.Fail(ctx, job, "preview_conflict")
		}
		if err = handler.confirmation(job); err != nil {
			return handler.store.Fail(ctx, job, safeWorkflowCode(err))
		}
	}
	scope, err := parseScope(job.Instance.DiscoveryScope)
	if err != nil {
		return handler.store.Fail(ctx, job, "scope_forbidden")
	}
	token, err := handler.resolver.ResolveToken(ctx, job.Instance)
	if err != nil {
		return handler.store.Fail(ctx, job, safeWorkflowCode(err))
	}
	defer func() { token = "" }()
	if job.Status == "executing" || job.AttemptCount > 0 {
		return handler.reconcile(ctx, job, token, scope.ProjectUUID, nil)
	}
	if err = handler.store.Begin(ctx, job); err != nil {
		return handler.store.Fail(ctx, job, safeWorkflowCode(err))
	}
	if job.ActionKind == "model-delete" {
		err = handler.client.DeleteOpenModel(ctx, token, scope.ProjectUUID, job.TargetRemoteID)
	} else {
		err = handler.client.DeleteOpenModelResource(ctx, token, scope.ProjectUUID, job.TargetRemoteID)
	}
	return handler.reconcile(ctx, job, token, scope.ProjectUUID, err)
}
