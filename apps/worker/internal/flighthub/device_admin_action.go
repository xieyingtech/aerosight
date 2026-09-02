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

const FlightHubDeviceAdminEventType = "flighthub.device_admin.requested"

type DeviceAdminClient interface {
	ExecuteDeviceAdminControl(context.Context, string, string, string, string, json.RawMessage) (ControlActionDefinition, any, error)
	DecryptSNs(context.Context, string, string, SNDecryptRequest) (SNDecryptResult, error)
}

func (client *Client) ExecuteDeviceAdminControl(ctx context.Context, token, projectUUID, actionCode, deviceSN string, parameters json.RawMessage) (ControlActionDefinition, any, error) {
	var fields map[string]any
	if json.Unmarshal(parameters, &fields) != nil || fields == nil {
		return ControlActionDefinition{}, nil, &APIError{SafeCode: "request_invalid"}
	}
	fields["deviceSn"] = deviceSN
	input, err := json.Marshal(fields)
	if err != nil {
		return ControlActionDefinition{}, nil, err
	}
	request, err := BuildControlActionRequest(actionCode, input)
	if err != nil {
		return ControlActionDefinition{}, nil, err
	}
	payload, err := client.request(ctx, token, projectUUID, request.Spec)
	if err != nil {
		return request.Definition, nil, err
	}
	data := payload.Data
	if len(data) == 0 {
		data = json.RawMessage(`null`)
	}
	decoded, err := DecodeControlActionOutput(actionCode, data)
	return request.Definition, decoded, err
}

type deviceAdminJob struct {
	ID, ActionKind, CapabilityCode, FeatureFlag, Status, DeviceSN      string
	ProjectID, TeamID, RequestedByUserID                               int
	ConnectorInstanceID                                                int64
	DeviceID                                                           sql.NullInt64
	AttemptCount                                                       int
	Authorized, ConnectorConnected, FeatureEnabled, CapabilityVerified bool
	DeviceAvailable, ApprovalValid                                     bool
	RequestEnvelope                                                    json.RawMessage
	Instance                                                           connector.Instance
}

type DeviceAdminActionHandler struct {
	db         *sql.DB
	client     DeviceAdminClient
	resolver   TokenResolver
	authSecret string
}

func NewDeviceAdminActionHandler(db *sql.DB, client DeviceAdminClient, resolver TokenResolver, authSecret string) (*DeviceAdminActionHandler, error) {
	if db == nil || client == nil || resolver == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub device admin dependencies are required")
	}
	return &DeviceAdminActionHandler{db: db, client: client, resolver: resolver, authSecret: authSecret}, nil
}

var deviceAdminPolicies = map[string]struct{ capability, feature, actionCode string }{
	"rtk-calibrate":         {"device.rtk.calibrate", FlightHubRTKCalibrateFeatureFlag, "rtk.calibrate"},
	"relay-pair":            {"device.relay.pair", FlightHubRelayPairFeatureFlag, "relay.pair"},
	"active-project-update": {"device.active-project.update", FlightHubDeviceMigrationFeatureFlag, "active_project.update"},
	"sn-decrypt":            {"security.sn.decrypt", FlightHubSNDecryptFeatureFlag, ""},
}

