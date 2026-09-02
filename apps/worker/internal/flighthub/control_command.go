package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/outbox"
)

const controlStateFreshness = 30 * time.Second

type DiscreteControlClient interface {
	ExecuteDeviceControl(context.Context, string, string, string, string, json.RawMessage) (ControlActionDefinition, error)
}

type ControlCommandDispatcher struct {
	client   DiscreteControlClient
	resolver TokenResolver
	now      func() time.Time
}

type controlCommandPolicy struct {
	actionCode, capabilityCode, connectorCapabilityCode, featureFlag, approvalAction string
	deviceTypes                                                                      map[string]bool
}

var discreteControlPolicies = map[string]controlCommandPolicy{
	"return_home":         {actionCode: "return_home", capabilityCode: "flight.return_home", connectorCapabilityCode: "device.control", featureFlag: "device.control", approvalAction: "flighthub.device.return_home"},
	"return_home_cancel":  {actionCode: "return_home_cancel", capabilityCode: "flight.return_home", connectorCapabilityCode: "device.control", featureFlag: "device.control", approvalAction: "flighthub.device.return_home_cancel"},
	"flighttask_pause":    {actionCode: "flighttask_pause", capabilityCode: "mission.execute", connectorCapabilityCode: "device.control", featureFlag: "device.control", approvalAction: "flighthub.device.flighttask_pause"},
	"flighttask_recovery": {actionCode: "flighttask_recovery", capabilityCode: "mission.execute", connectorCapabilityCode: "device.control", featureFlag: "device.control", approvalAction: "flighthub.device.flighttask_recovery"},
	"camera.change": {actionCode: "camera.change", capabilityCode: "camera.change", connectorCapabilityCode: "device.camera.change", featureFlag: FlightHubCameraChangeFeatureFlag,
		approvalAction: "flighthub.device.camera.change", deviceTypes: map[string]bool{"dji.dock2": true, "dji.dock3": true}},
	"camera.change_lens": {actionCode: "camera.change_lens", capabilityCode: "camera.lens.change", connectorCapabilityCode: "device.lens.change", featureFlag: FlightHubLensChangeFeatureFlag,
		approvalAction: "flighthub.device.camera.change_lens", deviceTypes: map[string]bool{"dji.matrice3d": true, "dji.matrice3td": true, "dji.matrice4d": true, "dji.matrice4td": true}},
}

type loadedControlCommand struct {
	ID, CommandKey, CapabilityCode, DeviceSN             string
	RecordedConnectorCapabilityCode, RecordedFeatureFlag string
	DeviceTypeKey, DeviceModel, FirmwareVersion          string
	ProjectID, TeamID, DeviceID, Priority                int
	AdapterID                                            int64
	Deadline                                             time.Time
	Instance                                             connector.Instance
	ProjectUUID                                          string
	ConnectorStatus                                      string
	FeatureEnabled, CapabilityVerified                   bool
	Parameters                                           json.RawMessage
	DeviceOnline, StateFresh, ApprovalValid              bool
	SafetyPolicyCurrent                                  bool
}

func NewControlCommandDispatcher(client DiscreteControlClient, resolver TokenResolver, now func() time.Time) (*ControlCommandDispatcher, error) {
	if client == nil || resolver == nil {
		return nil, errors.New("DJI_FLIGHTHUB_CONTROL_DEPENDENCY_REQUIRED")
	}
	if now == nil {
		now = time.Now
	}
	return &ControlCommandDispatcher{client: client, resolver: resolver, now: now}, nil
}

func (client *Client) ExecuteDiscreteControl(ctx context.Context, token, projectUUID, actionCode, deviceSN string) (ControlActionDefinition, error) {
	return client.ExecuteDeviceControl(ctx, token, projectUUID, actionCode, deviceSN, json.RawMessage(`{}`))
}

