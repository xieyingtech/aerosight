package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
)

const FlightHubGeospatialActionEventType = "flighthub.geospatial_action.requested"

type GeospatialActionJob struct {
	ID                    string
	ProjectID             int
	TeamID                int
	ConnectorInstanceID   int64
	TargetResourceID      sql.NullInt64
	ActionKind            string
	CapabilityCode        string
	FeatureFlag           string
	ExpectedRemoteVersion sql.NullString
	RequestEnvelope       json.RawMessage
	Status                string
	AttemptCount          int
	TargetRemoteID        string
	TargetRemoteVersion   string
	TargetKind            string
	TargetStatus          string
	ConnectorStatus       string
	Authorized            bool
	ActionEnabled         bool
	CapabilityVerified    bool
	Instance              connector.Instance
}

type GeospatialActionPayload struct {
	Create       *MapElementCreateRequest
	Update       *MapElementUpdateRequest
	Confirmation string
}

type GeospatialActionStore interface {
	Load(context.Context, int, string) (GeospatialActionJob, error)
	Begin(context.Context, GeospatialActionJob) error
	Complete(context.Context, GeospatialActionJob, GeospatialActionPayload, string, MapElementDeleteResult) error
	Fail(context.Context, GeospatialActionJob, string) error
	Block(context.Context, GeospatialActionJob, string) error
}

type SQLGeospatialActionStore struct{ db *sql.DB }

func NewSQLGeospatialActionStore(database *sql.DB) *SQLGeospatialActionStore {
	return &SQLGeospatialActionStore{db: database}
}

func (store *SQLGeospatialActionStore) Load(ctx context.Context, projectID int, jobID string) (job GeospatialActionJob, err error) {
	if store == nil || store.db == nil || projectID <= 0 || strings.TrimSpace(jobID) == "" {
		return job, &APIError{SafeCode: "request_invalid"}
	}
	var envelope []byte
	err = store.db.QueryRowContext(ctx, `select job.id::text,job.project_id,job.team_id,job.connector_instance_id,
		job.target_resource_id,job.action_kind,job.capability_code,job.feature_flag,job.expected_remote_version,
		job.request_envelope_json,job.status,job.attempt_count,
		coalesce(target.remote_id,''),coalesce(target.remote_version,''),coalesce(target.resource_kind,''),coalesce(target.status,''),adapter.status,
		case when job.action_kind='map-element-delete' then member.role in('owner','admin')
		  else member.role in('owner','admin') or exists(select 1 from project_permissions permission
		    where permission.project_id=job.project_id and permission.team_id=job.team_id
		      and permission.user_id=job.requested_by_user_id and permission.permission='mission:operate') end,
		coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(job.feature_flag,true),false),
		exists(select 1 from connector_capability_snapshots capability
		  where capability.project_id=job.project_id and capability.connector_instance_id=job.connector_instance_id
		    and capability.capability_code=job.capability_code and capability.status='supported'
		    and capability.evidence_level='field-write' and (capability.expires_at is null or capability.expires_at>now())
		    and capability.device_model is null and capability.firmware_version is null),
		definition.connector_key,definition.version,adapter.credential_envelope_json,adapter.discovery_scope_json
	 from connector_geospatial_action_jobs job
	 join device_adapters adapter on adapter.id=job.connector_instance_id and adapter.project_id=job.project_id
	 join connector_definitions definition on definition.id=adapter.connector_definition_id
	 join team_members member on member.team_id=job.team_id and member.user_id=job.requested_by_user_id
	 left join project_feature_flags flags on flags.project_id=job.project_id
	 left join connector_remote_resources target on target.id=job.target_resource_id and target.project_id=job.project_id
	   and target.connector_instance_id=job.connector_instance_id
	 where job.id=$1::uuid and job.project_id=$2`, jobID, projectID).Scan(
		&job.ID, &job.ProjectID, &job.TeamID, &job.ConnectorInstanceID, &job.TargetResourceID,
		&job.ActionKind, &job.CapabilityCode, &job.FeatureFlag, &job.ExpectedRemoteVersion, &envelope, &job.Status, &job.AttemptCount,
		&job.TargetRemoteID, &job.TargetRemoteVersion, &job.TargetKind, &job.TargetStatus, &job.ConnectorStatus,
		&job.Authorized, &job.ActionEnabled, &job.CapabilityVerified, &job.Instance.ConnectorKey, &job.Instance.Version,
		&job.Instance.CredentialEnvelope, &job.Instance.DiscoveryScope,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return job, &APIError{SafeCode: "scope_forbidden"}
	}
	job.RequestEnvelope = json.RawMessage(envelope)
	job.Instance.ID, job.Instance.ProjectID = job.ConnectorInstanceID, job.ProjectID
	return job, err
}

