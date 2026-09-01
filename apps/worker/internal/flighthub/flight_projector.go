package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
)

const flightProjectorScript = "dji-flighthub-flight-v1"

type SQLFlightCatalogProjector struct {
	db           *sql.DB
	now          func() time.Time
	unknownAfter time.Duration
}

func NewSQLFlightCatalogProjector(database *sql.DB, now func() time.Time, unknownAfter time.Duration) *SQLFlightCatalogProjector {
	if now == nil {
		now = time.Now
	}
	if unknownAfter <= 0 {
		unknownAfter = 30 * time.Minute
	}
	return &SQLFlightCatalogProjector{db: database, now: now, unknownAfter: unknownAfter}
}

func (projector *SQLFlightCatalogProjector) ApplyWaylines(ctx context.Context, instance connector.Instance, items []WaylineSummary) (returnedErr error) {
	if projector == nil || projector.db == nil {
		return errors.New("FlightHub flight catalog projector is unavailable")
	}
	tx, teamID, err := projector.beginWritable(ctx, instance)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	for _, item := range items {
		if !convertibleWayline(item) {
			continue
		}
		if err := projector.ensureWaylineTask(ctx, tx, instance, teamID, item); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (projector *SQLFlightCatalogProjector) ApplyFlightTasks(ctx context.Context, instance connector.Instance, items []FlightTaskSummary) (returnedErr error) {
	if projector == nil || projector.db == nil {
		return errors.New("FlightHub flight catalog projector is unavailable")
	}
	tx, teamID, err := projector.beginWritable(ctx, instance)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	now := projector.now().UTC()
	for _, item := range items {
		if err := projector.reconcileFlightTask(ctx, tx, instance, teamID, item, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (projector *SQLFlightCatalogProjector) beginWritable(ctx context.Context, instance connector.Instance) (*sql.Tx, int, error) {
	if instance.ID <= 0 || instance.ProjectID <= 0 {
		return nil, 0, errors.New("FlightHub flight catalog scope is invalid")
	}
	tx, err := projector.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	var teamID int
	err = tx.QueryRowContext(ctx, `select team_id from device_adapters
		where id=$1 and project_id=$2 and status in('connecting','connected','degraded')
		  and ($3='' or (lease_owner=$3 and connection_epoch=$4 and lease_expires_at>=now())) for update`,
		instance.ID, instance.ProjectID, instance.LeaseOwner, instance.LeaseEpoch).Scan(&teamID)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		return nil, 0, connector.ErrConnectorDisabled
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, 0, err
	}
	return tx, teamID, nil
}

func convertibleWayline(item WaylineSummary) bool {
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.DeviceModelKey) == "" || len(item.TemplateTypes) == 0 {
		return false
	}
	allowed := map[string]struct{}{
		"waypoint": {}, "mapping_2d": {}, "mapping_3d": {}, "mapping_strip": {},
		"facade": {}, "solid": {}, "mapping_gobject": {},
	}
	for _, value := range item.TemplateTypes {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(value))]; !ok {
			return false
		}
	}
	return true
}

type projectedRemoteResource struct {
	ID            int64
	RemoteVersion string
	CanonicalType sql.NullString
	CanonicalID   sql.NullString
}

func loadProjectedRemoteResource(ctx context.Context, tx *sql.Tx, instance connector.Instance, kind, remoteID string) (projectedRemoteResource, error) {
	var resource projectedRemoteResource
	err := tx.QueryRowContext(ctx, `select id,coalesce(remote_version,''),canonical_target_type,canonical_target_id
		from connector_remote_resources where project_id=$1 and connector_instance_id=$2 and resource_kind=$3 and remote_id=$4 for update`,
		instance.ProjectID, instance.ID, kind, remoteID).Scan(&resource.ID, &resource.RemoteVersion, &resource.CanonicalType, &resource.CanonicalID)
	if errors.Is(err, sql.ErrNoRows) {
		return resource, connector.ErrRemoteResourceUnavailable
	}
	return resource, err
}

func projectedFlightTaskName(name string, remoteResourceID int64) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "DJI FlightHub task"
	}
	return fmt.Sprintf("%s · FlightHub %d", name, remoteResourceID)
}