func (client *Client) ExecuteDeviceControl(ctx context.Context, token, projectUUID, actionCode, deviceSN string, parameters json.RawMessage) (ControlActionDefinition, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return ControlActionDefinition{}, err
	}
	input := json.RawMessage(fmt.Sprintf(`{"deviceSn":%q}`, deviceSN))
	if actionCode == "camera.change" || actionCode == "camera.change_lens" {
		var fields map[string]any
		if json.Unmarshal(parameters, &fields) != nil || fields == nil {
			return ControlActionDefinition{}, &APIError{SafeCode: "request_invalid"}
		}
		fields["sn"] = deviceSN
		input, err = json.Marshal(fields)
		if err != nil {
			return ControlActionDefinition{}, err
		}
	}
	request, err := BuildControlActionRequest(actionCode, input)
	if err != nil {
		return ControlActionDefinition{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, request.Spec)
	if err != nil {
		return request.Definition, err
	}
	data := payload.Data
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	if _, err := DecodeControlActionOutput(actionCode, data); err != nil {
		return request.Definition, err
	}
	return request.Definition, nil
}

func (dispatcher *ControlCommandDispatcher) DispatchHandler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var payload struct {
		CommandID string `json:"commandId"`
	}
	if tx == nil || json.Unmarshal(event.Payload, &payload) != nil || strings.TrimSpace(payload.CommandID) == "" {
		return errors.New("DJI_FLIGHTHUB_COMMAND_EVENT_INVALID")
	}
	command, err := loadControlCommand(ctx, tx, event.ProjectID, payload.CommandID, dispatcher.now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		_, cancelErr := tx.ExecContext(ctx, `update device_commands set status='canceled',completed_at=now(),
			result_json='{"safeCode":"safety_scope_changed","final":true}'::jsonb
			where project_id=$1 and id=$2::uuid and status='dispatchable'
			  and safety_context_json->>'connectorKey'='dji.flighthub2'`, event.ProjectID, payload.CommandID)
		if cancelErr != nil {
			return cancelErr
		}
		return nil
	}
	if err != nil {
		return err
	}
	policy, known := discreteControlPolicies[command.CommandKey]
	gateCode := controlCommandGate(command, policy, known, dispatcher.now().UTC())
	if gateCode == "deadline_expired" {
		return timeoutControlCommand(ctx, tx, command)
	}
	if gateCode != "" {
		return blockControlCommand(ctx, tx, command, gateCode)
	}
	if err := beginControlAttempt(ctx, tx, command); err != nil {
		return err
	}
	token, err := dispatcher.resolver.ResolveToken(ctx, command.Instance)
	if err != nil {
		return finishControlAttempt(ctx, tx, command, "transport_error", "unknown", "credential_unavailable")
	}
	defer func() { token = "" }()
	definition, callErr := invokeGovernedDeviceControl(ctx, dispatcher.client, token, command, policy, dispatcher.now().UTC())
	if callErr != nil {
		var apiError *APIError
		if errors.As(callErr, &apiError) && definitiveControlNack(apiError) {
			return finishControlAttempt(ctx, tx, command, "nacked", "nacked", apiError.SafeCode)
		}
		return finishControlAttempt(ctx, tx, command, "transport_error", "unknown", safeControlError(callErr))
	}
	result := map[string]any{"endpointId": definition.EndpointID, "resultSemantics": definition.ResultSemantics, "final": false}
	resultJSON, _ := json.Marshal(result)
	if _, err := tx.ExecContext(ctx, `update command_attempts set result_json=$3::jsonb
		where project_id=$1 and command_id=$2::uuid and attempt=1 and status='sent'`, command.ProjectID, command.ID, resultJSON); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `update device_commands set status='sent',result_json=$3::jsonb
		where project_id=$1 and id=$2::uuid and status='sent'`, command.ProjectID, command.ID, resultJSON)
	return err
}

func controlCommandGate(command loadedControlCommand, policy controlCommandPolicy, known bool, now time.Time) string {
	if !known || policy.capabilityCode != command.CapabilityCode || policy.connectorCapabilityCode != command.RecordedConnectorCapabilityCode ||
		policy.featureFlag != command.RecordedFeatureFlag {
		return "policy_mismatch"
	}
	if command.ConnectorStatus != "connected" || !command.FeatureEnabled || !command.CapabilityVerified ||
		!validControlCommandParameters(command.CommandKey, command.Parameters) || !command.DeviceOnline || !command.StateFresh ||
		!command.ApprovalValid || !command.SafetyPolicyCurrent {
		return "safety_gate_failed"
	}
	if len(policy.deviceTypes) > 0 && !policy.deviceTypes[command.DeviceTypeKey] {
		return "model_unsupported"
	}
	if command.CapabilityCode == "flight.return_home" && command.Priority < 90 {
		return "priority_too_low"
	}
	if !now.Before(command.Deadline) {
		return "deadline_expired"
	}
	return ""
}

