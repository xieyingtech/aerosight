package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
)

const (
	FlightActionEventType        = "flighthub.flight_action.requested"
	maxActionReconciliationReads = 3
)

type FlightActionRequest struct {
	Name                       string                  `json:"name"`
	TimeZone                   string                  `json:"timeZone"`
	TaskType                   string                  `json:"taskType"`
	RTHAltitude                int                     `json:"rthAltitude"`
	RTHMode                    string                  `json:"rthMode"`
	OutOfControlActionInFlight string                  `json:"outOfControlActionInFlight"`
	WaylinePrecisionType       string                  `json:"waylinePrecisionType"`
	ResumableStatus            string                  `json:"resumableStatus"`
	RepeatType                 string                  `json:"repeatType"`
	RepeatOption               *FlightTaskRepeatOption `json:"repeatOption"`
	LandingDeviceID            int                     `json:"landingDeviceId"`
	BeginAt                    int64                   `json:"beginAt"`
	EndAt                      int64                   `json:"endAt"`
	RecurringTaskStartTimes    []int64                 `json:"recurringTaskStartTimes"`
	ContinuousTaskPeriods      [][]int64               `json:"continuousTaskPeriods"`
	MinimumBatteryCapacity     int                     `json:"minimumBatteryCapacity"`
	DesiredStatus              string                  `json:"desiredStatus"`
}

type FlightActionJob struct {
	ID                     string
	ProjectID              int
	TeamID                 int
	ConnectorInstanceID    int64
	TaskRunID              int
	DeviceID               int
	WaylineResourceID      sql.NullInt64
	TargetResourceID       sql.NullInt64
	RemoteResultResourceID sql.NullInt64
	ApprovalRequestID      string
	RequestedByUserID      int
	ActionKind             string
	RequestDigest          string
	RequestEnvelope        json.RawMessage
	Status                 string
	AttemptCount           int
	ReconciliationCount    int
	LastErrorCode          string
	DeviceExternalID       string
	WaylineRemoteID        string
	TargetRemoteID         string
	RemoteResultID         string
	ConnectorStatus        string
	ActionEnabled          bool
	CapabilityVerified     bool
	TaskRunStatus          string
	PreflightAllowed       bool
	ApprovalValid          bool
	ApprovalAction         string
	Instance               connector.Instance
}

type FlightActionStore interface {
	Load(context.Context, int, string) (FlightActionJob, error)
	ExternalDeviceID(context.Context, int, int64, int) (string, error)
	MarkPrepared(context.Context, FlightActionJob, map[string]any) error
	BeginAttempt(context.Context, FlightActionJob) error
	RecordAccepted(context.Context, FlightActionJob, string) error
	RecordError(context.Context, FlightActionJob, string) error
	RecordReconciliationRead(context.Context, FlightActionJob) (int, error)
	Complete(context.Context, FlightActionJob, FlightTask) error
	Fail(context.Context, FlightActionJob, string) error
	Block(context.Context, FlightActionJob, string) error
}

type SQLFlightActionStore struct{ db *sql.DB }

func NewSQLFlightActionStore(database *sql.DB) *SQLFlightActionStore {
	return &SQLFlightActionStore{db: database}
}