func waylineDefinition(item WaylineSummary, remoteResourceID int64, remoteVersion, taskName string) map[string]any {
	return map[string]any{
		"name": taskName, "description": "DJI FlightHub wayline read projection", "steps": []any{},
		"source": map[string]any{
			"connector": ConnectorKey, "resourceKind": "wayline", "remoteResourceId": remoteResourceID, "remoteVersion": remoteVersion,
		},
		"wayline": map[string]any{
			"deviceModelKey": item.DeviceModelKey, "templateTypes": item.TemplateTypes,
			"payloadCount": len(item.PayloadInformation), "sizeBytes": item.SizeBytes, "updatedAt": item.UpdatedAt,
		},
	}
}

func insertProjectedTask(ctx context.Context, tx *sql.Tx, projectID, teamID int, name string, target map[string]any) (int, error) {
	targetJSON, err := json.Marshal(target)
	if err != nil {
		return 0, err
	}
	var taskID int
	err = tx.QueryRowContext(ctx, `insert into tasks(
		project_id,team_id,name,description,trigger_type,status,required_capability_code,target_selector_json,event_rule_json,script
	) values($1,$2,$3,'DJI FlightHub read-only projection','manual','active','flight.read',$4,'{}'::jsonb,$5) returning id`,
		projectID, teamID, name, targetJSON, flightProjectorScript).Scan(&taskID)
	return taskID, err
}

func insertProjectedTaskVersion(ctx context.Context, tx *sql.Tx, projectID, teamID, taskID int, definition map[string]any) (int64, error) {
	definitionJSON, err := json.Marshal(definition)
	if err != nil {
		return 0, err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `select coalesce(max(version),0)::int+1 from task_versions where project_id=$1 and task_id=$2`, projectID, taskID).Scan(&version); err != nil {
		return 0, err
	}
	var versionID int64
	err = tx.QueryRowContext(ctx, `insert into task_versions(
		project_id,team_id,task_id,version,status,definition_json,script,input_schema_json,trigger_json,concurrency_limit,published_at
	) values($1,$2,$3,$4,'published',$5,$6,'{"type":"object","properties":{},"additionalProperties":false}',
		'{"type":"manual"}',1,now()) returning id`, projectID, teamID, taskID, version, definitionJSON, flightProjectorScript).Scan(&versionID)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `update tasks set current_published_version_id=$3,name=$4,updated_at=now() where project_id=$1 and id=$2`,
		projectID, taskID, versionID, strings.TrimSpace(fmt.Sprint(definition["name"])))
	return versionID, err
}

func (projector *SQLFlightCatalogProjector) ensureWaylineTask(ctx context.Context, tx *sql.Tx, instance connector.Instance, teamID int, item WaylineSummary) error {
	resource, err := loadProjectedRemoteResource(ctx, tx, instance, "wayline", item.ID)
	if err != nil {
		return err
	}
	taskName := projectedFlightTaskName(item.Name, resource.ID)
	var taskID int
	if resource.CanonicalType.Valid || resource.CanonicalID.Valid {
		if !resource.CanonicalType.Valid || resource.CanonicalType.String != "task" || !resource.CanonicalID.Valid {
			return errors.New("FlightHub wayline canonical link is invalid")
		}
		taskID, err = strconv.Atoi(resource.CanonicalID.String)
		if err != nil || taskID <= 0 {
			return errors.New("FlightHub wayline canonical task is invalid")
		}
	} else {
		taskID, err = insertProjectedTask(ctx, tx, instance.ProjectID, teamID, taskName, map[string]any{
			"source": ConnectorKey, "deviceModelKey": item.DeviceModelKey, "templateTypes": item.TemplateTypes,
		})
		if err != nil {
			return err
		}
	}

	var currentVersion sql.NullString
	var currentResourceID sql.NullInt64
	err = tx.QueryRowContext(ctx, `select version.definition_json#>>'{source,remoteVersion}',
		(version.definition_json#>>'{source,remoteResourceId}')::bigint
		from tasks task left join task_versions version on version.id=task.current_published_version_id and version.project_id=task.project_id
		where task.project_id=$1 and task.id=$2 for update of task`, instance.ProjectID, taskID).Scan(&currentVersion, &currentResourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return connector.ErrRemoteResourceUnavailable
	}
	if err != nil {
		return err
	}
	if currentResourceID.Valid && currentResourceID.Int64 != resource.ID {
		return errors.New("FlightHub wayline task source is incompatible")
	}
	if !currentResourceID.Valid || currentVersion.String != resource.RemoteVersion {
		if _, err := insertProjectedTaskVersion(ctx, tx, instance.ProjectID, teamID, taskID, waylineDefinition(item, resource.ID, resource.RemoteVersion, taskName)); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `update tasks set name=$3,updated_at=now() where project_id=$1 and id=$2`, instance.ProjectID, taskID, taskName); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `update connector_remote_resources set canonical_target_type='task',canonical_target_id=$4,updated_at=now()
		where project_id=$1 and connector_instance_id=$2 and id=$3`, instance.ProjectID, instance.ID, resource.ID, strconv.Itoa(taskID))
	return err
}