func invokeGovernedDeviceControl(ctx context.Context, client DiscreteControlClient, token string, command loadedControlCommand, policy controlCommandPolicy, now time.Time) (ControlActionDefinition, error) {
	if code := controlCommandGate(command, policy, true, now); code != "" {
		return ControlActionDefinition{}, &APIError{SafeCode: "request_invalid"}
	}
	return client.ExecuteDeviceControl(ctx, token, command.ProjectUUID, policy.actionCode, command.DeviceSN, command.Parameters)
}

func validControlCommandParameters(commandKey string, raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	identifierField := func(name string, required bool) bool {
		value, exists := fields[name]
		if !exists {
			return !required
		}
		var decoded string
		if json.Unmarshal(value, &decoded) != nil {
			return false
		}
		_, err := controlIdentifier(decoded, false)
		return err == nil
	}
	switch commandKey {
	case "camera.change":
		if len(fields) < 1 || len(fields) > 2 {
			return false
		}
		for key := range fields {
			if key != "cameraIndex" && key != "cameraPosition" {
				return false
			}
		}
		return identifierField("cameraIndex", true) && identifierField("cameraPosition", false)
	case "camera.change_lens":
		if len(fields) != 2 {
			return false
		}
		return identifierField("cameraIndex", true) && identifierField("lensType", true)
	default:
		return len(fields) == 0
	}
}

func loadControlCommand(ctx context.Context, tx *sql.Tx, projectID int, commandID string, now time.Time) (command loadedControlCommand, err error) {
	var credential, scope []byte
	err = tx.QueryRowContext(ctx, `select command.id::text,command.project_id,command.team_id,command.device_id,
		command.command_key,command.capability_code,command.priority,command.deadline_at,identity.identity_json#>>'{attributes,serialNumber}',
		command.safety_context_json->>'connectorCapabilityCode',command.safety_context_json->>'featureFlag',
		device_type.type_key,coalesce(device.device_model,''),coalesce(device.firmware_version,''),command.parameters_json,
		adapter.id,adapter.status,adapter.credential_envelope_json,adapter.discovery_scope_json,
		coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(coalesce(command.safety_context_json->>'featureFlag',''),true),false),
		exists(select 1 from connector_capability_snapshots capability where capability.project_id=command.project_id
		  and capability.connector_instance_id=adapter.id
		  and capability.capability_code=command.safety_context_json->>'connectorCapabilityCode'
		  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		  and capability.region='cn' and capability.deployment='cn-public-cloud'
		  and capability.status='supported' and capability.evidence_level='field-write'
		  and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version
		  and (capability.expires_at is null or capability.expires_at>$3)),
		(device.status='online'),coalesce(latest.captured_at>$3-interval '30 seconds' and latest.captured_at<=$3+interval '1 second',false),
		exists(select 1 from approval_requests approval where approval.id::text=command.safety_context_json->>'approvalRequestId'
		  and approval.project_id=command.project_id and approval.team_id=command.team_id and approval.resource_type='device'
		  and approval.resource_id=command.device_id::text and approval.action='flighthub.device.'||command.command_key
		  and approval.status='approved' and approval.expires_at>$3),
		(project.current_safety_policy_version_id is not null and project.current_safety_policy_version_id::text=command.safety_context_json->>'safetyPolicyVersionId')
	 from device_commands command
	 join devices device on device.id=command.device_id and device.project_id=command.project_id
	 join device_types device_type on device_type.id=device.device_type_id
	 join projects project on project.id=command.project_id and project.team_id=command.team_id
	 join device_connector_bindings binding on binding.device_id=device.id and binding.project_id=device.project_id and binding.status='active'
	 join device_adapters adapter on adapter.id=binding.connector_instance_id and adapter.project_id=binding.project_id
	 join connector_definitions definition on definition.id=adapter.connector_definition_id
	 join device_external_identities identity on identity.id=binding.external_identity_id and identity.project_id=binding.project_id and identity.adapter_id=adapter.id
	 left join project_feature_flags flags on flags.project_id=command.project_id
	 left join device_latest_telemetry latest on latest.device_id=device.id and latest.project_id=device.project_id and latest.adapter_id=adapter.id
	 where command.project_id=$1 and command.id=$2::uuid and command.status='dispatchable'
	   and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
	   and adapter.id::text=command.safety_context_json->>'connectorInstanceId'
	 order by binding.priority desc limit 1 for update of command`, projectID, commandID, now).Scan(
		&command.ID, &command.ProjectID, &command.TeamID, &command.DeviceID, &command.CommandKey, &command.CapabilityCode,
		&command.Priority, &command.Deadline, &command.DeviceSN, &command.RecordedConnectorCapabilityCode, &command.RecordedFeatureFlag,
		&command.DeviceTypeKey, &command.DeviceModel, &command.FirmwareVersion,
		&command.Parameters, &command.AdapterID, &command.ConnectorStatus, &credential, &scope,
		&command.FeatureEnabled, &command.CapabilityVerified, &command.DeviceOnline, &command.StateFresh,
		&command.ApprovalValid, &command.SafetyPolicyCurrent)
	if err != nil {
		return command, err
	}
	command.Instance = connector.Instance{ID: command.AdapterID, ProjectID: command.ProjectID, ConnectorKey: ConnectorKey, Version: ConnectorVersion, CredentialEnvelope: credential, DiscoveryScope: scope}
	parsedScope, err := parseScope(scope)
	if err != nil {
		return command, err
	}
	command.ProjectUUID = parsedScope.ProjectUUID
	return command, nil
}

