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

const FlightHubLiveActionEventType = "flighthub.live_action.requested"

type LiveActionRequest struct {
	CameraIndex  string      `json:"cameraIndex"`
	QualityType  LiveQuality `json:"qualityType"`
	Name         string      `json:"name"`
	Schema       string      `json:"schema"`
	SchemaOption struct {
		URL            string `json:"url"`
		ServerIP       string `json:"serverIp"`
		ServerPort     string `json:"serverPort"`
		DevicePassword string `json:"devicePassword"`
		LocalPort      string `json:"localPort"`
		DeviceID       string `json:"deviceId"`
		LocalChannel   string `json:"localChannel"`
		Username       string `json:"username"`
		Password       string `json:"password"`
		EnableTS       *bool  `json:"enableTs"`
	} `json:"schemaOption"`
	Enabled bool `json:"enabled"`
}

type LiveActionJob struct {
	ID                  string
	ProjectID           int
	TeamID              int
	ConnectorInstanceID int64
	DeviceID            sql.NullInt64
	TargetResourceID    sql.NullInt64
	ActionKind          string
	CapabilityCode      string
	FeatureFlag         string
	RequestEnvelope     json.RawMessage
	Status              string
	AttemptCount        int
	DeviceExternalID    string
	TargetRemoteID      string
	TargetKind          string
	TargetStatus        string
	ConnectorStatus     string
	Authorized          bool
	ActionEnabled       bool
	CapabilityVerified  bool
	Instance            connector.Instance
}

type LiveActionStore interface {
	Load(context.Context, int, string) (LiveActionJob, error)
	Begin(context.Context, LiveActionJob) error
	Complete(context.Context, LiveActionJob, LiveActionRequest, string) error
	Fail(context.Context, LiveActionJob, string) error
	Block(context.Context, LiveActionJob, string) error
}

type SQLLiveActionStore struct{ db *sql.DB }

func NewSQLLiveActionStore(database *sql.DB) *SQLLiveActionStore {
	return &SQLLiveActionStore{db: database}
}

func (store *SQLLiveActionStore) Load(ctx context.Context, projectID int, jobID string) (job LiveActionJob, err error) {
	if store == nil || store.db == nil || projectID <= 0 || strings.TrimSpace(jobID) == "" {
		return job, &APIError{SafeCode: "request_invalid"}
	}
	var envelope []byte
	err = store.db.QueryRowContext(ctx, `select job.id::text,job.project_id,job.team_id,job.connector_instance_id,
		job.device_id,job.target_resource_id,job.action_kind,job.capability_code,job.feature_flag,
		job.request_envelope_json,job.status,job.attempt_count,
		coalesce(identity.identity_json#>>'{attributes,serialNumber}',''),coalesce(target.remote_id,''),
		coalesce(target.resource_kind,''),coalesce(target.status,''),adapter.status,
		case when job.action_kind='live-converter-delete' then member.role in('owner','admin')
		  else member.role in('owner','admin') or exists(select 1 from project_permissions permission
		    where permission.project_id=job.project_id and permission.team_id=job.team_id
		      and permission.user_id=job.requested_by_user_id and permission.permission='mission:operate') end,
		coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(job.feature_flag,true),false),
		exists(select 1 from connector_capability_snapshots capability
		  where capability.project_id=job.project_id and capability.connector_instance_id=job.connector_instance_id
		    and capability.capability_code=job.capability_code and capability.status='supported'
		    and capability.evidence_level='field-write' and (capability.expires_at is null or capability.expires_at>now())
		    and ((job.action_kind='live-quality-set' and capability.device_model=device.device_model
		          and capability.firmware_version=device.firmware_version)
		      or (job.action_kind<>'live-quality-set' and capability.device_model is null and capability.firmware_version is null))),
		definition.connector_key,definition.version,adapter.credential_envelope_json,adapter.discovery_scope_json
	 from connector_live_action_jobs job
	 join device_adapters adapter on adapter.id=job.connector_instance_id and adapter.project_id=job.project_id
	 join connector_definitions definition on definition.id=adapter.connector_definition_id
	 join team_members member on member.team_id=job.team_id and member.user_id=job.requested_by_user_id
	 left join project_feature_flags flags on flags.project_id=job.project_id
	 left join devices device on device.id=job.device_id and device.project_id=job.project_id
	 left join device_external_identities identity on identity.project_id=job.project_id
	   and identity.adapter_id=job.connector_instance_id and identity.device_id=job.device_id
	 left join connector_remote_resources target on target.id=job.target_resource_id and target.project_id=job.project_id
	   and target.connector_instance_id=job.connector_instance_id
	 where job.id=$1::uuid and job.project_id=$2`, jobID, projectID).Scan(
		&job.ID, &job.ProjectID, &job.TeamID, &job.ConnectorInstanceID, &job.DeviceID, &job.TargetResourceID,
		&job.ActionKind, &job.CapabilityCode, &job.FeatureFlag, &envelope, &job.Status, &job.AttemptCount,
		&job.DeviceExternalID, &job.TargetRemoteID, &job.TargetKind, &job.TargetStatus, &job.ConnectorStatus, &job.Authorized,
		&job.ActionEnabled, &job.CapabilityVerified, &job.Instance.ConnectorKey, &job.Instance.Version,
		&job.Instance.CredentialEnvelope, &job.Instance.DiscoveryScope,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return job, &APIError{SafeCode: "scope_forbidden"}
	}
	job.RequestEnvelope = json.RawMessage(envelope)
	job.Instance.ID, job.Instance.ProjectID = job.ConnectorInstanceID, job.ProjectID
	return job, err
}