type taskRunRecord struct {
	ID            int
	TaskID        int
	TaskVersionID int64
	Status        string
	Reason        sql.NullString
	StateVersion  int
	CreatedAt     time.Time
}

func flightTaskTriggerKey(instance connector.Instance, remoteID string) string {
	return fmt.Sprintf("flighthub:%d:%s", instance.ID, secureRemoteKey(remoteID))
}

func loadTaskRun(ctx context.Context, tx *sql.Tx, projectID, runID int) (taskRunRecord, error) {
	var run taskRunRecord
	err := tx.QueryRowContext(ctx, `select id,task_id,task_version_id,status,state_reason,state_version,created_at
		from task_runs where project_id=$1 and id=$2 for update`, projectID, runID).Scan(
		&run.ID, &run.TaskID, &run.TaskVersionID, &run.Status, &run.Reason, &run.StateVersion, &run.CreatedAt,
	)
	return run, err
}

func findTaskRunByTrigger(ctx context.Context, tx *sql.Tx, projectID int, triggerKey string) (taskRunRecord, bool, error) {
	var run taskRunRecord
	err := tx.QueryRowContext(ctx, `select id,task_id,task_version_id,status,state_reason,state_version,created_at
		from task_runs where project_id=$1 and trigger_key=$2 order by id limit 1 for update`, projectID, triggerKey).Scan(
		&run.ID, &run.TaskID, &run.TaskVersionID, &run.Status, &run.Reason, &run.StateVersion, &run.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return taskRunRecord{}, false, nil
	}
	return run, err == nil, err
}

