package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/outbox"
)

const FlightHubControlSessionEventType = "flighthub.control_session.reconcile"

type ControlSelection struct {
	Flight       bool     `json:"flight"`
	PayloadIndex []string `json:"payloadIndex"`
}

type FlightHubControlSession struct {
	ID                                             string
	ProjectID, TeamID, DeviceID, HolderUserID      int
	ConnectorInstanceID                            int64
	Status, DeviceSN, ConnectorStatus, FailureCode string
	Controls                                       ControlSelection
	AcquireAttemptCount, ReleaseAttemptCount       int
	LeaseExpiresAt, AbsoluteExpiresAt              time.Time
	FeatureEnabled, CapabilityVerified             bool
	DeviceOnline, StateFresh, ApprovalValid        bool
	SafetyPolicyCurrent, PermissionCurrent         bool
	Instance                                       connector.Instance
	ProjectUUID                                    string
}

type ControlOwnershipClient interface {
	ExecuteControlOwnership(context.Context, string, string, string, string, ControlSelection) (ControlOwnershipOutput, error)
}

func (client *Client) ExecuteControlOwnership(ctx context.Context, token, projectUUID, actionCode, deviceSN string, controls ControlSelection) (ControlOwnershipOutput, error) {
	input, err := json.Marshal(controlOwnershipInput{DroneSN: deviceSN, Flight: controls.Flight, PayloadIndex: controls.PayloadIndex})
	if err != nil {
		return ControlOwnershipOutput{}, err
	}
	request, err := BuildControlActionRequest(actionCode, input)
	if err != nil {
		return ControlOwnershipOutput{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, request.Spec)
	if err != nil {
		return ControlOwnershipOutput{}, err
	}
	decoded, err := DecodeControlActionOutput(actionCode, payload.Data)
	if err != nil {
		return ControlOwnershipOutput{}, err
	}
	output, ok := decoded.(ControlOwnershipOutput)
	if !ok {
		return ControlOwnershipOutput{}, schemaError()
	}
	return output, nil
}

type SQLControlSessionStore struct{ database *sql.DB }

type ControlSessionStore interface {
	Load(context.Context, int, string, time.Time) (FlightHubControlSession, error)
	BeginAcquire(context.Context, FlightHubControlSession, time.Time) (bool, error)
	Activate(context.Context, FlightHubControlSession, time.Time) error
	QueueRelease(context.Context, FlightHubControlSession, string, time.Time) error
	BeginRelease(context.Context, FlightHubControlSession, time.Time) (bool, error)
	RecordReleaseUnconfirmed(context.Context, FlightHubControlSession, string, time.Time) error
	Finish(context.Context, FlightHubControlSession, string, string, time.Time) error
	Sweep(context.Context, time.Time, int) (int, error)
}

func NewSQLControlSessionStore(database *sql.DB) *SQLControlSessionStore {
	return &SQLControlSessionStore{database: database}
}

func (store *SQLControlSessionStore) Load(ctx context.Context, projectID int, sessionID string, now time.Time) (FlightHubControlSession, error) {
	if store == nil || store.database == nil || projectID <= 0 || strings.TrimSpace(sessionID) == "" {
		return FlightHubControlSession{}, &APIError{SafeCode: "request_invalid"}
	}
	var session FlightHubControlSession
	var controls, credential, scope []byte
	err := store.database.QueryRowContext(ctx, `select session.id::text,session.project_id,session.team_id,session.device_id,
		session.holder_user_id,session.connector_instance_id,session.status,session.acquire_attempt_count,
		session.release_attempt_count,session.lease_expires_at,session.absolute_expires_at,coalesce(session.failure_code,''),
		identity.identity_json#>>'{attributes,serialNumber}',adapter.status,adapter.credential_envelope_json,
		adapter.discovery_scope_json,session.controls_json,
		coalesce(flags.flighthub_action_flags_json @> '{"device.control":true}'::jsonb,false),
		exists(select 1 from connector_capability_snapshots capability where capability.project_id=session.project_id
		  and capability.connector_instance_id=session.connector_instance_id and capability.capability_code='device.control'
		  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		  and capability.region='cn' and capability.deployment='cn-public-cloud'
		  and capability.status='supported' and capability.evidence_level='field-write'
		  and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version
		  and (capability.expires_at is null or capability.expires_at>$3)),
		device.status='online',coalesce(latest.captured_at>$3-interval '30 seconds' and latest.captured_at<=$3+interval '1 second',false),
		exists(select 1 from approval_requests approval where approval.id=session.approval_request_id
		  and approval.project_id=session.project_id and approval.team_id=session.team_id
		  and approval.resource_type='device' and approval.resource_id=session.device_id::text
		  and approval.action='flighthub.control.acquire' and approval.status='approved' and approval.expires_at>$3),
		project.current_safety_policy_version_id=session.safety_policy_version_id,
		exists(select 1 from team_members member where member.team_id=session.team_id and member.user_id=session.holder_user_id
		  and (member.role in('owner','admin') or exists(select 1 from project_permissions permission
		    where permission.project_id=session.project_id and permission.team_id=session.team_id
		      and permission.user_id=session.holder_user_id and permission.permission='mission:operate')))
	 from connector_control_sessions session
	 join projects project on project.id=session.project_id and project.team_id=session.team_id
	 join devices device on device.id=session.device_id and device.project_id=session.project_id
	 join device_adapters adapter on adapter.id=session.connector_instance_id and adapter.project_id=session.project_id
	 join connector_definitions definition on definition.id=adapter.connector_definition_id
	 join device_external_identities identity on identity.project_id=session.project_id and identity.adapter_id=adapter.id and identity.device_id=device.id
	 left join project_feature_flags flags on flags.project_id=session.project_id
	 left join device_latest_telemetry latest on latest.project_id=session.project_id and latest.device_id=device.id and latest.adapter_id=adapter.id
	 where session.project_id=$1 and session.id=$2::uuid and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'`,
		projectID, sessionID, now.UTC()).Scan(
		&session.ID, &session.ProjectID, &session.TeamID, &session.DeviceID, &session.HolderUserID,
		&session.ConnectorInstanceID, &session.Status, &session.AcquireAttemptCount, &session.ReleaseAttemptCount,
		&session.LeaseExpiresAt, &session.AbsoluteExpiresAt, &session.FailureCode, &session.DeviceSN,
		&session.ConnectorStatus, &credential, &scope, &controls, &session.FeatureEnabled,
		&session.CapabilityVerified, &session.DeviceOnline, &session.StateFresh, &session.ApprovalValid,
		&session.SafetyPolicyCurrent, &session.PermissionCurrent)
	if err != nil {
		return session, err
	}
	selection, err := decodeControlSelection(controls)
	if err != nil {
		return session, errors.New("DJI_FLIGHTHUB_CONTROL_SESSION_INVALID")
	}
	session.Controls = selection
	parsedScope, err := parseScope(scope)
	if err != nil {
		return session, err
	}
	session.ProjectUUID = parsedScope.ProjectUUID
	session.Instance = connector.Instance{
		ID: session.ConnectorInstanceID, ProjectID: session.ProjectID, ConnectorKey: ConnectorKey, Version: ConnectorVersion,
		CredentialEnvelope: credential, DiscoveryScope: scope,
	}
	return session, nil
}

func decodeControlSelection(raw json.RawMessage) (ControlSelection, error) {
	var fields map[string]json.RawMessage
	var selection ControlSelection
	if json.Unmarshal(raw, &fields) != nil || fields == nil || len(fields) == 0 || len(fields) > 2 {
		return selection, errors.New("invalid control selection")
	}
	for name := range fields {
		if name != "flight" && name != "payloadIndex" {
			return selection, errors.New("invalid control selection")
		}
	}
	if json.Unmarshal(raw, &selection) != nil || (!selection.Flight && len(selection.PayloadIndex) == 0) || len(selection.PayloadIndex) > 32 {
		return selection, errors.New("invalid control selection")
	}
	seen := make(map[string]bool, len(selection.PayloadIndex))
	for _, index := range selection.PayloadIndex {
		if _, err := controlIdentifier(index, false); err != nil || seen[index] {
			return selection, errors.New("invalid control selection")
		}
		seen[index] = true
	}
	sort.Strings(selection.PayloadIndex)
	return selection, nil
}

func (store *SQLControlSessionStore) BeginAcquire(ctx context.Context, session FlightHubControlSession, now time.Time) (bool, error) {
	result, err := store.database.ExecContext(ctx, `update connector_control_sessions set status='acquiring',acquire_attempt_count=1,updated_at=$3
		where project_id=$1 and id=$2::uuid and status='requested' and acquire_attempt_count=0`, session.ProjectID, session.ID, now.UTC())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (store *SQLControlSessionStore) Activate(ctx context.Context, session FlightHubControlSession, now time.Time) error {
	_, err := store.database.ExecContext(ctx, `update connector_control_sessions set status='active',acquired_at=$3,failure_code=null,updated_at=$3
		where project_id=$1 and id=$2::uuid and status='acquiring' and acquire_attempt_count=1`, session.ProjectID, session.ID, now.UTC())
	return err
}

func (store *SQLControlSessionStore) QueueRelease(ctx context.Context, session FlightHubControlSession, code string, now time.Time) error {
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `update connector_control_sessions set status='releasing',failure_code=nullif($3,''),
		release_requested_at=coalesce(release_requested_at,$4),updated_at=$4
		where project_id=$1 and id=$2::uuid and status in('requested','acquiring','active')`, session.ProjectID, session.ID, code, now.UTC())
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 && session.Status != "releasing" {
		return tx.Commit()
	}
	payload, _ := json.Marshal(map[string]any{"sessionId": session.ID})
	_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
		values($1,$2,$3,$4,'connector_control_session',$5,$6::jsonb,8) on conflict(event_id) do nothing`,
		session.ProjectID, session.TeamID, "flighthub-control-session:"+session.ID+":auto-release", FlightHubControlSessionEventType, session.ID, payload)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (store *SQLControlSessionStore) BeginRelease(ctx context.Context, session FlightHubControlSession, now time.Time) (bool, error) {
	result, err := store.database.ExecContext(ctx, `update connector_control_sessions set release_attempt_count=1,updated_at=$3
		where project_id=$1 and id=$2::uuid and status='releasing' and release_attempt_count=0`, session.ProjectID, session.ID, now.UTC())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (store *SQLControlSessionStore) RecordReleaseUnconfirmed(ctx context.Context, session FlightHubControlSession, code string, now time.Time) error {
	_, err := store.database.ExecContext(ctx, `update connector_control_sessions set failure_code=$3,updated_at=$4
		where project_id=$1 and id=$2::uuid and status='releasing' and release_attempt_count=1`,
		session.ProjectID, session.ID, code, now.UTC())
	return err
}

func (store *SQLControlSessionStore) Finish(ctx context.Context, session FlightHubControlSession, status, code string, now time.Time) error {
	_, err := store.database.ExecContext(ctx, `update connector_control_sessions set status=$3,failure_code=nullif($4,''),
		released_at=$5,lease_expires_at=least(lease_expires_at,$5),updated_at=$5
		where project_id=$1 and id=$2::uuid and status in('requested','acquiring','releasing')`,
		session.ProjectID, session.ID, status, code, now.UTC())
	return err
}

func (store *SQLControlSessionStore) Sweep(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `select session.id::text,session.project_id,session.team_id,
		case when session.absolute_expires_at<=$1 then 'maximum_duration_reached'
		     when session.lease_expires_at<=$1 then 'heartbeat_expired' else 'permission_revoked' end
	 from connector_control_sessions session
	 where session.status='active' and (session.lease_expires_at<=$1 or session.absolute_expires_at<=$1 or not exists(
		select 1 from team_members member where member.team_id=session.team_id and member.user_id=session.holder_user_id
		  and (member.role in('owner','admin') or exists(select 1 from project_permissions permission
		    where permission.project_id=session.project_id and permission.team_id=session.team_id
		      and permission.user_id=session.holder_user_id and permission.permission='mission:operate'))))
	 order by session.lease_expires_at limit $2 for update skip locked`, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	type expiredSession struct {
		id, reason        string
		projectID, teamID int
	}
	sessions := make([]expiredSession, 0)
	for rows.Next() {
		var session expiredSession
		if err := rows.Scan(&session.id, &session.projectID, &session.teamID, &session.reason); err != nil {
			rows.Close()
			return 0, err
		}
		sessions = append(sessions, session)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, session := range sessions {
		if _, err := tx.ExecContext(ctx, `update connector_control_sessions set status='releasing',failure_code=$3,
			release_requested_at=coalesce(release_requested_at,$4),updated_at=$4 where project_id=$1 and id=$2::uuid and status='active'`,
			session.projectID, session.id, session.reason, now.UTC()); err != nil {
			return 0, err
		}
		payload, _ := json.Marshal(map[string]any{"sessionId": session.id})
		if _, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
			values($1,$2,$3,$4,'connector_control_session',$5,$6::jsonb,8) on conflict(event_id) do nothing`,
			session.projectID, session.teamID, "flighthub-control-session:"+session.id+":auto-release",
			FlightHubControlSessionEventType, session.id, payload); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(sessions), nil
}