func (store *SQLGeospatialActionStore) update(ctx context.Context, statement string, args ...any) error {
	result, err := store.db.ExecContext(ctx, statement, args...)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("FlightHub geospatial action state changed concurrently")
	}
	return nil
}

func (store *SQLGeospatialActionStore) Begin(ctx context.Context, job GeospatialActionJob) error {
	if job.ActionKind == "map-element-create" {
		return store.update(ctx, `update connector_geospatial_action_jobs set status='executing',attempt_count=1,attempted_at=now(),updated_at=now()
			where id=$1::uuid and project_id=$2 and status='queued' and attempt_count=0`, job.ID, job.ProjectID)
	}
	result, err := store.db.ExecContext(ctx, `update connector_geospatial_action_jobs job set status='executing',attempt_count=1,attempted_at=now(),updated_at=now()
		where job.id=$1::uuid and job.project_id=$2 and job.status='queued' and job.attempt_count=0
		and exists(select 1 from connector_remote_resources target where target.id=job.target_resource_id
		  and target.project_id=job.project_id and target.connector_instance_id=job.connector_instance_id
		  and target.resource_kind='map-element' and target.status='active' and target.remote_version=job.expected_remote_version)`, job.ID, job.ProjectID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return &APIError{SafeCode: "version_conflict"}
	}
	return nil
}

func mapElementCreateSummary(request MapElementCreateRequest) (map[string]any, error) {
	properties := safeGeoJSONProperties(request.Resource.Content.Properties)
	return map[string]any{
		"name": request.Name, "description": request.Description, "elementType": request.Resource.Type,
		"remark": request.Resource.Remark, "status": 1, "display": 1,
		"geometry": request.Resource.Content.Geometry, "properties": properties,
		"coordinateReference": "unverified", "source": "governed-action",
	}, nil
}

func mapElementUpdatePatch(request MapElementUpdateRequest) (map[string]any, error) {
	patch := map[string]any{"source": "governed-action"}
	if request.Name != nil {
		patch["name"] = *request.Name
	}
	if request.Status != nil {
		patch["status"] = *request.Status
	}
	if request.Display != nil {
		patch["display"] = *request.Display
	}
	if request.Remark != nil {
		patch["remark"] = *request.Remark
	}
	if request.ElevationLoadStatus != nil {
		patch["elevationLoadStatus"] = *request.ElevationLoadStatus
	}
	if request.TargetLayerID != nil {
		patch["targetLayerId"] = *request.TargetLayerID
	}
	if request.Content != nil {
		properties := safeGeoJSONProperties(request.Content.Properties)
		patch["geometry"], patch["properties"] = request.Content.Geometry, properties
		patch["coordinateReference"] = "unverified"
	}
	return patch, nil
}

