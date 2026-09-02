package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"aerosight/worker/internal/connector"
)

const (
	commandStatusBatchSerials     = 50
	commandStatusBatchIdentifiers = 100
)

type CommandStatusClient interface {
	ListOrganizationCommandStatus(context.Context, string, string, string, []string, []string) (CommandStatusOutput, error)
}

type PendingStatusCommand struct {
	ID, CommandKey, DeviceSN, RemoteBusinessID string
	ProjectID, TeamID, DeviceID                int
	AdapterID                                  int64
	SentAt, Deadline                           time.Time
	RemoteUpdatedAt                            int64
	Instance                                   connector.Instance
	ProjectUUID, OrganizationUUID              string
}

type CommandStatusDecision struct {
	CommandID, RemoteBusinessID  string
	RemoteUpdatedAt              int64
	ProgressPercent, CurrentStep int
	DeviceCode                   int
	Outcome                      string
}

type commandStatusObservation struct {
	DeviceSN, Method, BusinessID string
	CreatedAt, UpdatedAt         int64
	ProgressPercent, CurrentStep int
	DeviceCode                   int
}

func (client *Client) ListOrganizationCommandStatus(ctx context.Context, token, projectUUID, organizationUUID string, deviceSNs, identifiers []string) (CommandStatusOutput, error) {
	input, err := json.Marshal(organizationCommandStatusInput{
		OrganizationUUID: organizationUUID,
		DeviceSNs:        deviceSNs,
		Identifiers:      identifiers,
	})
	if err != nil {
		return CommandStatusOutput{}, err
	}
	request, err := BuildControlActionRequest("command.status.organization", input)
	if err != nil {
		return CommandStatusOutput{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, request.Spec)
	if err != nil {
		return CommandStatusOutput{}, err
	}
	decoded, err := DecodeControlActionOutput("command.status.organization", payload.Data)
	if err != nil {
		return CommandStatusOutput{}, err
	}
	output, ok := decoded.(CommandStatusOutput)
	if !ok {
		return CommandStatusOutput{}, schemaError()
	}
	return output, nil
}

func ReconcileCommandStatus(commands []PendingStatusCommand, snapshot CommandStatusOutput) []CommandStatusDecision {
	observations := flattenCommandStatus(snapshot)
	claimedBusinessIDs := make(map[string]bool)
	for _, command := range commands {
		if command.RemoteBusinessID != "" {
			claimedBusinessIDs[command.RemoteBusinessID] = true
		}
	}
	duplicateBusinessIDs := make(map[string]bool)
	seenBusinessIDs := make(map[string]int)
	for _, observation := range observations {
		seenBusinessIDs[observation.BusinessID]++
		if seenBusinessIDs[observation.BusinessID] > 1 {
			duplicateBusinessIDs[observation.BusinessID] = true
		}
	}
	candidates := make([][]int, len(commands))
	observationMatches := make([]int, len(observations))
	for commandIndex, command := range commands {
		for observationIndex, observation := range observations {
			if duplicateBusinessIDs[observation.BusinessID] || observation.DeviceSN != command.DeviceSN || observation.Method != command.CommandKey {
				continue
			}
			if !command.Deadline.IsZero() && remoteCommandTime(observation.UpdatedAt).After(command.Deadline) {
				continue
			}
			if command.RemoteBusinessID != "" {
				if observation.BusinessID != command.RemoteBusinessID || observation.UpdatedAt <= command.RemoteUpdatedAt {
					continue
				}
			} else {
				if claimedBusinessIDs[observation.BusinessID] || remoteCommandTime(observation.CreatedAt).Before(command.SentAt) {
					continue
				}
			}
			candidates[commandIndex] = append(candidates[commandIndex], observationIndex)
			observationMatches[observationIndex]++
		}
	}
	decisions := make([]CommandStatusDecision, 0, len(commands))
	for commandIndex, commandCandidates := range candidates {
		if len(commandCandidates) != 1 || observationMatches[commandCandidates[0]] != 1 {
			continue
		}
		observation := observations[commandCandidates[0]]
		outcome := "pending"
		if observation.ProgressPercent == 100 {
			if observation.DeviceCode == 0 {
				outcome = "acknowledged"
			} else {
				outcome = "nacked"
			}
		}
		decisions = append(decisions, CommandStatusDecision{
			CommandID: commands[commandIndex].ID, RemoteBusinessID: observation.BusinessID,
			RemoteUpdatedAt: observation.UpdatedAt, ProgressPercent: observation.ProgressPercent,
			CurrentStep: observation.CurrentStep, DeviceCode: observation.DeviceCode, Outcome: outcome,
		})
	}
	return decisions
}

func flattenCommandStatus(snapshot CommandStatusOutput) []commandStatusObservation {
	observations := make([]commandStatusObservation, 0)
	for _, device := range snapshot.List {
		for method, service := range device.Services {
			observations = append(observations, commandStatusObservation{
				DeviceSN: device.SN, Method: method, BusinessID: service.BusinessID,
				CreatedAt: service.CreateTime, UpdatedAt: service.UpdateTime,
				ProgressPercent: service.Progress.Percent, CurrentStep: service.Progress.CurrentStep,
				DeviceCode: service.DeviceCode,
			})
		}
	}
	sort.Slice(observations, func(left, right int) bool {
		if observations[left].DeviceSN != observations[right].DeviceSN {
			return observations[left].DeviceSN < observations[right].DeviceSN
		}
		if observations[left].Method != observations[right].Method {
			return observations[left].Method < observations[right].Method
		}
		return observations[left].BusinessID < observations[right].BusinessID
	})
	return observations
}

func remoteCommandTime(value int64) time.Time {
	if value < 10_000_000_000 {
		return time.Unix(value, 0).UTC()
	}
	return time.UnixMilli(value).UTC()
}

type ControlCommandStatusReconciler struct {
	database *sql.DB
	client   CommandStatusClient
	resolver TokenResolver
	now      func() time.Time
	load     func(context.Context) ([]PendingStatusCommand, error)
	apply    func(context.Context, []PendingStatusCommand, []CommandStatusDecision, time.Time) (int, error)
}

func NewControlCommandStatusReconciler(database *sql.DB, client CommandStatusClient, resolver TokenResolver, now func() time.Time) (*ControlCommandStatusReconciler, error) {
	if database == nil || client == nil || resolver == nil {
		return nil, errors.New("DJI_FLIGHTHUB_COMMAND_STATUS_DEPENDENCY_REQUIRED")
	}
	if now == nil {
		now = time.Now
	}
	reconciler := &ControlCommandStatusReconciler{database: database, client: client, resolver: resolver, now: now}
	reconciler.load = reconciler.loadPending
	reconciler.apply = reconciler.applyDecisions
	return reconciler, nil
}

func (reconciler *ControlCommandStatusReconciler) PollOnce(ctx context.Context) (int, error) {
	commands, err := reconciler.load(ctx)
	if err != nil {
		return 0, err
	}
	groups := groupPendingStatusCommands(commands)
	applied := 0
	for _, group := range groups {
		token, err := reconciler.resolver.ResolveToken(ctx, group[0].Instance)
		if err != nil {
			return applied, err
		}
		serials, identifiers := commandStatusQuery(group)
		snapshot, pollErr := reconciler.client.ListOrganizationCommandStatus(
			ctx, token, group[0].ProjectUUID, group[0].OrganizationUUID, serials, identifiers,
		)
		token = ""
		if pollErr != nil {
			return applied, pollErr
		}
		decisions := ReconcileCommandStatus(group, snapshot)
		count, err := reconciler.apply(ctx, group, decisions, reconciler.now().UTC())
		if err != nil {
			return applied, err
		}
		applied += count
	}
	return applied, nil
}

func (reconciler *ControlCommandStatusReconciler) loadPending(ctx context.Context) ([]PendingStatusCommand, error) {
	rows, err := reconciler.database.QueryContext(ctx, `select command.id::text,command.project_id,command.team_id,command.device_id,
		command.command_key,command.deadline_at,attempt.sent_at,adapter.id,adapter.credential_envelope_json,adapter.discovery_scope_json,
		identity.identity_json#>>'{attributes,serialNumber}',coalesce(attempt.result_json->>'remoteBusinessId',''),
		case when jsonb_typeof(attempt.result_json->'remoteUpdatedAt')='number' then (attempt.result_json->>'remoteUpdatedAt')::bigint else 0 end
	 from device_commands command
	 join command_attempts attempt on attempt.command_id=command.id and attempt.project_id=command.project_id and attempt.attempt=1 and attempt.status='sent'
	 join device_adapters adapter on adapter.id=attempt.adapter_id and adapter.project_id=command.project_id and adapter.status in('connected','degraded')
	 join connector_definitions definition on definition.id=adapter.connector_definition_id
	 join device_external_identities identity on identity.project_id=command.project_id and identity.adapter_id=adapter.id and identity.device_id=command.device_id
	 where command.status='sent' and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
	   and command.safety_context_json->>'connectorInstanceId'=adapter.id::text
	 order by adapter.id,command.created_at,command.id limit 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	commands := make([]PendingStatusCommand, 0)
	for rows.Next() {
		var command PendingStatusCommand
		var credential, scope []byte
		if err := rows.Scan(&command.ID, &command.ProjectID, &command.TeamID, &command.DeviceID, &command.CommandKey,
			&command.Deadline, &command.SentAt, &command.AdapterID, &credential, &scope, &command.DeviceSN,
			&command.RemoteBusinessID, &command.RemoteUpdatedAt); err != nil {
			return nil, err
		}
		parsedScope, err := parseScope(scope)
		if err != nil || parsedScope.OrganizationUUID == "" {
			continue
		}
		command.ProjectUUID, command.OrganizationUUID = parsedScope.ProjectUUID, parsedScope.OrganizationUUID
		command.Instance = connector.Instance{
			ID: command.AdapterID, ProjectID: command.ProjectID, ConnectorKey: ConnectorKey, Version: ConnectorVersion,
			CredentialEnvelope: credential, DiscoveryScope: scope,
		}
		commands = append(commands, command)
	}
	return commands, rows.Err()
}

func groupPendingStatusCommands(commands []PendingStatusCommand) [][]PendingStatusCommand {
	groups := make([][]PendingStatusCommand, 0)
	for len(commands) > 0 {
		adapterID := commands[0].AdapterID
		group := make([]PendingStatusCommand, 0)
		serials := make(map[string]struct{})
		identifiers := 0
		index := 0
		for index < len(commands) && commands[index].AdapterID == adapterID {
			command := commands[index]
			_, existingSN := serials[command.DeviceSN]
			additionalIdentifier := 0
			if command.RemoteBusinessID != "" {
				additionalIdentifier = 1
			}
			if (!existingSN && len(serials) == commandStatusBatchSerials) || identifiers+additionalIdentifier > commandStatusBatchIdentifiers {
				break
			}
			serials[command.DeviceSN] = struct{}{}
			identifiers += additionalIdentifier
			group = append(group, command)
			index++
		}
		groups = append(groups, group)
		commands = commands[index:]
	}
	return groups
}

func commandStatusQuery(commands []PendingStatusCommand) ([]string, []string) {
	serialSet, identifierSet := make(map[string]struct{}), make(map[string]struct{})
	for _, command := range commands {
		serialSet[command.DeviceSN] = struct{}{}
		if command.RemoteBusinessID != "" {
			identifierSet[command.RemoteBusinessID] = struct{}{}
		}
	}
	serials, identifiers := make([]string, 0, len(serialSet)), make([]string, 0, len(identifierSet))
	for serial := range serialSet {
		serials = append(serials, serial)
	}
	for identifier := range identifierSet {
		identifiers = append(identifiers, identifier)
	}
	sort.Strings(serials)
	sort.Strings(identifiers)
	return serials, identifiers
}

func (reconciler *ControlCommandStatusReconciler) applyDecisions(ctx context.Context, commands []PendingStatusCommand, decisions []CommandStatusDecision, observedAt time.Time) (int, error) {
	byID := make(map[string]PendingStatusCommand, len(commands))
	for _, command := range commands {
		byID[command.ID] = command
	}
	tx, err := reconciler.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	applied := 0
	for _, decision := range decisions {
		command, ok := byID[decision.CommandID]
		if !ok || (command.RemoteBusinessID != "" && command.RemoteBusinessID != decision.RemoteBusinessID) {
			continue
		}
		result := map[string]any{
			"remoteBusinessId": decision.RemoteBusinessID, "remoteUpdatedAt": decision.RemoteUpdatedAt,
			"progressPercent": decision.ProgressPercent, "currentStep": decision.CurrentStep, "deviceCode": decision.DeviceCode,
		}
		resultJSON, _ := json.Marshal(result)
		if decision.Outcome == "pending" {
			attemptResult, err := tx.ExecContext(ctx, `update command_attempts set result_json=result_json||$4::jsonb
			where project_id=$1 and command_id=$2::uuid and attempt=1 and status='sent'
			  and (($3='' and coalesce(result_json->>'remoteBusinessId','')='') or
			       ($3<>'' and result_json->>'remoteBusinessId'=$3))
			  and (jsonb_typeof(result_json->'remoteUpdatedAt') is null or
			       (jsonb_typeof(result_json->'remoteUpdatedAt')='number' and (result_json->>'remoteUpdatedAt')::bigint<$5))`,
				command.ProjectID, command.ID, command.RemoteBusinessID, resultJSON, decision.RemoteUpdatedAt)
			if err != nil {
				return 0, err
			}
			if count, _ := attemptResult.RowsAffected(); count == 1 {
				applied++
			}
			continue
		}
		commandStatus, attemptStatus, outcome, safeCode := "acknowledged", "acknowledged", "ack", ""
		if decision.Outcome == "nacked" {
			commandStatus, attemptStatus, outcome, safeCode = "nacked", "nacked", "nack", "remote_command_failed"
		}
		commandResult, err := tx.ExecContext(ctx, `update device_commands set status=$3,completed_at=$4,
			result_json=result_json||jsonb_build_object('resultSource','remote_command_status','final',true,'safeCode',nullif($5,''))
			where project_id=$1 and id=$2::uuid and status='sent'`, command.ProjectID, command.ID, commandStatus, observedAt, safeCode)
		if err != nil {
			return 0, err
		}
		if count, _ := commandResult.RowsAffected(); count != 1 {
			continue
		}
		attemptResult, err := tx.ExecContext(ctx, `update command_attempts set status=$3,acknowledged_at=$4,error_code=nullif($5,''),
			result_json=result_json||$6::jsonb
			where project_id=$1 and command_id=$2::uuid and attempt=1 and status='sent'
			  and (($7='' and coalesce(result_json->>'remoteBusinessId','')='') or
			       ($7<>'' and result_json->>'remoteBusinessId'=$7))
			  and (jsonb_typeof(result_json->'remoteUpdatedAt') is null or
			       (jsonb_typeof(result_json->'remoteUpdatedAt')='number' and (result_json->>'remoteUpdatedAt')::bigint<$8))`,
			command.ProjectID, command.ID, attemptStatus, observedAt, safeCode, resultJSON,
			command.RemoteBusinessID, decision.RemoteUpdatedAt)
		if err != nil {
			return 0, err
		}
		if count, _ := attemptResult.RowsAffected(); count != 1 {
			return 0, errors.New("DJI_FLIGHTHUB_COMMAND_STATUS_STATE_CHANGED")
		}
		applied++
		payload, _ := json.Marshal(map[string]any{"commandId": command.ID, "outcome": outcome, "source": "remote_command_status"})
		eventID := fmt.Sprintf("command.ack:%s:flighthub-status:%d", command.ID, decision.RemoteUpdatedAt)
		if _, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
			values($1,$2,$3,'command.ack',$4::jsonb) on conflict(event_id) do nothing`,
			command.ProjectID, command.TeamID, eventID, payload); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return applied, nil
}

func (reconciler *ControlCommandStatusReconciler) Run(ctx context.Context, interval time.Duration, onError func(error)) error {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := reconciler.PollOnce(ctx); err != nil && ctx.Err() == nil && onError != nil {
				onError(errors.New("DJI_FLIGHTHUB_COMMAND_STATUS_RECONCILIATION_FAILED"))
			}
		}
	}
}