type ControlSessionHandler struct {
	store    ControlSessionStore
	client   ControlOwnershipClient
	resolver TokenResolver
	now      func() time.Time
}

func NewControlSessionHandler(store ControlSessionStore, client ControlOwnershipClient, resolver TokenResolver, now func() time.Time) (*ControlSessionHandler, error) {
	if store == nil || client == nil || resolver == nil {
		return nil, errors.New("DJI_FLIGHTHUB_CONTROL_SESSION_DEPENDENCY_REQUIRED")
	}
	if now == nil {
		now = time.Now
	}
	return &ControlSessionHandler{store: store, client: client, resolver: resolver, now: now}, nil
}

func (handler *ControlSessionHandler) Handler(ctx context.Context, _ *sql.Tx, event outbox.Event) error {
	var payload struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil || strings.TrimSpace(payload.SessionID) == "" {
		return errors.New("DJI_FLIGHTHUB_CONTROL_SESSION_EVENT_INVALID")
	}
	now := handler.now().UTC()
	session, err := handler.store.Load(ctx, event.ProjectID, payload.SessionID, now)
	if err != nil {
		return err
	}
	switch session.Status {
	case "requested":
		return handler.acquire(ctx, session, now)
	case "acquiring":
		return handler.store.QueueRelease(ctx, session, "acquire_interrupted", now)
	case "releasing":
		return handler.release(ctx, session, now)
	default:
		return nil
	}
}

