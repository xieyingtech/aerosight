package algorithm

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aerosight/worker/internal/orchestration"
	"aerosight/worker/internal/outbox"
)

type AssetAccessIssuer interface {
	IssueAssetURL(projectID, assetID, version int, expiresAt time.Time) (string, error)
}

type Trigger struct {
	issuer AssetAccessIssuer
	now    func() time.Time
}

func NewTrigger(issuer AssetAccessIssuer) *Trigger {
	return &Trigger{issuer: issuer, now: time.Now}
}

type triggerAsset struct {
	ID, ProjectID, TeamID, Version int
	TaskRunID, TaskRunStepID       int64
	DeviceID                       sql.NullInt64
	Kind, MIMEType, Checksum       string
	CapturedAt                     time.Time
}

type triggerDefinition struct {
	VersionID      int64
	ProviderType   string
	Model          string
	ExecutionMode  string
	MappingVersion string
}

type taskAlgorithmPayload struct {
	TaskRunID     int   `json:"taskRunId"`
	TaskRunStepID int64 `json:"taskRunStepId"`
}

func (trigger *Trigger) TaskStepHandler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	if trigger.issuer == nil {
		return errors.New("algorithm task step requires asset access issuer")
	}
	var payload taskAlgorithmPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.TaskRunID <= 0 || payload.TaskRunStepID <= 0 {
		return errors.New("task algorithm payload requires taskRunId and taskRunStepId")
	}
	var rawParameters, runInputs []byte
	var stepStatus string
	err := tx.QueryRowContext(ctx, `select run_step.status,step.parameters_json,run.input_snapshot_json
		from task_run_steps run_step
		join task_runs run on run.id=run_step.task_run_id and run.project_id=run_step.project_id
		join task_steps step on step.id=run_step.task_step_id and step.project_id=run_step.project_id and step.uses='algorithm.run'
		where run_step.project_id=$1 and run_step.team_id=$2 and run_step.id=$3 and run.id=$4`, event.ProjectID, event.TeamID,
		payload.TaskRunStepID, payload.TaskRunID).Scan(&stepStatus, &rawParameters, &runInputs)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("TASK_ALGORITHM_INPUT_INVALID")
	}
	if err != nil {
		return err
	}
	if stepStatus == "succeeded" || stepStatus == "skipped" {
		return nil
	}
	if stepStatus == "failed" || stepStatus == "paused" {
		return errors.New("TASK_ALGORITHM_STEP_NOT_EXECUTABLE")
	}
	inputs, err := decodeObject(runInputs)
	if err != nil {
		return errors.New("TASK_ALGORITHM_RUN_INPUT_INVALID")
	}
	if nested, ok := inputs["inputs"].(map[string]any); ok {
		inputs = nested
	}
	stepOutputs := map[string]map[string]any{}
	rows, err := tx.QueryContext(ctx, `select step.step_key,run_step.output_snapshot_json
		from task_run_steps run_step join task_steps step on step.id=run_step.task_step_id and step.project_id=run_step.project_id
		where run_step.project_id=$1 and run_step.task_run_id=$2 and run_step.position <
		(select position from task_run_steps where project_id=$1 and id=$3) and run_step.status in('succeeded','skipped')`,
		event.ProjectID, payload.TaskRunID, payload.TaskRunStepID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return err
		}
		output, err := decodeObject(raw)
		if err != nil {
			return err
		}
		stepOutputs[key] = output
	}
	if err := rows.Err(); err != nil {
		return err
	}
	parameters, err := decodeObject(rawParameters)
	if err != nil {
		return errors.New("TASK_ALGORITHM_PARAMETERS_INVALID")
	}
	resolved, err := orchestration.ResolveReferences(parameters, orchestration.Context{Inputs: inputs, Steps: stepOutputs})
	if err != nil {
		return err
	}
	resolvedJSON, err := json.Marshal(resolved)
	if err != nil {
		return err
	}
	var selected struct {
		AssetID             int            `json:"assetId"`
		DefinitionVersionID int64          `json:"definitionVersionId"`
		Parameters          map[string]any `json:"parameters"`
	}
	if err := json.Unmarshal(resolvedJSON, &selected); err != nil || selected.AssetID <= 0 || selected.DefinitionVersionID <= 0 {
		return errors.New("TASK_ALGORITHM_PARAMETERS_INVALID")
	}
	var asset triggerAsset
	var definition triggerDefinition
	err = tx.QueryRowContext(ctx, `select asset.id,asset.project_id,asset.team_id,asset.version,run.id,$5::bigint,asset.device_id,
		asset.kind,asset.mime_type,coalesce(asset.checksum_sha256,''),coalesce(asset.captured_at,asset.available_at,asset.created_at),
		version.id,provider.provider_type,version.model_or_process,version.execution_mode,coalesce(version.protocol_config_json->>'mappingVersion','v1')
		from task_runs run join assets asset on asset.id=$4 and asset.project_id=run.project_id and asset.team_id=run.team_id
		and asset.task_run_id=run.id and asset.status='available'
		join algorithm_definition_versions version on version.id=$6 and version.project_id=run.project_id and version.status='published'
		join algorithm_definitions definition on definition.id=version.algorithm_definition_id and definition.project_id=version.project_id
		join algorithm_providers provider on provider.id=definition.provider_id and provider.project_id=definition.project_id and provider.status='active'
		where run.project_id=$1 and run.team_id=$2 and run.id=$3`, event.ProjectID, event.TeamID, payload.TaskRunID,
		selected.AssetID, payload.TaskRunStepID, selected.DefinitionVersionID).Scan(&asset.ID, &asset.ProjectID, &asset.TeamID,
		&asset.Version, &asset.TaskRunID, &asset.TaskRunStepID, &asset.DeviceID, &asset.Kind, &asset.MIMEType, &asset.Checksum,
		&asset.CapturedAt, &definition.VersionID, &definition.ProviderType, &definition.Model, &definition.ExecutionMode, &definition.MappingVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("TASK_ALGORITHM_RESOURCE_SCOPE_INVALID")
	}
	if err != nil {
		return err
	}
	if asset.Kind != "image" {
		return errors.New("TASK_ALGORITHM_ASSET_KIND_UNSUPPORTED")
	}
	return trigger.createRun(ctx, tx, asset, definition, selected.Parameters, resolvedJSON)
}