func (store *SQLGeospatialActionStore) Complete(ctx context.Context, job GeospatialActionJob, payload GeospatialActionPayload,
	remoteID string, deleted MapElementDeleteResult,
) (returnedErr error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	result := map[string]any{"confirmed": true}
	switch job.ActionKind {
	case "map-element-create":
		if payload.Create == nil || strings.TrimSpace(remoteID) == "" {
			return &APIError{SafeCode: "schema_incompatible"}
		}
		version, digestErr := alertDigest(payload.Create)
		if digestErr != nil {
			return digestErr
		}
		summary, summaryErr := mapElementCreateSummary(*payload.Create)
		if summaryErr != nil {
			return summaryErr
		}
		var resourceID int64
		err = tx.QueryRowContext(ctx, `insert into connector_remote_resources(
			project_id,team_id,connector_instance_id,resource_kind,remote_id,remote_version,remote_updated_at,status,summary_json
		) values($1,$2,$3,'map-element',$4,$5,now(),'active',$6) returning id`,
			job.ProjectID, job.TeamID, job.ConnectorInstanceID, strings.TrimSpace(remoteID), version, summary).Scan(&resourceID)
		if err != nil {
			return err
		}
		result["remoteResourceId"], result["remoteVersion"] = resourceID, version
	case "map-element-update":
		if payload.Update == nil || !job.TargetResourceID.Valid || !job.ExpectedRemoteVersion.Valid {
			return &APIError{SafeCode: "request_invalid"}
		}
		version, digestErr := alertDigest(map[string]any{"previous": job.ExpectedRemoteVersion.String, "update": payload.Update})
		if digestErr != nil {
			return digestErr
		}
		patch, patchErr := mapElementUpdatePatch(*payload.Update)
		if patchErr != nil {
			return patchErr
		}
		changed, updateErr := tx.ExecContext(ctx, `update connector_remote_resources set remote_version=$4,remote_updated_at=now(),
			summary_json=summary_json||$5::jsonb,last_seen_at=now(),updated_at=now()
			where id=$1 and project_id=$2 and connector_instance_id=$3 and resource_kind='map-element'
			  and status='active' and remote_version=$6`, job.TargetResourceID.Int64, job.ProjectID, job.ConnectorInstanceID,
			version, patch, job.ExpectedRemoteVersion.String)
		if updateErr != nil {
			return updateErr
		}
		if count, _ := changed.RowsAffected(); count != 1 {
			return &APIError{SafeCode: "version_conflict"}
		}
		result["remoteResourceId"], result["remoteVersion"] = job.TargetResourceID.Int64, version
	case "map-element-delete":
		if !job.TargetResourceID.Valid || !job.ExpectedRemoteVersion.Valid || payload.Confirmation != "DELETE" {
			return &APIError{SafeCode: "request_invalid"}
		}
		changed, updateErr := tx.ExecContext(ctx, `update connector_remote_resources set status='missing',missing_at=now(),updated_at=now()
			where id=$1 and project_id=$2 and connector_instance_id=$3 and resource_kind='map-element'
			  and status='active' and remote_version=$4`, job.TargetResourceID.Int64, job.ProjectID,
			job.ConnectorInstanceID, job.ExpectedRemoteVersion.String)
		if updateErr != nil {
			return updateErr
		}
		if count, _ := changed.RowsAffected(); count != 1 {
			return &APIError{SafeCode: "version_conflict"}
		}
		result["remoteResourceId"], result["deleted"] = job.TargetResourceID.Int64, true
		result["affectedTriStateCount"] = len(deleted.AffectedTriStates)
	default:
		return &APIError{SafeCode: "request_invalid"}
	}
	changed, err := tx.ExecContext(ctx, `update connector_geospatial_action_jobs set status='succeeded',result_json=$3,
		last_error_code=null,completed_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status='executing'`, job.ID, job.ProjectID, result)
	if err != nil {
		return err
	}
	if count, _ := changed.RowsAffected(); count != 1 {
		return errors.New("FlightHub geospatial action completion state changed")
	}
	return tx.Commit()
}

func (store *SQLGeospatialActionStore) Fail(ctx context.Context, job GeospatialActionJob, code string) error {
	return store.update(ctx, `update connector_geospatial_action_jobs set status='failed',last_error_code=$3,completed_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status in('queued','executing')`, job.ID, job.ProjectID, code)
}

func (store *SQLGeospatialActionStore) Block(ctx context.Context, job GeospatialActionJob, code string) error {
	return store.update(ctx, `update connector_geospatial_action_jobs set status='blocked',last_error_code=$3,unknown_at=now(),completed_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status in('queued','executing')`, job.ID, job.ProjectID, code)
}

type GeospatialActionClient interface {
	CreateMapElement(context.Context, string, string, MapElementCreateRequest) (MapElementMutationResult, error)
	UpdateWorkspaceMapElement(context.Context, string, string, string, MapElementUpdateRequest) (MapElementMutationResult, error)
	DeleteWorkspaceMapElement(context.Context, string, string, string, string) (MapElementDeleteResult, error)
}

type GeospatialActionHandler struct {
	store      GeospatialActionStore
	client     GeospatialActionClient
	resolver   TokenResolver
	authSecret string
}

func NewGeospatialActionHandler(store GeospatialActionStore, client GeospatialActionClient, resolver TokenResolver,
	authSecret string,
) (*GeospatialActionHandler, error) {
	if store == nil || client == nil || resolver == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub geospatial action dependencies are required")
	}
	return &GeospatialActionHandler{store: store, client: client, resolver: resolver, authSecret: authSecret}, nil
}

type geospatialActionPolicy struct{ capability, featureFlag string }

var geospatialActionPolicies = map[string]geospatialActionPolicy{
	"map-element-create": {"geospatial.write", FlightHubActionFeatureFlag},
	"map-element-update": {"geospatial.write", FlightHubActionFeatureFlag},
	"map-element-delete": {"geospatial.element.delete", FlightHubGeospatialDeleteFeatureFlag},
}