func beginControlAttempt(ctx context.Context, tx *sql.Tx, command loadedControlCommand) error {
	result, err := tx.ExecContext(ctx, `update device_commands set status='sent',result_json='{"accepted":false,"final":false}'::jsonb
		where project_id=$1 and id=$2::uuid and status='dispatchable'`, command.ProjectID, command.ID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("DJI_FLIGHTHUB_COMMAND_STATE_CHANGED")
	}
	_, err = tx.ExecContext(ctx, `insert into command_attempts(project_id,team_id,command_id,adapter_id,attempt,status)
		values($1,$2,$3::uuid,$4,1,'sent') on conflict(command_id,attempt) do nothing`, command.ProjectID, command.TeamID, command.ID, command.AdapterID)
	return err
}

func finishControlAttempt(ctx context.Context, tx *sql.Tx, command loadedControlCommand, attemptStatus, commandStatus, code string) error {
	completed := commandStatus == "nacked" || commandStatus == "unknown"
	_, err := tx.ExecContext(ctx, `update command_attempts set status=$3,error_code=$4,
		acknowledged_at=case when $3='nacked' then now() else null end,result_json=jsonb_build_object('safeCode',$4,'final',false)
		where project_id=$1 and command_id=$2::uuid and attempt=1 and status='sent'`, command.ProjectID, command.ID, attemptStatus, code)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `update device_commands set status=$3,result_json=jsonb_build_object('safeCode',$4,'final',false),
		completed_at=case when $5 then now() else null end where project_id=$1 and id=$2::uuid and status='sent'`, command.ProjectID, command.ID, commandStatus, code, completed)
	return err
}

func blockControlCommand(ctx context.Context, tx *sql.Tx, command loadedControlCommand, code string) error {
	_, err := tx.ExecContext(ctx, `update device_commands set status='canceled',completed_at=now(),result_json=jsonb_build_object('safeCode',$3,'final',true)
		where project_id=$1 and id=$2::uuid and status='dispatchable'`, command.ProjectID, command.ID, code)
	return err
}

func timeoutControlCommand(ctx context.Context, tx *sql.Tx, command loadedControlCommand) error {
	_, err := tx.ExecContext(ctx, `update device_commands set status='timed_out',completed_at=now(),result_json='{"safeCode":"deadline_expired","final":true}'::jsonb
		where project_id=$1 and id=$2::uuid and status='dispatchable'`, command.ProjectID, command.ID)
	return err
}