func (store *SQLLiveActionStore) update(ctx context.Context, statement string, args ...any) error {
	result, err := store.db.ExecContext(ctx, statement, args...)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("FlightHub live action state changed concurrently")
	}
	return nil
}

func (store *SQLLiveActionStore) Begin(ctx context.Context, job LiveActionJob) error {
	return store.update(ctx, `update connector_live_action_jobs set status='executing',attempt_count=1,attempted_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status='queued' and attempt_count=0`, job.ID, job.ProjectID)
}

func (store *SQLLiveActionStore) Complete(ctx context.Context, job LiveActionJob, request LiveActionRequest, remoteID string) (returnedErr error) {
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
	case "live-quality-set":
		result["qualityType"] = request.QualityType
	case "live-converter-create":
		if strings.TrimSpace(remoteID) == "" {
			return &APIError{SafeCode: "schema_incompatible"}
		}
		var resourceID int64
		err = tx.QueryRowContext(ctx, `insert into connector_remote_resources(
			project_id,team_id,connector_instance_id,resource_kind,remote_id,status,summary_json
		) values($1,$2,$3,'stream-converter',$4,'active',$5)
		 on conflict(project_id,connector_instance_id,resource_kind,remote_id) do update set
			status='active',summary_json=connector_remote_resources.summary_json||excluded.summary_json,
			last_seen_at=now(),missing_at=null,updated_at=now() returning id`,
			job.ProjectID, job.TeamID, job.ConnectorInstanceID, strings.TrimSpace(remoteID),
			map[string]any{"source": "governed-action", "confirmed": true}).Scan(&resourceID)
		if err != nil {
			return err
		}
		result["remoteResourceId"] = resourceID
	case "live-converter-toggle":
		changed, updateErr := tx.ExecContext(ctx, `update connector_remote_resources set
			summary_json=summary_json||$3::jsonb,last_seen_at=now(),updated_at=now()
			where id=$1 and project_id=$2 and connector_instance_id=$4 and resource_kind='stream-converter' and status='active'`,
			job.TargetResourceID.Int64, job.ProjectID, map[string]any{"enabled": request.Enabled, "source": "governed-action"}, job.ConnectorInstanceID)
		if updateErr != nil {
			return updateErr
		}
		if count, _ := changed.RowsAffected(); count != 1 {
			return &APIError{SafeCode: "scope_forbidden"}
		}
		result["enabled"] = request.Enabled
	case "live-converter-delete":
		changed, updateErr := tx.ExecContext(ctx, `update connector_remote_resources set status='missing',missing_at=now(),updated_at=now()
			where id=$1 and project_id=$2 and connector_instance_id=$3 and resource_kind='stream-converter' and status='active'`,
			job.TargetResourceID.Int64, job.ProjectID, job.ConnectorInstanceID)
		if updateErr != nil {
			return updateErr
		}
		if count, _ := changed.RowsAffected(); count != 1 {
			return &APIError{SafeCode: "scope_forbidden"}
		}
		result["deleted"] = true
	default:
		return &APIError{SafeCode: "request_invalid"}
	}
	changed, err := tx.ExecContext(ctx, `update connector_live_action_jobs set status='succeeded',result_json=$3,
		last_error_code=null,completed_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status='executing'`, job.ID, job.ProjectID, result)
	if err != nil {
		return err
	}
	if count, _ := changed.RowsAffected(); count != 1 {
		return errors.New("FlightHub live action completion state changed")
	}
	return tx.Commit()
}

func (store *SQLLiveActionStore) Fail(ctx context.Context, job LiveActionJob, code string) error {
	return store.update(ctx, `update connector_live_action_jobs set status='failed',last_error_code=$3,completed_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status in('queued','executing')`, job.ID, job.ProjectID, code)
}

func (store *SQLLiveActionStore) Block(ctx context.Context, job LiveActionJob, code string) error {
	return store.update(ctx, `update connector_live_action_jobs set status='blocked',last_error_code=$3,unknown_at=now(),completed_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status in('queued','executing')`, job.ID, job.ProjectID, code)
}

type LiveActionClient interface {
	SetStreamQuality(context.Context, string, string, StreamQualityRequest) error
	CreateStreamConverter(context.Context, string, string, StreamConverterCreateRequest) (StreamConverterCreateResult, error)
	SetStreamConverterEnabled(context.Context, string, string, string, bool) error
	DeleteStreamConverter(context.Context, string, string, string) error
}