func (handler *DeviceAdminActionHandler) load(ctx context.Context, projectID int, jobID string) (job deviceAdminJob, err error) {
	var request, credential, scope []byte
	err = handler.db.QueryRowContext(ctx, `select job.id::text,job.project_id,job.team_id,job.connector_instance_id,job.device_id,
		job.requested_by_user_id,job.action_kind,job.capability_code,job.feature_flag,job.status,job.attempt_count,
		job.request_envelope_json,coalesce(identity.identity_json#>>'{attributes,serialNumber}',''),adapter.credential_envelope_json,adapter.discovery_scope_json,
		member.role in('owner','admin'),adapter.status='connected',coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(job.feature_flag,true),false),
		exists(select 1 from connector_capability_snapshots capability where capability.project_id=job.project_id
		  and capability.connector_instance_id=job.connector_instance_id and capability.capability_code=job.capability_code
		  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		  and capability.region='cn' and capability.deployment='cn-public-cloud'
		  and capability.status='supported' and capability.evidence_level='field-write' and (capability.expires_at is null or capability.expires_at>now())
		  and ((job.device_id is null and capability.device_model is null and capability.firmware_version is null)
		    or (job.device_id is not null and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version))),
		case when job.device_id is null then true else device.status='online' and latest.captured_at>now()-interval '30 seconds' and latest.captured_at<=now()+interval '1 second' and identity.id is not null end,
		exists(select 1 from approval_requests approval where approval.id=job.approval_request_id and approval.project_id=job.project_id
		  and approval.team_id=job.team_id and approval.resource_type=case when job.device_id is null then 'connector' else 'device' end
		  and approval.resource_id=coalesce(job.device_id::text,job.connector_instance_id::text)
		  and approval.action='flighthub.admin.'||job.action_kind and approval.status='approved' and approval.expires_at>now())
	 from connector_device_admin_jobs job join device_adapters adapter on adapter.id=job.connector_instance_id and adapter.project_id=job.project_id
	 join connector_definitions definition on definition.id=adapter.connector_definition_id and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
	 join team_members member on member.team_id=job.team_id and member.user_id=job.requested_by_user_id
	 left join project_feature_flags flags on flags.project_id=job.project_id
	 left join devices device on device.id=job.device_id and device.project_id=job.project_id
	 left join device_external_identities identity on identity.project_id=job.project_id and identity.adapter_id=job.connector_instance_id and identity.device_id=job.device_id
	 left join device_latest_telemetry latest on latest.project_id=job.project_id and latest.adapter_id=job.connector_instance_id and latest.device_id=job.device_id
	 where job.project_id=$1 and job.id=$2::uuid`, projectID, jobID).Scan(
		&job.ID, &job.ProjectID, &job.TeamID, &job.ConnectorInstanceID, &job.DeviceID, &job.RequestedByUserID,
		&job.ActionKind, &job.CapabilityCode, &job.FeatureFlag, &job.Status, &job.AttemptCount, &request, &job.DeviceSN,
		&credential, &scope, &job.Authorized, &job.ConnectorConnected, &job.FeatureEnabled, &job.CapabilityVerified, &job.DeviceAvailable, &job.ApprovalValid)
	job.RequestEnvelope = request
	job.Instance = connector.Instance{ID: job.ConnectorInstanceID, ProjectID: job.ProjectID, ConnectorKey: ConnectorKey, Version: ConnectorVersion, CredentialEnvelope: credential, DiscoveryScope: scope}
	return job, err
}

func (handler *DeviceAdminActionHandler) finish(ctx context.Context, job deviceAdminJob, status, code string, result map[string]any, envelope any) error {
	resultRaw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	var envelopeRaw any
	if envelope != nil {
		encoded, encodeErr := json.Marshal(envelope)
		if encodeErr != nil {
			return encodeErr
		}
		envelopeRaw = encoded
	}
	_, err = handler.db.ExecContext(ctx, `update connector_device_admin_jobs set status=$3,last_error_code=nullif($4,''),result_json=$5,
		result_envelope_json=$6,unknown_at=case when $3='blocked' then now() else null end,
		completed_at=case when $3 in('succeeded','failed','blocked') then now() else null end,updated_at=now()
		where project_id=$1 and id=$2::uuid and status='executing'`, job.ProjectID, job.ID, status, code, resultRaw, envelopeRaw)
	return err
}

