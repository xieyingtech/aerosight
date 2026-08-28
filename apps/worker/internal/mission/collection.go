package mission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aerosight/worker/internal/outbox"
)

type assetAvailablePayload struct {
	AssetID int `json:"assetId"`
}

type collectedAsset struct {
	ID, Version, TaskRunID, TeamID int
	StepID                         int64
	MIMEType, Checksum             string
	CapturedAt                     time.Time
}

// CompleteCollectionStep makes media availability, rather than a device ACK,
// the immutable output boundary for device.collect steps.
func CompleteCollectionStep(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var payload assetAvailablePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.AssetID <= 0 {
		return errors.New("asset.available payload requires a valid assetId")
	}
	var asset collectedAsset
	err := tx.QueryRowContext(ctx, `select asset.id,asset.version,run.id,run.team_id,run_step.id,
		coalesce(asset.mime_type,''),coalesce(asset.checksum_sha256,asset.checksum,''),
		coalesce(asset.captured_at,asset.available_at,asset.created_at)
		from assets asset join task_runs run on run.id=asset.task_run_id and run.project_id=asset.project_id and run.team_id=asset.team_id
		join task_run_steps run_step on run_step.task_run_id=run.id and run_step.project_id=run.project_id and run_step.status='running'
		join task_steps step on step.id=run_step.task_step_id and step.project_id=run_step.project_id and step.uses='device.collect'
		where asset.project_id=$1 and asset.team_id=$2 and asset.id=$3 and asset.status='available'
		and run.status in('dispatching','running') and (asset.device_id is null or asset.device_id=run.selected_device_id)
		order by run_step.position limit 1 for update of run_step`, event.ProjectID, event.TeamID, payload.AssetID).Scan(
		&asset.ID, &asset.Version, &asset.TaskRunID, &asset.TeamID, &asset.StepID,
		&asset.MIMEType, &asset.Checksum, &asset.CapturedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	output, err := json.Marshal(map[string]any{
		"assetId": asset.ID, "assetVersion": asset.Version, "mimeType": asset.MIMEType,
		"checksumSha256": asset.Checksum, "capturedAt": asset.CapturedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `update task_run_steps set status='succeeded',attempt_count=greatest(attempt_count,1),
		output_snapshot_json=$3,result_json=result_json||$3,finished_at=now()
		where project_id=$1 and id=$2 and status='running'`, event.ProjectID, asset.StepID, output)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated == 0 {
		return err
	}
	continuation := map[string]any{"taskRunId": asset.TaskRunID, "to": "running", "completedStepId": asset.StepID}
	_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, event.ProjectID, asset.TeamID,
		fmt.Sprintf("task-collect-complete:%d:step:%d:asset:%d:v%d", asset.TaskRunID, asset.StepID, asset.ID, asset.Version), continuation)
	return err
}
