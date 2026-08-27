package algorithm

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
	TaskRunID, DeviceID            sql.NullInt64
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

func (trigger *Trigger) Handler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var payload struct {
		AssetID int `json:"assetId"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.AssetID <= 0 {
		return errors.New("asset.available payload requires assetId")
	}
	var asset triggerAsset
	err := tx.QueryRowContext(ctx, `
		select asset.id, asset.project_id, asset.team_id, asset.version, asset.task_run_id, asset.device_id,
		       asset.kind, asset.mime_type, coalesce(asset.checksum_sha256,''),
		       coalesce(asset.captured_at, asset.available_at, asset.created_at)
		from assets asset
		join project_feature_flags flags on flags.project_id=asset.project_id and flags.external_algorithms_enabled
		where asset.id=$1 and asset.project_id=$2 and asset.team_id=$3 and asset.status='available'`,
		payload.AssetID, event.ProjectID, event.TeamID).Scan(
		&asset.ID, &asset.ProjectID, &asset.TeamID, &asset.Version, &asset.TaskRunID, &asset.DeviceID,
		&asset.Kind, &asset.MIMEType, &asset.Checksum, &asset.CapturedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if asset.Kind != "image" || !asset.TaskRunID.Valid || trigger.issuer == nil {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
		select version.id, provider.provider_type, version.model_or_process, version.execution_mode,
		       coalesce(version.protocol_config_json->>'mappingVersion','suspected-construction/v1')
		from algorithm_definitions definition
		join algorithm_definition_versions version
		  on version.id=definition.current_published_version_id and version.project_id=definition.project_id and version.status='published'
		join algorithm_providers provider
		  on provider.id=definition.provider_id and provider.project_id=definition.project_id and provider.status='active'
		where definition.project_id=$1 and definition.capability_code='perception.suspected-construction'
		order by definition.id`, asset.ProjectID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var definitions []triggerDefinition
	for rows.Next() {
		var definition triggerDefinition
		if err := rows.Scan(&definition.VersionID, &definition.ProviderType, &definition.Model, &definition.ExecutionMode, &definition.MappingVersion); err != nil {
			return err
		}
		definitions = append(definitions, definition)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, definition := range definitions {
		if err := trigger.createRun(ctx, tx, asset, definition); err != nil {
			return err
		}
	}
	return nil
}

func (trigger *Trigger) createRun(ctx context.Context, tx *sql.Tx, asset triggerAsset, definition triggerDefinition) error {
	runID, err := randomRunID()
	if err != nil {
		return err
	}
	expiresAt := trigger.now().Add(5 * time.Minute).UTC()
	accessURL, err := trigger.issuer.IssueAssetURL(asset.ProjectID, asset.ID, asset.Version, expiresAt)
	if err != nil {
		return err
	}
	input := buildTriggeredInput(runID, asset, definition, accessURL, expiresAt)
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}
	idempotencyKey := triggerIdempotencyKey(asset, definition)
	var insertedRunID string
	err = tx.QueryRowContext(ctx, `
		insert into algorithm_runs (
		  id, project_id, team_id, algorithm_definition_version_id, input_asset_id, task_run_id, device_id,
		  idempotency_key, parameters_json, input_snapshot_json
		) values ($1,$2,$3,$4,$5,$6,$7,$8,'{}'::jsonb,$9)
		on conflict (project_id,idempotency_key) do nothing returning id`,
		runID, asset.ProjectID, asset.TeamID, definition.VersionID, asset.ID, asset.TaskRunID.Int64,
		nullableInt64(asset.DeviceID), idempotencyKey, inputJSON).Scan(&insertedRunID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	eventID := "algorithm-run-requested:" + insertedRunID
	payload := map[string]any{"runId": insertedRunID, "sourceAssetId": asset.ID, "taskRunId": asset.TaskRunID.Int64}
	_, err = tx.ExecContext(ctx, `
		insert into project_events (project_id,team_id,event_id,event_type,payload_json)
		values ($1,$2,$3,'algorithm.run.requested',$4) on conflict(event_id) do nothing`,
		asset.ProjectID, asset.TeamID, eventID, payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		insert into outbox_events (project_id,team_id,event_id,event_type,payload_json)
		values ($1,$2,$3,'algorithm.run.requested',$4) on conflict(event_id) do nothing`,
		asset.ProjectID, asset.TeamID, eventID, payload)
	return err
}

func buildTriggeredInput(runID string, asset triggerAsset, definition triggerDefinition, accessURL string, expiresAt time.Time) Input {
	var taskRunID, deviceID any
	if asset.TaskRunID.Valid {
		taskRunID = asset.TaskRunID.Int64
	}
	if asset.DeviceID.Valid {
		deviceID = asset.DeviceID.Int64
	}
	return Input{
		SchemaVersion: InputSchemaVersionV1, RunID: runID, ProjectID: asset.ProjectID,
		Definition: DefinitionReference{DefinitionVersionID: definition.VersionID, ProviderType: definition.ProviderType,
			ModelOrProcess: definition.Model, ExecutionMode: definition.ExecutionMode, MappingVersion: definition.MappingVersion},
		InputAsset: AssetReference{AssetID: asset.ID, Version: asset.Version, ChecksumSHA256: asset.Checksum,
			MIMEType: asset.MIMEType, AccessURL: accessURL, AccessExpiresAt: expiresAt},
		Context: map[string]any{"capturedAt": asset.CapturedAt.UTC().Format(time.RFC3339Nano), "taskRunId": taskRunID,
			"deviceId": deviceID, "position": nil, "coordinateReference": nil, "calibrationVersion": nil, "quality": map[string]any{}},
		Parameters: map[string]any{},
	}
}

func triggerIdempotencyKey(asset triggerAsset, definition triggerDefinition) string {
	return fmt.Sprintf("suspected-construction:task:%d:asset:%d:v%d:definition:%d",
		asset.TaskRunID.Int64, asset.ID, asset.Version, definition.VersionID)
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