func findWaylineTask(ctx context.Context, tx *sql.Tx, instance connector.Instance, remoteWaylineID string) (int, int64, bool, error) {
	var taskID int
	var versionID int64
	err := tx.QueryRowContext(ctx, `select task.id,task.current_published_version_id
		from connector_remote_resources resource join tasks task
		  on resource.canonical_target_type='task' and resource.canonical_target_id=task.id::text and task.project_id=resource.project_id
		where resource.project_id=$1 and resource.connector_instance_id=$2 and resource.resource_kind='wayline' and resource.remote_id=$3`,
		instance.ProjectID, instance.ID, remoteWaylineID).Scan(&taskID, &versionID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	return taskID, versionID, err == nil, err
}

func ensureFallbackFlightTask(ctx context.Context, tx *sql.Tx, projectID, teamID int, item FlightTaskSummary, resource projectedRemoteResource) (int, int64, error) {
	taskName := projectedFlightTaskName(item.Name, resource.ID)
	taskID, err := insertProjectedTask(ctx, tx, projectID, teamID, taskName, map[string]any{"source": ConnectorKey, "resourceKind": "flight-task"})
	if err != nil {
		return 0, 0, err
	}
	definition := map[string]any{
		"name": taskName, "description": "DJI FlightHub flight task read projection", "steps": []any{},
		"source": map[string]any{
			"connector": ConnectorKey, "resourceKind": "flight-task", "remoteResourceId": resource.ID, "remoteVersion": resource.RemoteVersion,
		},
	}
	versionID, err := insertProjectedTaskVersion(ctx, tx, projectID, teamID, taskID, definition)
	return taskID, versionID, err
}

func selectedTaskDevice(ctx context.Context, tx *sql.Tx, instance connector.Instance, serial string) (*int, error) {
	var deviceID int
	err := tx.QueryRowContext(ctx, `select device_id from device_external_identities
		where project_id=$1 and adapter_id=$2 and device_id is not null and identity_json#>>'{attributes,serialNumber}'=$3
		order by id limit 1`, instance.ProjectID, instance.ID, serial).Scan(&deviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &deviceID, nil
}

type desiredFlightRunState struct {
	Status     string
	Reason     string
	Error      *string
	StartedAt  *time.Time
	FinishedAt *time.Time
	Known      bool
}

func parseFlightTime(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func desiredTaskRunState(item FlightTaskSummary, now time.Time, unknownAfter time.Duration, firstSeen time.Time) desiredFlightRunState {
	state := desiredFlightRunState{Status: "blocked", Reason: "flighthub_status_unverified"}
	switch item.Status {
	case "waiting", "preparing", "queue_for_takeoff":
		state.Status, state.Reason, state.Known = "dispatching", "flighthub_accepted", true
	case "executing":
		state.Status, state.Reason, state.Known = "running", "flighthub_executing", true
	case "paused", "suspended":
		state.Status, state.Reason, state.Known = "paused", "flighthub_paused", true
	case "success":
		state.Status, state.Reason, state.Known = "succeeded", "flighthub_succeeded", true
	case "starting_failure", "partially_done":
		message := "DJI_FLIGHTHUB_TASK_FAILED"
		state.Status, state.Reason, state.Error, state.Known = "failed", "flighthub_failed", &message, true
	case "timeout":
		message := "DJI_FLIGHTHUB_TASK_TIMEOUT"
		state.Status, state.Reason, state.Error, state.Known = "failed", "flighthub_timeout", &message, true
	case "terminated":
		state.Status, state.Reason, state.Known = "canceled", "flighthub_canceled", true
	}
	state.StartedAt = parseFlightTime(item.RunAt)
	if state.StartedAt == nil && (state.Status == "running" || state.Status == "paused" || state.Status == "succeeded" || state.Status == "failed" || state.Status == "canceled") {
		state.StartedAt = parseFlightTime(item.BeginAt)
	}
	if state.Status == "succeeded" || state.Status == "failed" || state.Status == "canceled" {
		state.FinishedAt = parseFlightTime(item.CompletedAt)
		if state.FinishedAt == nil {
			state.FinishedAt = parseFlightTime(item.EndAt)
		}
	}
	if state.Status == "dispatching" || state.Status == "running" || state.Status == "paused" || !state.Known {
		deadline := parseFlightTime(item.EndAt)
		if deadline == nil && !state.Known {
			value := firstSeen.UTC()
			deadline = &value
		}
		if deadline != nil && now.After(deadline.Add(unknownAfter)) {
			state.Status, state.Reason = "blocked", "flighthub_result_unknown_timeout"
			state.Error = nil
		}
	}
	return state
}

func transitionAllowed(current, currentReason string, desired desiredFlightRunState) bool {
	if current == "succeeded" || current == "failed" || current == "canceled" {
		return current == desired.Status
	}
	if desired.Status == "blocked" {
		return true
	}
	if current == "blocked" && currentReason == "flighthub_result_unknown_timeout" {
		return desired.Status == "succeeded" || desired.Status == "failed" || desired.Status == "canceled"
	}
	if (current == "running" || current == "paused") && desired.Status == "dispatching" {
		return false
	}
	return true
}

func flightRunSnapshot(item FlightTaskSummary, resource projectedRemoteResource, now time.Time) map[string]any {
	return map[string]any{
		"source": "dji-flighthub-openapi", "remoteResourceId": resource.ID, "remoteVersion": resource.RemoteVersion,
		"remoteStatus": item.Status, "observedAt": now.Format(time.RFC3339Nano),
		"timeline": map[string]any{"beginAt": item.BeginAt, "endAt": item.EndAt, "runAt": item.RunAt, "completedAt": item.CompletedAt},
	}
}

func recordFlightRunTransition(ctx context.Context, tx *sql.Tx, projectID, teamID int, runID, stateVersion int, from string, desired desiredFlightRunState) error {
	payload := map[string]any{
		"taskRunId": runID, "from": from, "to": desired.Status, "stateVersion": stateVersion,
		"reason": desired.Reason, "source": "dji-flighthub-openapi",
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	eventID := fmt.Sprintf("flighthub-task-run:%d:%d:state:%d", projectID, runID, stateVersion)
	_, err = tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, projectID, teamID, eventID, payloadJSON)
	return err
}

func (projector *SQLFlightCatalogProjector) reconcileFlightTask(ctx context.Context, tx *sql.Tx, instance connector.Instance, teamID int, item FlightTaskSummary, now time.Time) error {
	resource, err := loadProjectedRemoteResource(ctx, tx, instance, "flight-task", item.UUID)
	if err != nil {
		return err
	}
	triggerKey := flightTaskTriggerKey(instance, item.UUID)
	var run taskRunRecord
	var exists bool
	if resource.CanonicalType.Valid || resource.CanonicalID.Valid {
		if !resource.CanonicalType.Valid || resource.CanonicalType.String != "task_run" || !resource.CanonicalID.Valid {
			return errors.New("FlightHub flight task canonical link is invalid")
		}
		runID, parseErr := strconv.Atoi(resource.CanonicalID.String)
		if parseErr != nil || runID <= 0 {
			return errors.New("FlightHub canonical task run is invalid")
		}
		run, err = loadTaskRun(ctx, tx, instance.ProjectID, runID)
		exists = err == nil
	} else {
		run, exists, err = findTaskRunByTrigger(ctx, tx, instance.ProjectID, triggerKey)
	}
	if err != nil {
		return err
	}

	if !exists {
		taskID, versionID, found, findErr := findWaylineTask(ctx, tx, instance, item.WaylineUUID)
		if findErr != nil {
			return findErr
		}
		if !found {
			taskID, versionID, err = ensureFallbackFlightTask(ctx, tx, instance.ProjectID, teamID, item, resource)
			if err != nil {
				return err
			}
		}
		deviceID, deviceErr := selectedTaskDevice(ctx, tx, instance, item.SN)
		if deviceErr != nil {
			return deviceErr
		}
		desired := desiredTaskRunState(item, now, projector.unknownAfter, now)
		snapshotJSON, marshalErr := json.Marshal(flightRunSnapshot(item, resource, now))
		if marshalErr != nil {
			return marshalErr
		}
		err = tx.QueryRowContext(ctx, `insert into task_runs(
			project_id,team_id,task_id,task_version_id,selected_device_id,trigger_source,trigger_key,status,state_version,
			input_snapshot_json,state_reason,error_message,started_at,finished_at
		) values($1,$2,$3,$4,$5,'dji-flighthub',$6,$7,0,$8,$9,$10,$11,$12) returning id,task_id,task_version_id,status,state_reason,state_version,created_at`,
			instance.ProjectID, teamID, taskID, versionID, deviceID, triggerKey, desired.Status, snapshotJSON, desired.Reason, desired.Error, desired.StartedAt, desired.FinishedAt).Scan(
			&run.ID, &run.TaskID, &run.TaskVersionID, &run.Status, &run.Reason, &run.StateVersion, &run.CreatedAt,
		)
		if err != nil {
			return err
		}
		if err := recordFlightRunTransition(ctx, tx, instance.ProjectID, teamID, run.ID, 0, "", desired); err != nil {
			return err
		}
	} else {
		desired := desiredTaskRunState(item, now, projector.unknownAfter, run.CreatedAt)
		if transitionAllowed(run.Status, run.Reason.String, desired) {
			changed := run.Status != desired.Status || run.Reason.String != desired.Reason
			nextVersion := run.StateVersion
			if changed {
				nextVersion++
			}
			snapshotJSON, marshalErr := json.Marshal(flightRunSnapshot(item, resource, now))
			if marshalErr != nil {
				return marshalErr
			}
			_, err = tx.ExecContext(ctx, `update task_runs set status=$3,state_version=$4,state_reason=$5,error_message=$6,
				input_snapshot_json=$7,started_at=coalesce(started_at,$8),finished_at=case when $3 in('succeeded','failed','canceled') then coalesce(finished_at,$9) else finished_at end
				where project_id=$1 and id=$2`, instance.ProjectID, run.ID, desired.Status, nextVersion, desired.Reason, desired.Error,
				snapshotJSON, desired.StartedAt, desired.FinishedAt)
			if err != nil {
				return err
			}
			if changed {
				if err := recordFlightRunTransition(ctx, tx, instance.ProjectID, teamID, run.ID, nextVersion, run.Status, desired); err != nil {
					return err
				}
				run.StateVersion = nextVersion
			}
			run.Status, run.Reason = desired.Status, sql.NullString{String: desired.Reason, Valid: true}
		}
	}
	_, err = tx.ExecContext(ctx, `update connector_remote_resources set canonical_target_type='task_run',canonical_target_id=$4,updated_at=now()
		where project_id=$1 and connector_instance_id=$2 and id=$3`, instance.ProjectID, instance.ID, resource.ID, strconv.Itoa(run.ID))
	return err
}
