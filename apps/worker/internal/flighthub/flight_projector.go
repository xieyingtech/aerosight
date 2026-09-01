package flighthub

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"aerosight/worker/internal/adapter"
	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/telemetry"
)

const flightProjectorScript = "dji-flighthub-flight-v1"

type SQLFlightCatalogProjector struct {
	db           *sql.DB
	telemetry    TelemetryBatchIngestor
	now          func() time.Time
	unknownAfter time.Duration
	authSecret   string
}

func NewSQLFlightCatalogProjector(database *sql.DB, telemetryIngestor TelemetryBatchIngestor, now func() time.Time, unknownAfter time.Duration, authSecret string) *SQLFlightCatalogProjector {
	if now == nil {
		now = time.Now
	}
	if unknownAfter <= 0 {
		unknownAfter = 30 * time.Minute
	}
	return &SQLFlightCatalogProjector{db: database, telemetry: telemetryIngestor, now: now, unknownAfter: unknownAfter, authSecret: authSecret}
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

func flightArtifactMarkerID(projectID, runID int, kind string) string {
	return fmt.Sprintf("flighthub-task-run:%d:%d:%s-synced", projectID, runID, kind)
}

func (projector *SQLFlightCatalogProjector) ListArtifactTargets(ctx context.Context, instance connector.Instance, limit int) (targets []FlightArtifactTarget, returnedErr error) {
	if projector == nil || projector.db == nil || limit < 1 || limit > 100 {
		return nil, errors.New("FlightHub flight artifact target query is invalid")
	}
	tx, _, err := projector.beginWritable(ctx, instance)
	if err != nil {
		return nil, err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	rows, err := tx.QueryContext(ctx, `select resource.remote_id,run.id,coalesce(resource.remote_version,''),
		(run.status='succeeded' and track_marker.event_id is null) as need_track,
		(operation_marker.event_id is null) as need_operation,
		(media_marker.event_id is null or coalesce(media_marker.payload_json->>'taskRemoteVersion','')<>coalesce(resource.remote_version,'')) as need_media,
		(lower(coalesce(resource.summary_json->>'mediaUploadStatus','')) in('','uploaded','success','completed','upload_success','upload_complete')) as media_upload_final
		from connector_remote_resources resource
		join task_runs run on resource.project_id=run.project_id
		 and resource.canonical_target_type='task_run' and resource.canonical_target_id=run.id::text
		left join project_events track_marker on track_marker.project_id=run.project_id
		 and track_marker.event_id='flighthub-task-run:'||run.project_id::text||':'||run.id::text||':track-synced'
		left join project_events operation_marker on operation_marker.project_id=run.project_id
		 and operation_marker.event_id='flighthub-task-run:'||run.project_id::text||':'||run.id::text||':operations-synced'
		left join project_events media_marker on media_marker.project_id=run.project_id
		 and media_marker.event_id='flighthub-task-run:'||run.project_id::text||':'||run.id::text||':media-synced'
		where resource.project_id=$1 and resource.connector_instance_id=$2 and resource.resource_kind='flight-task'
		 and resource.status='active' and run.status in('succeeded','failed','canceled')
		 and ((run.status='succeeded' and track_marker.event_id is null) or operation_marker.event_id is null
		      or media_marker.event_id is null or coalesce(media_marker.payload_json->>'taskRemoteVersion','')<>coalesce(resource.remote_version,''))
		order by run.finished_at nulls last,resource.id limit $3`, instance.ProjectID, instance.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var target FlightArtifactTarget
		if err := rows.Scan(&target.RemoteTaskID, &target.TaskRunID, &target.RemoteVersion, &target.NeedTrack, &target.NeedOperation, &target.NeedMedia, &target.MediaUploadFinal); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return targets, nil
}

func validateFlightArtifactTarget(ctx context.Context, tx *sql.Tx, instance connector.Instance, target FlightArtifactTarget) error {
	if strings.TrimSpace(target.RemoteTaskID) == "" || target.TaskRunID <= 0 {
		return errors.New("FlightHub flight artifact target is invalid")
	}
	resource, err := loadProjectedRemoteResource(ctx, tx, instance, "flight-task", target.RemoteTaskID)
	if err != nil {
		return err
	}
	if !resource.CanonicalType.Valid || resource.CanonicalType.String != "task_run" || !resource.CanonicalID.Valid || resource.CanonicalID.String != strconv.Itoa(target.TaskRunID) {
		return errors.New("FlightHub flight artifact canonical link is invalid")
	}
	if target.RemoteVersion == "" || resource.RemoteVersion != target.RemoteVersion {
		return connector.ErrRemoteResourceUnavailable
	}
	var runID int
	err = tx.QueryRowContext(ctx, `select id from task_runs where project_id=$1 and id=$2 and status in('succeeded','failed','canceled') for update`,
		instance.ProjectID, target.TaskRunID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return connector.ErrRemoteResourceUnavailable
	}
	return err
}

func flightTrackTime(timestamp int64) (time.Time, bool) {
	if timestamp <= 0 {
		return time.Time{}, false
	}
	if timestamp >= 1_000_000_000_000 {
		return time.UnixMilli(timestamp).UTC(), true
	}
	return time.Unix(timestamp, 0).UTC(), true
}

func validFlightTrackPoint(point FlightTrackPoint) bool {
	return point.Timestamp > 0 && point.Longitude >= -180 && point.Longitude <= 180 && point.Latitude >= -90 && point.Latitude <= 90 &&
		!math.IsNaN(point.Longitude) && !math.IsInf(point.Longitude, 0) && !math.IsNaN(point.Latitude) && !math.IsInf(point.Latitude, 0) &&
		!math.IsNaN(point.Height) && !math.IsInf(point.Height, 0)
}

func (projector *SQLFlightCatalogProjector) ingestFlightTrack(ctx context.Context, instance connector.Instance, teamID, deviceID int, target FlightArtifactTarget, track FlightTaskTrack, receivedAt time.Time) (valid, invalid int, returnedErr error) {
	if projector.telemetry == nil {
		return 0, 0, errors.New("FlightHub flight track telemetry ingestor is unavailable")
	}
	batch := make([]telemetry.Telemetry, 0, len(track.Track.Points))
	for _, point := range track.Track.Points {
		capturedAt, timeOK := flightTrackTime(point.Timestamp)
		if !timeOK || !validFlightTrackPoint(point) {
			invalid++
			continue
		}
		altitude := point.Height
		payload, err := json.Marshal(adapter.Pose{
			DeviceType: "drone", CRS: "dji-flighthub-track:unverified", TransformVersion: "dji-flighthub-track-v1",
			Longitude: point.Longitude, Latitude: point.Latitude, AltitudeMeters: &altitude,
			Quality: map[string]any{"source": "dji-flighthub-openapi", "coordinateReference": "unverified", "taskRunId": target.TaskRunID},
		})
		if err != nil {
			return valid, invalid, err
		}
		quality, err := json.Marshal(map[string]any{
			"source": "dji-flighthub-openapi", "coordinateReference": "unverified", "transformVersion": "dji-flighthub-track-v1", "taskRunId": target.TaskRunID,
		})
		if err != nil {
			return valid, invalid, err
		}
		sequence := point.Timestamp
		runID := target.TaskRunID
		batch = append(batch, telemetry.Telemetry{
			ProjectID: instance.ProjectID, TeamID: teamID, AdapterID: instance.ID, DeviceID: deviceID, TaskRunID: &runID,
			EventID: fmt.Sprintf("flighthub-track:%d:%s", target.TaskRunID, secureRemoteKey(strconv.FormatInt(capturedAt.UnixNano(), 10))),
			Type:    "telemetry.pose", Sequence: &sequence, CapturedAt: capturedAt, ReceivedAt: receivedAt, Payload: payload, Quality: quality,
			RequireActiveAdapter: true, AdapterLeaseOwner: instance.LeaseOwner, AdapterLeaseEpoch: instance.LeaseEpoch,
		})
		valid++
	}
	if len(batch) == 0 {
		return valid, invalid, nil
	}
	inserted, err := projector.telemetry.IngestBatch(ctx, batch)
	return inserted, invalid, err
}

func insertFlightArtifactEvent(ctx context.Context, tx *sql.Tx, projectID, teamID int, eventID, eventType string, occurredAt time.Time, payload map[string]any) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json,occurred_at)
		values($1,$2,$3,$4,$5,$6) on conflict(event_id) do nothing`, projectID, teamID, eventID, eventType, payloadJSON, occurredAt)
	return err
}

func operationEventID(projectID, runID int, category string, timestamp int64, action, remoteKey string) string {
	fingerprint := strings.Join([]string{category, strconv.FormatInt(timestamp, 10), action, remoteKey}, ":")
	return fmt.Sprintf("flighthub-task-run:%d:%d:operation:%s", projectID, runID, secureRemoteKey(fingerprint))
}

func insertFlightOperationTimeline(ctx context.Context, tx *sql.Tx, projectID, teamID int, target FlightArtifactTarget, timeline FlightTaskOperationTimeline) error {
	insertChange := func(category string, item FlightControlChange) error {
		occurredAt, ok := flightTrackTime(item.Time)
		if !ok || !validFlightOperationCode(item.ControlType) {
			return errors.New("FlightHub control change is invalid")
		}
		return insertFlightArtifactEvent(ctx, tx, projectID, teamID,
			operationEventID(projectID, target.TaskRunID, category, item.Time, item.ControlType, item.UserID),
			"task_run.vendor_operation", occurredAt,
			map[string]any{"taskRunId": target.TaskRunID, "category": category, "action": item.ControlType, "source": "dji-flighthub-openapi"})
	}
	for _, item := range timeline.ControlChanges {
		if err := insertChange("control_change", item); err != nil {
			return err
		}
	}
	for _, item := range timeline.PayloadChanges {
		if err := insertChange("payload_change", item); err != nil {
			return err
		}
	}
	for _, item := range timeline.OperationLogs {
		occurredAt, ok := flightTrackTime(item.Time)
		if !ok || !validFlightOperationCode(item.Method) {
			return errors.New("FlightHub operation log is invalid")
		}
		if err := insertFlightArtifactEvent(ctx, tx, projectID, teamID,
			operationEventID(projectID, target.TaskRunID, "operation", item.Time, item.Method, item.Bid+":"+item.UserID),
			"task_run.vendor_operation", occurredAt,
			map[string]any{"taskRunId": target.TaskRunID, "category": "operation", "action": item.Method, "source": "dji-flighthub-openapi"}); err != nil {
			return err
		}
	}
	return nil
}

func (projector *SQLFlightCatalogProjector) ApplyFlightArtifacts(ctx context.Context, instance connector.Instance, poll FlightArtifactPoll) (returnedErr error) {
	if projector == nil || projector.db == nil || poll.ReceivedAt.IsZero() || (poll.Track == nil && poll.Operations == nil && poll.Media == nil) {
		return errors.New("FlightHub flight artifact projection is invalid")
	}
	tx, teamID, err := projector.beginWritable(ctx, instance)
	if err != nil {
		return err
	}
	if err := validateFlightArtifactTarget(ctx, tx, instance, poll.Target); err != nil {
		_ = tx.Rollback()
		return err
	}
	var deviceID *int
	if poll.Track != nil {
		deviceID, err = selectedTaskDevice(ctx, tx, instance, poll.Track.Track.DroneSN)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if deviceID == nil {
			_ = tx.Rollback()
			return errors.New("FlightHub flight track device is outside managed scope")
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	validPoints, invalidPoints := 0, 0
	if poll.Track != nil {
		validPoints, invalidPoints, err = projector.ingestFlightTrack(ctx, instance, teamID, *deviceID, poll.Target, *poll.Track, poll.ReceivedAt.UTC())
		if err != nil {
			return err
		}
	}

	tx, teamID, err = projector.beginWritable(ctx, instance)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	if err := validateFlightArtifactTarget(ctx, tx, instance, poll.Target); err != nil {
		return err
	}
	if poll.Track != nil {
		if err := tx.QueryRowContext(ctx, `select count(*) from observations
			where project_id=$1 and task_run_id=$2 and adapter_id=$3 and source_event_id like $4`,
			instance.ProjectID, poll.Target.TaskRunID, instance.ID, fmt.Sprintf("flighthub-track:%d:%%", poll.Target.TaskRunID)).Scan(&validPoints); err != nil {
			return err
		}
		if err := insertFlightArtifactEvent(ctx, tx, instance.ProjectID, teamID,
			flightArtifactMarkerID(instance.ProjectID, poll.Target.TaskRunID, "track"), "task_run.vendor_track_synced", poll.ReceivedAt.UTC(),
			map[string]any{
				"taskRunId": poll.Target.TaskRunID, "source": "dji-flighthub-openapi", "coordinateReference": "unverified",
				"validPointCount": validPoints, "invalidPointCount": invalidPoints,
				"flightDistance": poll.Track.Track.FlightDistance, "flightDuration": poll.Track.Track.FlightDuration,
			}); err != nil {
			return err
		}
	}
	if poll.Operations != nil {
		if err := insertFlightOperationTimeline(ctx, tx, instance.ProjectID, teamID, poll.Target, *poll.Operations); err != nil {
			return err
		}
		if err := insertFlightArtifactEvent(ctx, tx, instance.ProjectID, teamID,
			flightArtifactMarkerID(instance.ProjectID, poll.Target.TaskRunID, "operations"), "task_run.vendor_operations_synced", poll.ReceivedAt.UTC(),
			map[string]any{
				"taskRunId": poll.Target.TaskRunID, "source": "dji-flighthub-openapi",
				"controlChangeCount": len(poll.Operations.ControlChanges), "payloadChangeCount": len(poll.Operations.PayloadChanges),
				"operationCount": len(poll.Operations.OperationLogs), "relatedUserCount": len(poll.Operations.RelatedUsers),
			}); err != nil {
			return err
		}
	}
	if poll.Media != nil {
		mediaCount, err := projector.applyFlightMedia(ctx, tx, instance, teamID, poll.Target, *poll.Media, poll.ReceivedAt.UTC())
		if err != nil {
			return err
		}
		if poll.Target.MediaUploadFinal {
			payload := map[string]any{
				"taskRunId": poll.Target.TaskRunID, "source": "dji-flighthub-openapi",
				"mediaCount": mediaCount, "taskRemoteVersion": poll.Target.RemoteVersion,
			}
			payloadJSON, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json,occurred_at)
				values($1,$2,$3,'task_run.vendor_media_synced',$4,$5)
				on conflict(event_id) do update set payload_json=excluded.payload_json,occurred_at=excluded.occurred_at`,
				instance.ProjectID, teamID, flightArtifactMarkerID(instance.ProjectID, poll.Target.TaskRunID, "media"), payloadJSON, poll.ReceivedAt.UTC()); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func assetTimestamp(value string) (*time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, schemaError()
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func mediaAssetKind(fileType string) (string, string) {
	switch fileType {
	case "image":
		return "image", "image/*"
	case "video":
		return "video", "video/*"
	case "model_2d", "model_3d":
		return "model", "application/octet-stream"
	default:
		return "file", "application/octet-stream"
	}
}

func exportAssetMIME(fileTypes []string) string {
	for _, value := range fileTypes {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "pdf":
			return "application/pdf"
		case "csv":
			return "text/csv"
		case "excel", "xlsx":
			return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		}
	}
	return "application/octet-stream"
}

type externalAssetInput struct {
	ResourceKind    string
	RemoteID        string
	RemoteVersion   string
	RemoteUpdatedAt *time.Time
	TaskRunID       *int
	AssetKind       string
	MIMEType        string
	Status          string
	SizeBytes       *int64
	CapturedAt      *time.Time
	Summary         map[string]any
	Metadata        map[string]any
	Locator         map[string]string
}

func (projector *SQLFlightCatalogProjector) upsertExternalAsset(ctx context.Context, tx *sql.Tx, instance connector.Instance, teamID int, input externalAssetInput) (int, error) {
	if projector.authSecret == "" || strings.TrimSpace(input.RemoteID) == "" || strings.TrimSpace(input.RemoteVersion) == "" ||
		!validEnum(input.ResourceKind, "flight-media", "flight-record") || !validEnum(input.Status, "pending", "available", "failed") {
		return 0, errors.New("FlightHub external asset projection is invalid")
	}
	summaryJSON, err := json.Marshal(input.Summary)
	if err != nil {
		return 0, err
	}
	var remoteResourceID int64
	err = tx.QueryRowContext(ctx, `insert into connector_remote_resources(
		project_id,team_id,connector_instance_id,resource_kind,remote_id,remote_version,remote_updated_at,status,summary_json
	) values($1,$2,$3,$4,$5,$6,$7,'active',$8)
	 on conflict(project_id,connector_instance_id,resource_kind,remote_id) do update set
		team_id=excluded.team_id,remote_version=excluded.remote_version,remote_updated_at=excluded.remote_updated_at,
		status='active',summary_json=excluded.summary_json,last_seen_at=now(),missing_at=null,updated_at=now()
	 returning id`, instance.ProjectID, teamID, instance.ID, input.ResourceKind, input.RemoteID, input.RemoteVersion, input.RemoteUpdatedAt, summaryJSON).Scan(&remoteResourceID)
	if err != nil {
		return 0, err
	}
	logicalKey := fmt.Sprintf("dji-flighthub/%d/%s/%s", instance.ID, input.ResourceKind, secureRemoteKey(input.RemoteID))
	storageKey := fmt.Sprintf("projects/%d/remote/dji-flighthub/%s/%s", instance.ProjectID, input.ResourceKind, secureRemoteKey(input.RemoteID))
	metadataJSON, err := json.Marshal(input.Metadata)
	if err != nil {
		return 0, err
	}
	var assetID int
	err = tx.QueryRowContext(ctx, `insert into assets(
		project_id,team_id,task_run_id,kind,mime_type,storage_key,logical_key,version,status,object_version,size_bytes,captured_at,metadata_json,available_at,failed_at,failure_code
	) values($1,$2,$3,$4,$5,$6,$7,1,$8,$9,$10,$11,$12,
		case when $8='available' then now() else null end,case when $8='failed' then now() else null end,
		case when $8='failed' then 'DJI_FLIGHTHUB_EXPORT_FAILED' else null end)
	 on conflict(project_id,logical_key,version) do update set
		team_id=excluded.team_id,task_run_id=coalesce(excluded.task_run_id,assets.task_run_id),kind=excluded.kind,
		mime_type=excluded.mime_type,storage_key=excluded.storage_key,status=excluded.status,object_version=excluded.object_version,
		size_bytes=excluded.size_bytes,captured_at=excluded.captured_at,metadata_json=excluded.metadata_json,
		available_at=case when excluded.status='available' then coalesce(assets.available_at,now()) else null end,
		failed_at=case when excluded.status='failed' then coalesce(assets.failed_at,now()) else null end,
		failure_code=excluded.failure_code,deleted_at=null
	 returning id`, instance.ProjectID, teamID, input.TaskRunID, input.AssetKind, input.MIMEType, storageKey, logicalKey,
		input.Status, input.RemoteVersion, input.SizeBytes, input.CapturedAt, metadataJSON).Scan(&assetID)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `update connector_remote_resources set canonical_target_type='asset',canonical_target_id=$4,updated_at=now()
		where project_id=$1 and connector_instance_id=$2 and id=$3`, instance.ProjectID, instance.ID, remoteResourceID, strconv.Itoa(assetID)); err != nil {
		return 0, err
	}
	if input.Locator == nil {
		return assetID, nil
	}
	locatorJSON, err := json.Marshal(input.Locator)
	if err != nil {
		return 0, err
	}
	digest := sha256.Sum256(locatorJSON)
	envelope, err := credentials.EncryptJSON(input.Locator, projector.authSecret,
		credentials.AAD("flighthub-asset-reference", assetID, instance.ProjectID))
	if err != nil {
		return 0, err
	}
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `insert into connector_asset_access_refs(
		id,project_id,team_id,connector_instance_id,remote_resource_id,access_kind,reference_digest,credential_envelope_json
	) values($1,$2,$3,$4,$5,$6,$7,$8)
	 on conflict(id) do update set team_id=excluded.team_id,connector_instance_id=excluded.connector_instance_id,
		remote_resource_id=excluded.remote_resource_id,access_kind=excluded.access_kind,
		reference_digest=excluded.reference_digest,credential_envelope_json=excluded.credential_envelope_json,updated_at=now()
	 where connector_asset_access_refs.project_id=excluded.project_id`, assetID, instance.ProjectID, teamID, instance.ID,
		remoteResourceID, input.ResourceKind, hex.EncodeToString(digest[:]), envelopeJSON)
	return assetID, err
}