func (store *SQLFlightActionStore) Load(ctx context.Context, projectID int, jobID string) (job FlightActionJob, err error) {
	if store == nil || store.db == nil || projectID <= 0 || strings.TrimSpace(jobID) == "" {
		return job, &APIError{SafeCode: "request_invalid"}
	}
	var envelope []byte
	err = store.db.QueryRowContext(ctx, `select job.id::text,job.project_id,job.team_id,job.connector_instance_id,
		job.task_run_id,job.device_id,job.wayline_resource_id,job.target_resource_id,job.remote_result_resource_id,
		job.approval_request_id::text,job.requested_by_user_id,job.action_kind,job.request_digest,job.request_envelope_json,
		job.status,job.attempt_count,job.reconciliation_count,coalesce(job.last_error_code,''),
		coalesce(identity.identity_json#>>'{attributes,serialNumber}',''),coalesce(wayline.remote_id,''),coalesce(target.remote_id,''),coalesce(remote_result.remote_id,''),adapter.status,
		coalesce(flags.flighthub_action_flags_json @> '{"flight.execute":true}'::jsonb,false),
		exists(select 1 from connector_capability_snapshots capability
		  where capability.project_id=job.project_id and capability.connector_instance_id=job.connector_instance_id
		    and capability.capability_code='flight.execute' and capability.status='supported'
		    and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		    and capability.region='cn' and capability.deployment='cn-public-cloud'
		    and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version
		    and capability.evidence_level='field-write' and (capability.expires_at is null or capability.expires_at>now())),
		run.status,coalesce((run.preflight_snapshot_json->>'allowed')::boolean,false),
		(approval.status='approved' and approval.expires_at>now()
		  and approval.project_id=job.project_id and approval.team_id=job.team_id
		  and approval.resource_type='task_run' and approval.resource_id=job.task_run_id::text
		  and coalesce((approval.context_json#>>'{preflight,allowed}')::boolean,false)),approval.action,
		definition.connector_key,definition.version,adapter.credential_envelope_json,adapter.discovery_scope_json
	 from connector_action_jobs job
	 join device_adapters adapter on adapter.id=job.connector_instance_id and adapter.project_id=job.project_id
	 join connector_definitions definition on definition.id=adapter.connector_definition_id
	 join task_runs run on run.id=job.task_run_id and run.project_id=job.project_id and run.team_id=job.team_id
	 join devices device on device.id=job.device_id and device.project_id=job.project_id
	 join device_external_identities identity on identity.project_id=job.project_id and identity.adapter_id=job.connector_instance_id
	   and identity.device_id=job.device_id and run.selected_device_id=job.device_id
	 join team_members member on member.team_id=job.team_id and member.user_id=job.requested_by_user_id
	 join approval_requests approval on approval.id=job.approval_request_id and approval.project_id=job.project_id
	 left join project_feature_flags flags on flags.project_id=job.project_id
	 left join connector_remote_resources wayline on wayline.id=job.wayline_resource_id and wayline.project_id=job.project_id
	   and wayline.connector_instance_id=job.connector_instance_id and wayline.resource_kind='wayline' and wayline.status='active'
	 left join connector_remote_resources target on target.id=job.target_resource_id and target.project_id=job.project_id
	   and target.connector_instance_id=job.connector_instance_id and target.resource_kind='flight-task' and target.status='active'
	 left join connector_remote_resources remote_result on remote_result.id=job.remote_result_resource_id and remote_result.project_id=job.project_id
	   and remote_result.connector_instance_id=job.connector_instance_id and remote_result.resource_kind='flight-task'
	 where job.id=$1::uuid and job.project_id=$2
	   and (member.role in('owner','admin') or exists(select 1 from project_permissions permission
	     where permission.project_id=job.project_id and permission.team_id=job.team_id
	       and permission.user_id=job.requested_by_user_id and permission.permission='mission:operate'))`, jobID, projectID).Scan(
		&job.ID, &job.ProjectID, &job.TeamID, &job.ConnectorInstanceID, &job.TaskRunID, &job.DeviceID,
		&job.WaylineResourceID, &job.TargetResourceID, &job.RemoteResultResourceID, &job.ApprovalRequestID,
		&job.RequestedByUserID, &job.ActionKind, &job.RequestDigest, &envelope, &job.Status, &job.AttemptCount,
		&job.ReconciliationCount, &job.LastErrorCode, &job.DeviceExternalID, &job.WaylineRemoteID,
		&job.TargetRemoteID, &job.RemoteResultID, &job.ConnectorStatus, &job.ActionEnabled, &job.CapabilityVerified,
		&job.TaskRunStatus, &job.PreflightAllowed, &job.ApprovalValid, &job.ApprovalAction,
		&job.Instance.ConnectorKey, &job.Instance.Version, &job.Instance.CredentialEnvelope, &job.Instance.DiscoveryScope,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return job, &APIError{SafeCode: "scope_forbidden"}
	}
	job.RequestEnvelope = json.RawMessage(envelope)
	job.Instance.ID, job.Instance.ProjectID = job.ConnectorInstanceID, job.ProjectID
	return job, err
}