type LiveActionHandler struct {
	store      LiveActionStore
	client     LiveActionClient
	resolver   TokenResolver
	authSecret string
}

func NewLiveActionHandler(store LiveActionStore, client LiveActionClient, resolver TokenResolver, authSecret string) (*LiveActionHandler, error) {
	if store == nil || client == nil || resolver == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub live action dependencies are required")
	}
	return &LiveActionHandler{store: store, client: client, resolver: resolver, authSecret: authSecret}, nil
}

type liveActionPolicy struct{ capability, featureFlag string }

var liveActionPolicies = map[string]liveActionPolicy{
	"live-quality-set":      {"live.quality.set", FlightHubLiveQualityFeatureFlag},
	"live-converter-create": {"live.converter.create", FlightHubLiveConverterCreateFeatureFlag},
	"live-converter-toggle": {"live.converter.toggle", FlightHubLiveConverterToggleFeatureFlag},
	"live-converter-delete": {"live.converter.delete", FlightHubLiveConverterDeleteFeatureFlag},
}

func parseLiveActionEvent(event outbox.Event) (string, error) {
	var payload struct {
		JobID string `json:"jobId"`
	}
	if event.ProjectID <= 0 || json.Unmarshal(event.Payload, &payload) != nil || strings.TrimSpace(payload.JobID) == "" {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return payload.JobID, nil
}

func (handler *LiveActionHandler) decryptRequest(job LiveActionJob) (LiveActionRequest, error) {
	var envelope credentials.Envelope
	if json.Unmarshal(job.RequestEnvelope, &envelope) != nil {
		return LiveActionRequest{}, errors.New("FlightHub live action request unavailable")
	}
	var request LiveActionRequest
	err := credentials.DecryptJSON(envelope, handler.authSecret, credentials.AAD("flighthub-live-action", job.ID, job.ProjectID), &request)
	return request, err
}

func definitiveLiveActionError(code string) bool {
	switch code {
	case "credential_invalid", "scope_forbidden", "request_invalid", "configuration_required", "capability_not_supported", "schema_incompatible", "rate_limited":
		return true
	default:
		return false
	}
}

func (handler *LiveActionHandler) Handler(ctx context.Context, _ *sql.Tx, event outbox.Event) error {
	jobID, err := parseLiveActionEvent(event)
	if err != nil {
		return err
	}
	job, err := handler.store.Load(ctx, event.ProjectID, jobID)
	if err != nil {
		return err
	}
	policy, known := liveActionPolicies[job.ActionKind]
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
	if job.ActionKind == "live-quality-set" || job.ActionKind == "live-converter-create" {
		if !job.DeviceID.Valid || strings.TrimSpace(job.DeviceExternalID) == "" {
			return handler.store.Fail(ctx, job, "scope_forbidden")
		}
	} else if !job.TargetResourceID.Valid || job.TargetKind != "stream-converter" || job.TargetStatus != "active" || strings.TrimSpace(job.TargetRemoteID) == "" {
		return handler.store.Fail(ctx, job, "scope_forbidden")
	}
	request, err := handler.decryptRequest(job)
	if err != nil {
		return handler.store.Fail(ctx, job, "request_unavailable")
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
		return err
	}
	remoteID := ""
	switch job.ActionKind {
	case "live-quality-set":
		err = handler.client.SetStreamQuality(ctx, token, scope.ProjectUUID, StreamQualityRequest{SN: job.DeviceExternalID, CameraIndex: request.CameraIndex, QualityType: request.QualityType})
	case "live-converter-create":
		result, callErr := handler.client.CreateStreamConverter(ctx, token, scope.ProjectUUID, StreamConverterCreateRequest{
			Name: request.Name, SN: job.DeviceExternalID, CameraIndex: request.CameraIndex, Schema: request.Schema,
			SchemaOption: StreamConverterSchemaOption{URL: request.SchemaOption.URL, ServerIP: request.SchemaOption.ServerIP,
				ServerPort: request.SchemaOption.ServerPort, DevicePassword: request.SchemaOption.DevicePassword,
				LocalPort: request.SchemaOption.LocalPort, DeviceID: request.SchemaOption.DeviceID, LocalChannel: request.SchemaOption.LocalChannel,
				Username: request.SchemaOption.Username, Password: request.SchemaOption.Password, EnableTS: request.SchemaOption.EnableTS},
		})
		err, remoteID = callErr, result.ID
	case "live-converter-toggle":
		err = handler.client.SetStreamConverterEnabled(ctx, token, scope.ProjectUUID, job.TargetRemoteID, request.Enabled)
	case "live-converter-delete":
		err = handler.client.DeleteStreamConverter(ctx, token, scope.ProjectUUID, job.TargetRemoteID)
	}
	if err != nil {
		code := safeWorkflowCode(err)
		if definitiveLiveActionError(code) {
			return handler.store.Fail(ctx, job, code)
		}
		return handler.store.Block(ctx, job, "write_result_unknown")
	}
	return handler.store.Complete(ctx, job, request, remoteID)
}