func safeControlError(err error) string {
	var apiError *APIError
	if errors.As(err, &apiError) {
		return apiError.SafeCode
	}
	return "result_unknown"
}

func definitiveControlNack(apiError *APIError) bool {
	if apiError == nil || apiError.Retryable || apiError.HTTPStatus == 0 {
		return false
	}
	switch apiError.SafeCode {
	case "request_invalid", "credential_invalid", "scope_forbidden", "scope_not_found", "configuration_required", "upstream_error":
		return true
	default:
		return false
	}
}

type ControlTelemetryEvidence struct {
	CapturedAt       time.Time
	ReturnHomeActive *bool
}

func ReconcileDiscreteControl(actionCode string, acceptedAt, deadline, now time.Time, evidence ControlTelemetryEvidence) (string, string) {
	if evidence.CapturedAt.After(acceptedAt) {
		var observed, wanted *bool
		switch actionCode {
		case "return_home":
			observed, wanted = evidence.ReturnHomeActive, boolPointer(true)
		}
		if observed != nil && wanted != nil && *observed == *wanted {
			return "acknowledged", "telemetry_confirmed"
		}
	}
	if !now.Before(deadline) {
		return "unknown", "deadline_without_fresh_evidence"
	}
	return "sent", "awaiting_fresh_evidence"
}

func boolPointer(value bool) *bool { return &value }

type ControlEvidenceProjector interface {
	ApplyControlEvidence(context.Context, connector.Instance, DeviceStatePoll) error
}

type SQLControlCommandReconciler struct {
	database *sql.DB
	now      func() time.Time
}

func NewSQLControlCommandReconciler(database *sql.DB, now func() time.Time) (*SQLControlCommandReconciler, error) {
	if database == nil {
		return nil, errors.New("DJI_FLIGHTHUB_CONTROL_DATABASE_REQUIRED")
	}
	if now == nil {
		now = time.Now
	}
	return &SQLControlCommandReconciler{database: database, now: now}, nil
}

func (reconciler *SQLControlCommandReconciler) ApplyControlEvidence(ctx context.Context, instance connector.Instance, poll DeviceStatePoll) error {
	if instance.ID <= 0 || instance.ProjectID <= 0 || poll.Device.DeviceID <= 0 || poll.Mapped.Mode != "auto_returning_to_home" {
		return nil
	}
	capturedAt, _ := stateCapturedAt(poll.Snapshot.State, poll.ReceivedAt)
	tx, err := reconciler.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `with acknowledged as (
		update device_commands command set status='acknowledged',completed_at=$4,
			result_json=command.result_json||'{"evidence":"device_state.mode_code:auto_returning_to_home","final":true}'::jsonb
		where command.project_id=$1 and command.device_id=$2 and command.status='sent'
		  and command.command_key='return_home'
		  and command.safety_context_json->>'connectorKey'='dji.flighthub2'
		  and command.safety_context_json->>'connectorInstanceId'=$3
		  and exists(select 1 from command_attempts attempt where attempt.command_id=command.id
		    and attempt.project_id=command.project_id and attempt.adapter_id=$3::bigint
		    and attempt.status='sent' and attempt.sent_at<$4)
		returning command.id,command.team_id
	)
	select id::text,team_id from acknowledged`, instance.ProjectID, poll.Device.DeviceID, fmt.Sprint(instance.ID), capturedAt)
	if err != nil {
		return err
	}
	type acknowledgedCommand struct {
		id     string
		teamID int
	}
	commands := make([]acknowledgedCommand, 0)
	for rows.Next() {
		var command acknowledgedCommand
		if err := rows.Scan(&command.id, &command.teamID); err != nil {
			rows.Close()
			return err
		}
		commands = append(commands, command)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, command := range commands {
		if _, err := tx.ExecContext(ctx, `update command_attempts set status='acknowledged',acknowledged_at=$3,
			result_json=result_json||'{"evidence":"device_state.mode_code:auto_returning_to_home","final":true}'::jsonb
			where project_id=$1 and command_id=$2::uuid and status='sent'`, instance.ProjectID, command.id, capturedAt); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"commandId": command.id, "outcome": "ack", "source": "device_state"})
		if _, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
			values($1,$2,$3,'command.ack',$4::jsonb) on conflict(event_id) do nothing`,
			instance.ProjectID, command.teamID, "command.ack:"+command.id+":flighthub-state", payload); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (reconciler *SQLControlCommandReconciler) ExpireUnknown(ctx context.Context) (int64, error) {
	now := reconciler.now().UTC()
	tx, err := reconciler.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `update device_commands command set status='unknown',completed_at=$1,
		result_json=command.result_json||'{"safeCode":"deadline_without_fresh_evidence","final":false}'::jsonb
		where command.status='sent' and command.deadline_at<=$1
		  and command.safety_context_json->>'connectorKey'='dji.flighthub2'
		returning command.id::text,command.project_id,command.team_id`, now)
	if err != nil {
		return 0, err
	}
	type expiredCommand struct {
		id                string
		projectID, teamID int
	}
	commands := make([]expiredCommand, 0)
	for rows.Next() {
		var command expiredCommand
		if err := rows.Scan(&command.id, &command.projectID, &command.teamID); err != nil {
			rows.Close()
			return 0, err
		}
		commands = append(commands, command)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, command := range commands {
		if _, err := tx.ExecContext(ctx, `update command_attempts set status='timed_out',error_code='deadline_without_fresh_evidence',
			result_json=result_json||'{"safeCode":"deadline_without_fresh_evidence","final":false}'::jsonb
			where project_id=$1 and command_id=$2::uuid and status='sent'`, command.projectID, command.id); err != nil {
			return 0, err
		}
		payload, _ := json.Marshal(map[string]any{"commandId": command.id, "outcome": "timeout"})
		if _, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
			values($1,$2,$3,'command.ack',$4::jsonb) on conflict(event_id) do nothing`, command.projectID, command.teamID,
			"command.ack:"+command.id+":flighthub-timeout", payload); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(commands)), nil
}