func (store *SQLFlightActionStore) ExternalDeviceID(ctx context.Context, projectID int, connectorID int64, deviceID int) (string, error) {
	var externalID string
	err := store.db.QueryRowContext(ctx, `select coalesce(identity_json#>>'{attributes,serialNumber}','') from device_external_identities
		where project_id=$1 and adapter_id=$2 and device_id=$3`, projectID, connectorID, deviceID).Scan(&externalID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", &APIError{SafeCode: "scope_forbidden"}
	}
	if err == nil && strings.TrimSpace(externalID) == "" {
		return "", &APIError{SafeCode: "scope_forbidden"}
	}
	return externalID, err
}

func (store *SQLFlightActionStore) update(ctx context.Context, query string, args ...any) error {
	result, err := store.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("FlightHub action state changed concurrently")
	}
	return nil
}

func (store *SQLFlightActionStore) MarkPrepared(ctx context.Context, job FlightActionJob, summary map[string]any) error {
	return store.update(ctx, `update connector_action_jobs set status='prepared',dispatch_check_json=$3,last_error_code=null,updated_at=now()
		where id=$1::uuid and project_id=$2 and status='queued'`, job.ID, job.ProjectID, summary)
}

func (store *SQLFlightActionStore) BeginAttempt(ctx context.Context, job FlightActionJob) error {
	return store.update(ctx, `update connector_action_jobs set status='reconciling',attempt_count=attempt_count+1,updated_at=now()
		where id=$1::uuid and project_id=$2 and status in('queued','prepared') and attempt_count=0`, job.ID, job.ProjectID)
}

func (store *SQLFlightActionStore) RecordAccepted(ctx context.Context, job FlightActionJob, remoteID string) (returnedErr error) {
	if strings.TrimSpace(remoteID) == "" {
		return store.update(ctx, `update connector_action_jobs set accepted_at=now(),last_error_code=null,updated_at=now()
			where id=$1::uuid and project_id=$2 and status='reconciling'`, job.ID, job.ProjectID)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	var resourceID int64
	err = tx.QueryRowContext(ctx, `insert into connector_remote_resources(
		project_id,team_id,connector_instance_id,resource_kind,remote_id,status,summary_json,canonical_target_type,canonical_target_id
	) values($1,$2,$3,'flight-task',$4,'active',$5,'task_run',$6)
	 on conflict(project_id,connector_instance_id,resource_kind,remote_id) do update set
		status='active',summary_json=connector_remote_resources.summary_json||excluded.summary_json,
		canonical_target_type='task_run',canonical_target_id=excluded.canonical_target_id,last_seen_at=now(),missing_at=null,updated_at=now()
	 returning id`, job.ProjectID, job.TeamID, job.ConnectorInstanceID, strings.TrimSpace(remoteID),
		map[string]any{"source": "governed-action", "confirmed": false}, fmt.Sprint(job.TaskRunID)).Scan(&resourceID)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `update connector_action_jobs set remote_result_resource_id=$3,accepted_at=now(),last_error_code=null,updated_at=now()
		where id=$1::uuid and project_id=$2 and status='reconciling'`, job.ID, job.ProjectID, resourceID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("FlightHub action acceptance state changed")
	}
	return tx.Commit()
}

func (store *SQLFlightActionStore) RecordError(ctx context.Context, job FlightActionJob, code string) error {
	return store.update(ctx, `update connector_action_jobs set last_error_code=$3,updated_at=now()
		where id=$1::uuid and project_id=$2 and status not in('succeeded','failed','blocked')`, job.ID, job.ProjectID, code)
}

func (store *SQLFlightActionStore) RecordReconciliationRead(ctx context.Context, job FlightActionJob) (int, error) {
	var count int
	err := store.db.QueryRowContext(ctx, `update connector_action_jobs
		set reconciliation_count=reconciliation_count+1,reconciled_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status='reconciling' and reconciliation_count<8
		returning reconciliation_count`, job.ID, job.ProjectID).Scan(&count)
	return count, err
}

func taskRunStatusFromRemote(remoteStatus string) string {
	switch strings.TrimSpace(remoteStatus) {
	case "executing":
		return "running"
	case "paused", "suspended":
		return "paused"
	case "success", "partially_done":
		return "succeeded"
	case "terminated", "starting_failure", "timeout":
		return "failed"
	default:
		return "dispatching"
	}
}