func (handler *ControlSessionHandler) acquire(ctx context.Context, session FlightHubControlSession, now time.Time) error {
	if session.ConnectorStatus != "connected" || !session.FeatureEnabled || !session.CapabilityVerified ||
		!session.DeviceOnline || !session.StateFresh || !session.ApprovalValid || !session.SafetyPolicyCurrent || !session.PermissionCurrent ||
		!now.Before(session.LeaseExpiresAt) || !now.Before(session.AbsoluteExpiresAt) {
		return handler.store.Finish(ctx, session, "failed", "safety_gate_failed", now)
	}
	begun, err := handler.store.BeginAcquire(ctx, session, now)
	if err != nil || !begun {
		return err
	}
	session.Status, session.AcquireAttemptCount = "acquiring", 1
	token, err := handler.resolver.ResolveToken(ctx, session.Instance)
	if err != nil {
		return handler.store.Finish(ctx, session, "failed", "credential_unavailable", now)
	}
	output, callErr := handler.client.ExecuteControlOwnership(ctx, token, session.ProjectUUID, "control.acquire", session.DeviceSN, session.Controls)
	token = ""
	if callErr != nil {
		var apiError *APIError
		if errors.As(callErr, &apiError) && definitiveControlNack(apiError) {
			return handler.store.Finish(ctx, session, "failed", apiError.SafeCode, now)
		}
		return handler.store.QueueRelease(ctx, session, "acquire_result_unknown", now)
	}
	if !controlOwnershipMatches(output, session.DeviceSN, session.Controls) {
		return handler.store.QueueRelease(ctx, session, "acquire_evidence_mismatch", now)
	}
	return handler.store.Activate(ctx, session, now)
}

