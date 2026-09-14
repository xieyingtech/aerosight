package algorithm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// QueueInspectionAsset reuses the existing provider protocol and durable job
// execution, but authorizes the input through the observation binding rather
// than requiring the asset to have been captured by the current business Run.
func (trigger *Trigger) QueueInspectionAsset(ctx context.Context, tx *sql.Tx, projectID, teamID, runID int, stepID int64, observationID string, assetID int, definitionID int64, parameters map[string]any) error {
	if trigger.issuer == nil {
		return errors.New("INSPECTION_ALGORITHM_ASSET_ACCESS_UNAVAILABLE")
	}
	var asset triggerAsset
	var definition triggerDefinition
	err := tx.QueryRowContext(ctx, `select asset.id,asset.project_id,asset.team_id,asset.version,run.id,run_step.id,asset.device_id,asset.kind,asset.mime_type,
 frozen->>'checksumSha256',coalesce(asset.captured_at at time zone 'UTC',asset.available_at,asset.created_at at time zone 'UTC'),
 version.id,provider.provider_type,version.model_or_process,version.execution_mode,coalesce(version.protocol_config_json->>'mappingVersion','v1')
 from task_runs run
 join task_versions task_version on task_version.id=run.task_version_id and task_version.project_id=run.project_id and task_version.dsl_version='aerosight/v2'
 join task_run_steps run_step on run_step.task_run_id=run.id and run_step.project_id=run.project_id and run_step.id=$4 and run_step.status='running'
 join task_steps step on step.id=run_step.task_step_id and step.project_id=run.project_id and step.uses='inspection.detect'
 join inspection_observations observation on observation.task_run_id=run.id and observation.project_id=run.project_id and observation.id=$5 and observation.sealed_at is not null
 join inspection_observation_assets binding on binding.observation_id=observation.id and binding.project_id=run.project_id and binding.asset_id=$6
 join assets asset on asset.id=binding.asset_id and asset.project_id=binding.project_id and asset.version=binding.asset_version and asset.status='available' and asset.task_run_id is not distinct from binding.source_run_id
 cross join lateral jsonb_array_elements(observation.manifest_json->'assets') frozen
 join algorithm_definition_versions version on version.id=$7 and version.project_id=run.project_id and version.status='published'
 join algorithm_definitions definition on definition.id=version.algorithm_definition_id and definition.project_id=run.project_id and definition.capability_code='detection'
 join algorithm_providers provider on provider.id=definition.provider_id and provider.project_id=run.project_id and provider.status='active' and provider.provider_type='http-json'
 where run.project_id=$1 and run.team_id=$2 and run.id=$3 and run.status in('running','dispatching')
 and (frozen->>'assetId')::bigint=asset.id and (frozen->>'version')::int=asset.version
 and coalesce(frozen->>'objectVersion','')=coalesce(asset.object_version,'')
 and frozen->>'checksumSha256' ~ '^[a-f0-9]{64}$'`, projectID, teamID, runID, stepID, observationID, assetID, definitionID).Scan(&asset.ID, &asset.ProjectID, &asset.TeamID, &asset.Version, &asset.TaskRunID, &asset.TaskRunStepID, &asset.DeviceID, &asset.Kind, &asset.MIMEType, &asset.Checksum, &asset.CapturedAt, &definition.VersionID, &definition.ProviderType, &definition.Model, &definition.ExecutionMode, &definition.MappingVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("INSPECTION_ALGORITHM_INPUT_SCOPE_INVALID")
	}
	if err != nil {
		return err
	}
	if asset.Kind != "image" {
		return errors.New("INSPECTION_ALGORITHM_IMAGE_REQUIRED")
	}
	asset.Inspection = true
	raw, err := json.Marshal(parameters)
	if err != nil {
		return err
	}
	return trigger.createRun(ctx, tx, asset, definition, parameters, raw)
}