func (reconciler *SQLControlCommandReconciler) Run(ctx context.Context, interval time.Duration) error {
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
			if _, err := reconciler.ExpireUnknown(ctx); err != nil && ctx.Err() == nil {
				return err
			}
		}
	}
}

func RouteDeviceCommand(flightHub outbox.Handler, fallback outbox.Handler) outbox.Handler {
	return func(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
		var payload struct {
			CommandID string `json:"commandId"`
		}
		if json.Unmarshal(event.Payload, &payload) != nil || payload.CommandID == "" {
			return errors.New("DEVICE_COMMAND_EVENT_INVALID")
		}
		var deviceID int
		var recordedConnectorKey string
		err := tx.QueryRowContext(ctx, `select device_id,coalesce(safety_context_json->>'connectorKey','')
			from device_commands where project_id=$1 and id=$2::uuid`, event.ProjectID, payload.CommandID).Scan(&deviceID, &recordedConnectorKey)
		if err != nil {
			return err
		}
		if recordedConnectorKey == ConnectorKey {
			if flightHub == nil {
				return errors.New("DJI_FLIGHTHUB_COMMAND_HANDLER_UNAVAILABLE")
			}
			return flightHub(ctx, tx, event)
		}
		route, err := connector.ResolvePrimaryBinding(ctx, tx, event.ProjectID, deviceID)
		if err != nil {
			return err
		}
		var connectorKey string
		if err := tx.QueryRowContext(ctx, `select definition.connector_key from device_adapters adapter
			join connector_definitions definition on definition.id=adapter.connector_definition_id
			where adapter.project_id=$1 and adapter.id=$2`, event.ProjectID, route.ConnectorInstanceID).Scan(&connectorKey); err != nil {
			return err
		}
		if connectorKey == ConnectorKey {
			if flightHub == nil {
				return errors.New("DJI_FLIGHTHUB_COMMAND_HANDLER_UNAVAILABLE")
			}
			return flightHub(ctx, tx, event)
		}
		if fallback == nil {
			return errors.New("DEVICE_COMMAND_HANDLER_UNAVAILABLE")
		}
		return fallback(ctx, tx, event)
	}
}