func (trigger *Trigger) createRun(ctx context.Context, tx *sql.Tx, asset triggerAsset, definition triggerDefinition, algorithmParameters map[string]any, resolvedParameters []byte) error {
	runID, err := randomRunID()
	if err != nil {
		return err
	}
	expiresAt := trigger.now().Add(5 * time.Minute).UTC()
	accessURL, err := trigger.issuer.IssueAssetURL(asset.ProjectID, asset.ID, asset.Version, expiresAt)
	if err != nil {
		return err
	}
	input := buildTriggeredInput(runID, asset, definition, algorithmParameters, accessURL, expiresAt)
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}
	idempotencyKey := triggerIdempotencyKey(asset, definition)
	var insertedRunID string
	err = tx.QueryRowContext(ctx, `
		insert into algorithm_runs (
		  id,project_id,team_id,algorithm_definition_version_id,input_asset_id,task_run_id,task_run_step_id,device_id,
		  idempotency_key,parameters_json,input_snapshot_json
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		on conflict(project_id,idempotency_key) do update set idempotency_key=excluded.idempotency_key returning id`,
		runID, asset.ProjectID, asset.TeamID, definition.VersionID, asset.ID, asset.TaskRunID, asset.TaskRunStepID,
		nullableInt64(asset.DeviceID), idempotencyKey, resolvedParameters, inputJSON).Scan(&insertedRunID)
	if err != nil {
		return err
	}
	executionKey := fmt.Sprintf("task-run:%d:step:%d", asset.TaskRunID, asset.TaskRunStepID)
	if _, err := tx.ExecContext(ctx, `update task_run_steps set status='running',attempt_count=greatest(attempt_count,1),
		input_snapshot_json=$3,result_json=result_json||jsonb_build_object('algorithmRunId',$4::text),execution_key=$5
		where project_id=$1 and id=$2`, asset.ProjectID, asset.TaskRunStepID, resolvedParameters, insertedRunID, executionKey); err != nil {
		return err
	}
	eventID := "algorithm-run-requested:" + insertedRunID
	payload := map[string]any{"runId": insertedRunID, "sourceAssetId": asset.ID, "taskRunId": asset.TaskRunID, "taskRunStepId": asset.TaskRunStepID}
	if _, err := tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'algorithm.run.requested',$4) on conflict(event_id) do nothing`, asset.ProjectID, asset.TeamID, eventID, payload); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'algorithm.run.requested',$4) on conflict(event_id) do nothing`, asset.ProjectID, asset.TeamID, eventID, payload)
	return err
}

func decodeObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	err := decoder.Decode(&object)
	return object, err
}

func buildTriggeredInput(runID string, asset triggerAsset, definition triggerDefinition, parameters map[string]any, accessURL string, expiresAt time.Time) Input {
	if parameters == nil {
		parameters = map[string]any{}
	}
	var deviceID any
	if asset.DeviceID.Valid {
		deviceID = asset.DeviceID.Int64
	}
	return Input{
		SchemaVersion: InputSchemaVersionV1, RunID: runID, ProjectID: asset.ProjectID,
		Definition: DefinitionReference{DefinitionVersionID: definition.VersionID, ProviderType: definition.ProviderType,
			ModelOrProcess: definition.Model, ExecutionMode: definition.ExecutionMode, MappingVersion: definition.MappingVersion},
		InputAsset: AssetReference{AssetID: asset.ID, Version: asset.Version, ChecksumSHA256: asset.Checksum,
			MIMEType: asset.MIMEType, AccessURL: accessURL, AccessExpiresAt: expiresAt},
		Context: map[string]any{"capturedAt": asset.CapturedAt.UTC().Format(time.RFC3339Nano), "taskRunId": asset.TaskRunID,
			"taskRunStepId": asset.TaskRunStepID, "deviceId": deviceID, "position": nil, "coordinateReference": nil,
			"calibrationVersion": nil, "quality": map[string]any{}},
		Parameters: parameters,
	}
}

func triggerIdempotencyKey(asset triggerAsset, definition triggerDefinition) string {
	return fmt.Sprintf("task-run:%d:step:%d:asset:%d:v%d:definition:%d",
		asset.TaskRunID, asset.TaskRunStepID, asset.ID, asset.Version, definition.VersionID)
}

func nullableInt64(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func randomRunID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	hexValue := hex.EncodeToString(value)
	return hexValue[0:8] + "-" + hexValue[8:12] + "-" + hexValue[12:16] + "-" + hexValue[16:20] + "-" + hexValue[20:32], nil
}