func (store *SQLFlightActionStore) Complete(ctx context.Context, job FlightActionJob, task FlightTask) (returnedErr error) {
	if strings.TrimSpace(task.UUID) == "" {
		return &APIError{SafeCode: "schema_incompatible"}
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	var resourceID int64
	err = tx.QueryRowContext(ctx, `insert into connector_remote_resources(
		project_id,team_id,connector_instance_id,resource_kind,remote_id,status,summary_json,canonical_target_type,canonical_target_id
	) values($1,$2,$3,'flight-task',$4,'active',$5,'task_run',$6)
	 on conflict(project_id,connector_instance_id,resource_kind,remote_id) do update set
		status='active',summary_json=excluded.summary_json,canonical_target_type='task_run',canonical_target_id=excluded.canonical_target_id,
		last_seen_at=now(),missing_at=null,updated_at=now()
	 returning id`, job.ProjectID, job.TeamID, job.ConnectorInstanceID, task.UUID,
		map[string]any{"source": "governed-action", "confirmed": true, "status": task.Status}, fmt.Sprint(job.TaskRunID)).Scan(&resourceID)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `update connector_action_jobs set status='succeeded',remote_result_resource_id=$3,
		last_error_code=null,reconciled_at=now(),completed_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status='reconciling'`, job.ID, job.ProjectID, resourceID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("FlightHub action completion state changed")
	}
	localStatus := taskRunStatusFromRemote(task.Status)
	_, err = tx.ExecContext(ctx, `update task_runs set status=$3,state_version=state_version+1,
		state_reason='flighthub_remote_reconciled',output_snapshot_json=output_snapshot_json||$4::jsonb,
		started_at=case when $3='running' then coalesce(started_at,now()) else started_at end,
		finished_at=case when $3 in('succeeded','failed','canceled') then coalesce(finished_at,now()) else finished_at end
		where id=$1 and project_id=$2 and status not in('succeeded','failed','canceled')`,
		job.TaskRunID, job.ProjectID, localStatus, map[string]any{"flighthub": map[string]any{"actionJobId": job.ID, "remoteConfirmed": true}})
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (store *SQLFlightActionStore) Fail(ctx context.Context, job FlightActionJob, code string) error {
	return store.update(ctx, `update connector_action_jobs set status='failed',last_error_code=$3,updated_at=now()
		where id=$1::uuid and project_id=$2 and status not in('succeeded','failed','blocked')`, job.ID, job.ProjectID, code)
}

func (store *SQLFlightActionStore) Block(ctx context.Context, job FlightActionJob, code string) (returnedErr error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	result, err := tx.ExecContext(ctx, `update connector_action_jobs set status='blocked',last_error_code=$3,unknown_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status='reconciling'`, job.ID, job.ProjectID, code)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("FlightHub action blocking state changed")
	}
	_, err = tx.ExecContext(ctx, `update task_runs set status='blocked',state_version=state_version+1,
		state_reason='flighthub_result_unknown',error_message=null,output_snapshot_json=output_snapshot_json||$3::jsonb
		where id=$1 and project_id=$2 and status not in('succeeded','failed','canceled')`, job.TaskRunID, job.ProjectID,
		map[string]any{"flighthub": map[string]any{"actionJobId": job.ID, "remoteResult": "unknown", "manualReviewRequired": true}})
	if err != nil {
		return err
	}
	return tx.Commit()
}

type FlightActionClient interface {
	CheckFlightTaskDispatch(context.Context, string, string, string, string) (FlightTaskDispatchCheck, error)
	CreateFlightTask(context.Context, string, string, FlightTaskCreateRequest) (FlightTaskCreateResult, error)
	UpdateFlightTaskStatus(context.Context, string, string, string, string) error
	CreateFlightTaskResumption(context.Context, string, string, string, string) (FlightTaskResumption, error)
	GetFlightTask(context.Context, string, string, string) (FlightTask, error)
	ListFlightTasks(context.Context, string, string, FlightTaskListOptions) ([]FlightTaskSummary, error)
	ListRecentFlightTasks(context.Context, string, string, []string) ([]FlightTaskSummary, error)
}

type FlightActionHandler struct {
	store      FlightActionStore
	client     FlightActionClient
	resolver   TokenResolver
	authSecret string
}