func (handler *ControlSessionHandler) release(ctx context.Context, session FlightHubControlSession, now time.Time) error {
	if session.ReleaseAttemptCount > 0 {
		return handler.store.RecordReleaseUnconfirmed(ctx, session, "release_interrupted", now)
	}
	begun, err := handler.store.BeginRelease(ctx, session, now)
	if err != nil || !begun {
		return err
	}
	token, err := handler.resolver.ResolveToken(ctx, session.Instance)
	if err != nil {
		return handler.store.RecordReleaseUnconfirmed(ctx, session, "release_unconfirmed", now)
	}
	output, callErr := handler.client.ExecuteControlOwnership(ctx, token, session.ProjectUUID, "control.release", session.DeviceSN, session.Controls)
	token = ""
	if callErr != nil || output.DroneSN != session.DeviceSN {
		return handler.store.RecordReleaseUnconfirmed(ctx, session, "release_unconfirmed", now)
	}
	return handler.store.Finish(ctx, session, "released", "", now)
}

func controlOwnershipMatches(output ControlOwnershipOutput, deviceSN string, controls ControlSelection) bool {
	if output.DroneSN != deviceSN {
		return false
	}
	flight := !controls.Flight
	payloads := make(map[string]bool, len(controls.PayloadIndex))
	for _, index := range controls.PayloadIndex {
		payloads[index] = false
	}
	for _, control := range output.Controls {
		switch control.Type {
		case "flight":
			flight = true
		case "payload":
			if _, requested := payloads[control.PayloadIndex]; requested {
				payloads[control.PayloadIndex] = true
			}
		}
	}
	if !flight {
		return false
	}
	for _, present := range payloads {
		if !present {
			return false
		}
	}
	return true
}

func (handler *ControlSessionHandler) Run(ctx context.Context, interval time.Duration, onError func(error)) error {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := handler.store.Sweep(ctx, handler.now().UTC(), 100); err != nil && ctx.Err() == nil && onError != nil {
				onError(errors.New("DJI_FLIGHTHUB_CONTROL_SESSION_SWEEP_FAILED"))
			}
		}
	}
}

func (session FlightHubControlSession) String() string {
	return fmt.Sprintf("FlightHubControlSession{project:%d,status:%s}", session.ProjectID, session.Status)
}