func (handler *DeviceAdminActionHandler) Handler(ctx context.Context, _ *sql.Tx, event outbox.Event) error {
	var payload struct {
		JobID string `json:"jobId"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil || strings.TrimSpace(payload.JobID) == "" {
		return &APIError{SafeCode: "request_invalid"}
	}
	job, err := handler.load(ctx, event.ProjectID, payload.JobID)
	if err != nil {
		return err
	}
	policy, ok := deviceAdminPolicies[job.ActionKind]
	if !ok || policy.capability != job.CapabilityCode || policy.feature != job.FeatureFlag || !job.Authorized || !job.ConnectorConnected || !job.FeatureEnabled || !job.CapabilityVerified || !job.DeviceAvailable || !job.ApprovalValid {
		if job.Status == "queued" {
			_, err = handler.db.ExecContext(ctx, `update connector_device_admin_jobs set status='failed',attempt_count=0,last_error_code='action_disabled',completed_at=now(),updated_at=now() where project_id=$1 and id=$2::uuid and status='queued'`, job.ProjectID, job.ID)
		}
		return err
	}
	if job.Status == "accepted" || job.Status == "succeeded" || job.Status == "failed" || job.Status == "blocked" {
		return nil
	}
	if job.Status == "executing" || job.AttemptCount > 0 {
		return handler.finish(ctx, job, "blocked", "result_unknown_after_restart", map[string]any{"final": false}, nil)
	}
	if job.Status != "queued" {
		return nil
	}
	var request map[string]any
	var requestEnvelope credentials.Envelope
	if json.Unmarshal(job.RequestEnvelope, &requestEnvelope) != nil || credentials.DecryptJSON(requestEnvelope, handler.authSecret, credentials.AAD("flighthub-device-admin-action", job.ID, job.ProjectID), &request) != nil {
		_, updateErr := handler.db.ExecContext(ctx, `update connector_device_admin_jobs set status='failed',last_error_code='request_unavailable',result_json='{"final":true}'::jsonb,completed_at=now(),updated_at=now() where project_id=$1 and id=$2::uuid and status='queued'`, job.ProjectID, job.ID)
		return updateErr
	}
	result, err := handler.db.ExecContext(ctx, `update connector_device_admin_jobs set status='executing',attempt_count=1,attempted_at=now(),updated_at=now() where project_id=$1 and id=$2::uuid and status='queued' and attempt_count=0`, job.ProjectID, job.ID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil
	}
	token, err := handler.resolver.ResolveToken(ctx, job.Instance)
	if err != nil {
		return handler.finish(ctx, job, "failed", safeWorkflowCode(err), map[string]any{"final": true}, nil)
	}
	defer func() { token = "" }()
	scope, err := parseScope(job.Instance.DiscoveryScope)
	if err != nil {
		return handler.finish(ctx, job, "failed", "scope_forbidden", map[string]any{"final": true}, nil)
	}
	if job.ActionKind == "sn-decrypt" {
		values, ok := request["encryptedSNs"].([]any)
		if !ok {
			return handler.finish(ctx, job, "failed", "request_invalid", map[string]any{"final": true}, nil)
		}
		sns := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return handler.finish(ctx, job, "failed", "request_invalid", map[string]any{"final": true}, nil)
			}
			sns = append(sns, text)
		}
		decrypted, callErr := handler.client.DecryptSNs(ctx, token, scope.ProjectUUID, SNDecryptRequest{EncryptedSNs: sns})
		if callErr != nil {
			return handler.handleError(ctx, job, callErr)
		}
		sealed, sealErr := credentials.EncryptJSON(map[string]any{"mapping": decrypted.Mapping}, handler.authSecret, credentials.AAD("flighthub-device-admin-result", job.ID, job.ProjectID))
		if sealErr != nil {
			return handler.finish(ctx, job, "blocked", "result_seal_failed", map[string]any{"final": false}, nil)
		}
		return handler.finish(ctx, job, "succeeded", "", map[string]any{"count": len(decrypted.Mapping), "final": true}, sealed)
	}
	raw, _ := json.Marshal(request)
	definition, output, callErr := handler.client.ExecuteDeviceAdminControl(ctx, token, scope.ProjectUUID, policy.actionCode, job.DeviceSN, raw)
	if callErr != nil {
		return handler.handleError(ctx, job, callErr)
	}
	resultJSON := map[string]any{"endpointId": definition.EndpointID, "accepted": true, "final": false}
	if rtk, ok := output.(RTKCalibrationOutput); ok {
		if rtk.Status != "ok" {
			return handler.finish(ctx, job, "failed", "upstream_rejected", map[string]any{"final": true}, nil)
		}
	}
	return handler.finish(ctx, job, "accepted", "", resultJSON, nil)
}

func (handler *DeviceAdminActionHandler) handleError(ctx context.Context, job deviceAdminJob, err error) error {
	code := safeWorkflowCode(err)
	var apiErr *APIError
	if errors.As(err, &apiErr) && definitiveControlNack(apiErr) {
		return handler.finish(ctx, job, "failed", code, map[string]any{"final": true}, nil)
	}
	return handler.finish(ctx, job, "blocked", "write_result_unknown", map[string]any{"final": false}, nil)
}