func (projector *SQLFlightCatalogProjector) applyFlightMedia(ctx context.Context, tx *sql.Tx, instance connector.Instance, teamID int, target FlightArtifactTarget, items []FlightTaskMedia, receivedAt time.Time) (int, error) {
	seen := make(map[string]FlightTaskMedia, len(items))
	for _, item := range items {
		if !validFlightMedia(&item) {
			return 0, schemaError()
		}
		if previous, duplicate := seen[item.UUID]; duplicate {
			if previous.Name != item.Name || previous.FileType != item.FileType || previous.Suffix != item.Suffix || previous.SizeBytes != item.SizeBytes || previous.UpdatedAt != item.UpdatedAt {
				return 0, schemaError()
			}
			continue
		}
		seen[item.UUID] = item
		capturedAt, err := assetTimestamp(item.CreatedAt)
		if err != nil {
			return 0, err
		}
		updatedAt, err := assetTimestamp(item.UpdatedAt)
		if err != nil {
			return 0, err
		}
		kind, mimeType := mediaAssetKind(item.FileType)
		remoteVersion := secureRemoteKey(strings.Join([]string{item.UpdatedAt, strconv.FormatInt(item.SizeBytes, 10), item.FileType, item.Suffix}, ":"))
		size := item.SizeBytes
		if _, err := projector.upsertExternalAsset(ctx, tx, instance, teamID, externalAssetInput{
			ResourceKind: "flight-media", RemoteID: item.UUID, RemoteVersion: remoteVersion, RemoteUpdatedAt: updatedAt,
			TaskRunID: &target.TaskRunID, AssetKind: kind, MIMEType: mimeType, Status: "available", SizeBytes: &size, CapturedAt: capturedAt,
			Summary: map[string]any{"name": item.Name, "fileType": item.FileType, "suffix": item.Suffix, "sizeBytes": item.SizeBytes, "capturedAt": item.CreatedAt},
			Metadata: map[string]any{
				"source": "dji-flighthub-openapi", "sourceKind": "flight-media", "remoteReference": true,
				"temporaryAccess": true, "fileType": item.FileType, "suffix": item.Suffix, "name": item.Name,
			},
			Locator: map[string]string{"taskUUID": target.RemoteTaskID, "mediaUUID": item.UUID},
		}); err != nil {
			return 0, err
		}
	}
	return len(seen), nil
}