func NewFlightActionHandler(store FlightActionStore, client FlightActionClient, resolver TokenResolver, authSecret string) (*FlightActionHandler, error) {
	if store == nil || client == nil || resolver == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub action dependencies are required")
	}
	return &FlightActionHandler{store: store, client: client, resolver: resolver, authSecret: authSecret}, nil
}

func parseFlightActionEvent(event outbox.Event) (string, error) {
	var payload struct {
		JobID string `json:"jobId"`
	}
	if event.ProjectID <= 0 || json.Unmarshal(event.Payload, &payload) != nil || strings.TrimSpace(payload.JobID) == "" {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return payload.JobID, nil
}

func expectedApprovalAction(kind string) string {
	switch kind {
	case "flight-task-create":
		return "flighthub.flight-task.create"
	case "flight-task-status":
		return "flighthub.flight-task.status"
	case "flight-task-resumption":
		return "flighthub.flight-task.resume"
	default:
		return ""
	}
}

func (handler *FlightActionHandler) decryptRequest(job FlightActionJob) (FlightActionRequest, error) {
	var envelope credentials.Envelope
	if json.Unmarshal(job.RequestEnvelope, &envelope) != nil {
		return FlightActionRequest{}, errors.New("FlightHub action request is unavailable")
	}
	var request FlightActionRequest
	if err := credentials.DecryptJSON(envelope, handler.authSecret,
		credentials.AAD("flighthub-flight-action", job.ID, job.ProjectID), &request); err != nil {
		return FlightActionRequest{}, errors.New("FlightHub action request is unavailable")
	}
	return request, nil
}

func reconciledTaskName(requestedName, digest string) string {
	suffix := " · AeroSight-" + digest[:12]
	requestedName = strings.TrimSpace(requestedName)
	for len(requestedName)+len(suffix) > 200 {
		_, size := utf8.DecodeLastRuneInString(requestedName)
		if size == 0 {
			break
		}
		requestedName = requestedName[:len(requestedName)-size]
	}
	return requestedName + suffix
}

func terminalFlightActionError(code string) bool {
	switch code {
	case "credential_invalid", "scope_forbidden", "request_invalid", "configuration_required", "capability_not_supported", "schema_incompatible":
		return true
	default:
		return false
	}
}

func summaryToTask(summary FlightTaskSummary) FlightTask {
	return FlightTask{UUID: summary.UUID, Name: summary.Name, TaskType: summary.TaskType, Status: summary.Status,
		SN: summary.SN, WaylineUUID: summary.WaylineUUID, BeginAt: summary.BeginAt, EndAt: summary.EndAt}
}

func (handler *FlightActionHandler) Handler(ctx context.Context, _ *sql.Tx, event outbox.Event) error {
	jobID, err := parseFlightActionEvent(event)
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
	if job.ProjectID != event.ProjectID || job.TeamID != event.TeamID || job.Instance.ConnectorKey != ConnectorKey ||
		job.Instance.Version != ConnectorVersion || !isActiveConnectorStatus(job.ConnectorStatus) {
		return handler.store.Fail(ctx, job, "connector_disabled")
	}
	if !job.ActionEnabled || !job.CapabilityVerified {
		return handler.store.Fail(ctx, job, "action_disabled")
	}
	if !job.PreflightAllowed || !job.ApprovalValid || job.ApprovalAction != expectedApprovalAction(job.ActionKind) {
		return handler.store.Fail(ctx, job, "governance_revoked")
	}
	if strings.TrimSpace(job.DeviceExternalID) == "" ||
		(job.ActionKind == "flight-task-create" && strings.TrimSpace(job.WaylineRemoteID) == "") ||
		(job.ActionKind != "flight-task-create" && strings.TrimSpace(job.TargetRemoteID) == "") {
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
		code := safeWorkflowCode(err)
		if persistErr := handler.store.RecordError(ctx, job, code); persistErr != nil {
			return persistErr
		}
		return retryableWorkflowError(code)
	}
	defer func() { token = "" }()

	if job.Status == "reconciling" {
		return handler.reconcile(ctx, job, request, token, scope.ProjectUUID)
	}
	if job.ActionKind == "flight-task-create" && job.Status == "queued" {
		check, err := handler.client.CheckFlightTaskDispatch(ctx, token, scope.ProjectUUID, job.DeviceExternalID, job.WaylineRemoteID)
		if err != nil {
			code := safeWorkflowCode(err)
			if terminalFlightActionError(code) {
				return handler.store.Fail(ctx, job, code)
			}
			_ = handler.store.RecordError(ctx, job, code)
			return retryableWorkflowError(code)
		}
		codes := make([]string, 0, len(check.Warnings))
		hasWarning := false
		for _, warning := range check.Warnings {
			codes = append(codes, warning.Code)
			hasWarning = hasWarning || warning.Type == "warning"
		}
		if hasWarning {
			return handler.store.Fail(ctx, job, "dispatch_check_warning")
		}
		if err := handler.store.MarkPrepared(ctx, job, map[string]any{
			"passed": true, "warningCodes": codes, "devicePositionPresent": check.DevicePosition != nil,
		}); err != nil {
			return err
		}
		job.Status = "prepared"
	}
	if err := handler.store.BeginAttempt(ctx, job); err != nil {
		return err
	}
	job.Status = "reconciling"
	job.AttemptCount = 1
	remoteID := ""
	switch job.ActionKind {
	case "flight-task-create":
		landingSN := ""
		if request.LandingDeviceID > 0 {
			landingSN, err = handler.store.ExternalDeviceID(ctx, job.ProjectID, job.ConnectorInstanceID, request.LandingDeviceID)
			if err != nil {
				return handler.store.Fail(ctx, job, "scope_forbidden")
			}
		}
		result, callErr := handler.client.CreateFlightTask(ctx, token, scope.ProjectUUID, FlightTaskCreateRequest{
			Name: reconciledTaskName(request.Name, job.RequestDigest), SN: job.DeviceExternalID, WaylineUUID: job.WaylineRemoteID,
			TimeZone: request.TimeZone, TaskType: request.TaskType, RTHAltitude: request.RTHAltitude, RTHMode: request.RTHMode,
			OutOfControlActionInFlight: request.OutOfControlActionInFlight, WaylinePrecisionType: request.WaylinePrecisionType,
			ResumableStatus: request.ResumableStatus, RepeatType: request.RepeatType, RepeatOption: request.RepeatOption,
			LandingDockSN: landingSN, BeginAt: request.BeginAt, EndAt: request.EndAt,
			RecurringTaskStartTimes: request.RecurringTaskStartTimes, ContinuousTaskPeriods: request.ContinuousTaskPeriods,
			MinimumBatteryCapacity: request.MinimumBatteryCapacity,
		})
		if callErr != nil {
			return handler.afterUnknownWrite(ctx, job, callErr)
		}
		remoteID = result.TaskUUID
	case "flight-task-status":
		if err := handler.client.UpdateFlightTaskStatus(ctx, token, scope.ProjectUUID, job.TargetRemoteID, request.DesiredStatus); err != nil {
			return handler.afterUnknownWrite(ctx, job, err)
		}
		remoteID = job.TargetRemoteID
	case "flight-task-resumption":
		result, callErr := handler.client.CreateFlightTaskResumption(ctx, token, scope.ProjectUUID, scope.ProjectUUID, job.TargetRemoteID)
		if callErr != nil {
			return handler.afterUnknownWrite(ctx, job, callErr)
		}
		if result.Task.ParentTask == nil || result.Task.ParentTask.UUID != job.TargetRemoteID {
			return handler.store.Fail(ctx, job, "schema_incompatible")
		}
		remoteID = result.Task.UUID
	default:
		return handler.store.Fail(ctx, job, "request_invalid")
	}
	if err := handler.store.RecordAccepted(ctx, job, remoteID); err != nil {
		return err
	}
	return retryableWorkflowError("reconciliation_pending")
}

func (handler *FlightActionHandler) afterUnknownWrite(ctx context.Context, job FlightActionJob, err error) error {
	code := safeWorkflowCode(err)
	if terminalFlightActionError(code) {
		return handler.store.Fail(ctx, job, code)
	}
	if persistErr := handler.store.RecordError(ctx, job, code); persistErr != nil {
		return persistErr
	}
	return retryableWorkflowError(code)
}

func statusActionConfirmed(desired, remote string) bool {
	if desired == "suspended" {
		return remote == "suspended" || remote == "paused"
	}
	return desired == "restored" && (remote == "executing" || remote == "waiting" || remote == "preparing" || remote == "queue_for_takeoff")
}

func (handler *FlightActionHandler) reconcile(ctx context.Context, job FlightActionJob, request FlightActionRequest, token, projectUUID string) error {
	if job.RemoteResultResourceID.Valid {
		remoteID := job.TargetRemoteID
		if job.ActionKind != "flight-task-status" {
			var err error
			remoteID, err = handler.remoteResultID(ctx, job)
			if err != nil {
				return err
			}
		}
		task, err := handler.client.GetFlightTask(ctx, token, projectUUID, remoteID)
		if err == nil {
			if job.ActionKind != "flight-task-status" || statusActionConfirmed(request.DesiredStatus, task.Status) {
				return handler.store.Complete(ctx, job, task)
			}
		} else if terminalFlightActionError(safeWorkflowCode(err)) && !IsSafeCode(err, "scope_not_found") {
			return handler.store.Fail(ctx, job, safeWorkflowCode(err))
		}
		return handler.recordUnknownRead(ctx, job)
	}

	var candidates []FlightTaskSummary
	var err error
	switch job.ActionKind {
	case "flight-task-create":
		candidates, err = handler.client.ListFlightTasks(ctx, token, projectUUID, FlightTaskListOptions{
			SNs: []string{job.DeviceExternalID}, Name: reconciledTaskName(request.Name, job.RequestDigest),
		})
		if err == nil {
			expectedName := reconciledTaskName(request.Name, job.RequestDigest)
			filtered := candidates[:0]
			for _, candidate := range candidates {
				if candidate.Name == expectedName && candidate.SN == job.DeviceExternalID && candidate.WaylineUUID == job.WaylineRemoteID {
					filtered = append(filtered, candidate)
				}
			}
			candidates = filtered
		}
	case "flight-task-resumption":
		parent, parentErr := handler.client.GetFlightTask(ctx, token, projectUUID, job.TargetRemoteID)
		if parentErr != nil {
			err = parentErr
			break
		}
		candidates, err = handler.client.ListRecentFlightTasks(ctx, token, projectUUID, []string{job.DeviceExternalID})
		if err == nil {
			filtered := candidates[:0]
			for _, candidate := range candidates {
				if candidate.BreakPointResume && candidate.UUID != job.TargetRemoteID && candidate.SN == job.DeviceExternalID &&
					candidate.WaylineUUID == parent.WaylineUUID {
					filtered = append(filtered, candidate)
				}
			}
			candidates = filtered
		}
	case "flight-task-status":
		task, readErr := handler.client.GetFlightTask(ctx, token, projectUUID, job.TargetRemoteID)
		if readErr == nil && statusActionConfirmed(request.DesiredStatus, task.Status) {
			return handler.store.Complete(ctx, job, task)
		}
		err = readErr
	}
	if err != nil {
		code := safeWorkflowCode(err)
		if persistErr := handler.store.RecordError(ctx, job, code); persistErr != nil {
			return persistErr
		}
		return retryableWorkflowError(code)
	}
	if len(candidates) > 1 {
		return handler.store.Block(ctx, job, "reconciliation_ambiguous")
	}
	if len(candidates) == 1 {
		task := summaryToTask(candidates[0])
		if err := handler.store.RecordAccepted(ctx, job, task.UUID); err != nil {
			return err
		}
		return handler.store.Complete(ctx, job, task)
	}
	return handler.recordUnknownRead(ctx, job)
}

func (handler *FlightActionHandler) remoteResultID(ctx context.Context, job FlightActionJob) (string, error) {
	loaded, err := handler.store.Load(ctx, job.ProjectID, job.ID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(loaded.RemoteResultID) != "" {
		return loaded.RemoteResultID, nil
	}
	return loaded.TargetRemoteID, nil
}

func (handler *FlightActionHandler) recordUnknownRead(ctx context.Context, job FlightActionJob) error {
	count, err := handler.store.RecordReconciliationRead(ctx, job)
	if err != nil {
		return err
	}
	if count >= maxActionReconciliationReads {
		return handler.store.Block(ctx, job, "remote_result_unknown")
	}
	return retryableWorkflowError("reconciliation_pending")
}

var _ FlightActionStore = (*SQLFlightActionStore)(nil)