func parseGeospatialActionEvent(event outbox.Event) (string, error) {
	var payload struct {
		JobID string `json:"jobId"`
	}
	if event.ProjectID <= 0 || json.Unmarshal(event.Payload, &payload) != nil || strings.TrimSpace(payload.JobID) == "" {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return payload.JobID, nil
}

func (handler *GeospatialActionHandler) decryptRequest(job GeospatialActionJob) (GeospatialActionPayload, error) {
	var envelope credentials.Envelope
	if json.Unmarshal(job.RequestEnvelope, &envelope) != nil {
		return GeospatialActionPayload{}, errors.New("FlightHub geospatial action request unavailable")
	}
	var raw json.RawMessage
	if err := credentials.DecryptJSON(envelope, handler.authSecret,
		credentials.AAD("flighthub-geospatial-action", job.ID, job.ProjectID), &raw); err != nil {
		return GeospatialActionPayload{}, err
	}
	var payload GeospatialActionPayload
	switch job.ActionKind {
	case "map-element-create":
		var request MapElementCreateRequest
		if json.Unmarshal(raw, &request) != nil || validateMapElementCreate(&request) != nil {
			return payload, &APIError{SafeCode: "request_invalid"}
		}
		payload.Create = &request
	case "map-element-update":
		var request MapElementUpdateRequest
		if json.Unmarshal(raw, &request) != nil || validateMapElementUpdate(&request) != nil {
			return payload, &APIError{SafeCode: "request_invalid"}
		}
		payload.Update = &request
	case "map-element-delete":
		var request struct {
			Confirmation string `json:"confirmation"`
		}
		if json.Unmarshal(raw, &request) != nil || request.Confirmation != "DELETE" {
			return payload, &APIError{SafeCode: "request_invalid"}
		}
		payload.Confirmation = request.Confirmation
	default:
		return payload, &APIError{SafeCode: "request_invalid"}
	}
	return payload, nil
}

func (handler *GeospatialActionHandler) Handler(ctx context.Context, _ *sql.Tx, event outbox.Event) error {
	jobID, err := parseGeospatialActionEvent(event)
	if err != nil {
		return err
	}
	job, err := handler.store.Load(ctx, event.ProjectID, jobID)
	if err != nil {
		return err
	}
	policy, known := geospatialActionPolicies[job.ActionKind]
	if !known || policy.capability != job.CapabilityCode || policy.featureFlag != job.FeatureFlag {
		return handler.store.Fail(ctx, job, "request_invalid")
	}
	if job.Status == "executing" || job.AttemptCount > 0 {
		return handler.store.Block(ctx, job, "result_unknown_after_restart")
	}
	if job.Status != "queued" {
		return nil
	}
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
	if job.ActionKind != "map-element-create" {
		if !job.TargetResourceID.Valid || !job.ExpectedRemoteVersion.Valid || job.TargetKind != "map-element" ||
			job.TargetStatus != "active" || strings.TrimSpace(job.TargetRemoteID) == "" {
			return handler.store.Fail(ctx, job, "scope_forbidden")
		}
		if job.TargetRemoteVersion != job.ExpectedRemoteVersion.String {
			return handler.store.Fail(ctx, job, "version_conflict")
		}
	}
	payload, err := handler.decryptRequest(job)
	if err != nil {
		return handler.store.Fail(ctx, job, safeWorkflowCode(err))
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
	if err = handler.store.Begin(ctx, job); err != nil {
		if IsSafeCode(err, "version_conflict") {
			return handler.store.Fail(ctx, job, "version_conflict")
		}
		return err
	}
	remoteID := ""
	var deleted MapElementDeleteResult
	switch job.ActionKind {
	case "map-element-create":
		result, callErr := handler.client.CreateMapElement(ctx, token, scope.ProjectUUID, *payload.Create)
		remoteID, err = result.ID, callErr
	case "map-element-update":
		result, callErr := handler.client.UpdateWorkspaceMapElement(ctx, token, scope.ProjectUUID, job.TargetRemoteID, *payload.Update)
		remoteID, err = result.ID, callErr
	case "map-element-delete":
		deleted, err = handler.client.DeleteWorkspaceMapElement(ctx, token, scope.ProjectUUID, scope.ProjectUUID, job.TargetRemoteID)
	}
	if err != nil {
		code := safeWorkflowCode(err)
		if definitiveLiveActionError(code) {
			return handler.store.Fail(ctx, job, code)
		}
		return handler.store.Block(ctx, job, "write_result_unknown")
	}
	if job.ActionKind == "map-element-update" && remoteID != job.TargetRemoteID {
		return handler.store.Block(ctx, job, "write_result_mismatch")
	}
	return handler.store.Complete(ctx, job, payload, remoteID, deleted)
}