func (projector *SQLFlightCatalogProjector) ApplyFlightExports(ctx context.Context, instance connector.Instance, poll FlightExportPoll) (returnedErr error) {
	if projector == nil || projector.db == nil || poll.ReceivedAt.IsZero() || poll.Records == nil {
		return errors.New("FlightHub flight export projection is invalid")
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
	seen := make(map[string]FlightExportRecord, len(poll.Records))
	for _, item := range poll.Records {
		if !validFlightExport(&item) {
			return schemaError()
		}
		if previous, duplicate := seen[item.UUID]; duplicate {
			if previous.Status != item.Status || previous.Progress != item.Progress || previous.ObjectKey != item.ObjectKey {
				return schemaError()
			}
			continue
		}
		seen[item.UUID] = item
		createdAt, err := assetTimestamp(item.CreatedAt)
		if err != nil {
			return err
		}
		updatedAt := createdAt
		if item.ExportTime != nil {
			updatedAt, err = assetTimestamp(*item.ExportTime)
			if err != nil {
				return err
			}
		}
		status := "pending"
		if item.Status == "export_complete" {
			status = "available"
		} else if item.Status == "export_failed" {
			status = "failed"
		}
		remoteVersion := secureRemoteKey(strings.Join([]string{item.Status, strconv.Itoa(item.Progress), updatedAt.Format(time.RFC3339Nano), strings.Join(item.FileTypes, ",")}, ":"))
		var locator map[string]string
		if item.ObjectKey != "" {
			locator = map[string]string{"objectKey": item.ObjectKey}
		}
		if _, err := projector.upsertExternalAsset(ctx, tx, instance, teamID, externalAssetInput{
			ResourceKind: "flight-record", RemoteID: item.UUID, RemoteVersion: remoteVersion, RemoteUpdatedAt: updatedAt,
			AssetKind: "file", MIMEType: exportAssetMIME(item.FileTypes), Status: status, CapturedAt: createdAt,
			Summary: map[string]any{
				"name": item.FileName, "contentType": item.ContentType, "status": item.Status,
				"progress": item.Progress, "fileTypes": item.FileTypes, "failedReasonCode": item.FailedReasonCode,
			},
			Metadata: map[string]any{
				"source": "dji-flighthub-openapi", "sourceKind": "flight-record", "remoteReference": true,
				"temporaryAccess": item.ObjectKey != "", "contentType": item.ContentType, "fileTypes": item.FileTypes, "name": item.FileName,
			},
			Locator: locator,
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
