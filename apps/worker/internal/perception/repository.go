package perception

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"aerosight/worker/internal/algorithm"
)

type SQLDetectionSink struct{}

func NewSQLDetectionSink() SQLDetectionSink { return SQLDetectionSink{} }

func (SQLDetectionSink) SaveDetections(ctx context.Context, tx *sql.Tx, projectID int, runID string, detections []algorithm.Detection) error {
	if tx == nil {
		return errors.New("detection sink requires an active transaction")
	}
	var teamID, assetID int
	var taskRunID sql.NullInt64
	var inputSnapshot []byte
	if err := tx.QueryRowContext(ctx, `select team_id,input_asset_id,task_run_id,input_snapshot_json from algorithm_runs where id=$1 and project_id=$2`, runID, projectID).Scan(&teamID, &assetID, &taskRunID, &inputSnapshot); err != nil {
		return err
	}
	capturedAt := time.Now().UTC()
	var snapshot struct {
		Context struct {
			CapturedAt string `json:"capturedAt"`
		} `json:"context"`
	}
	if json.Unmarshal(inputSnapshot, &snapshot) == nil {
		if parsed, err := time.Parse(time.RFC3339Nano, snapshot.Context.CapturedAt); err == nil {
			capturedAt = parsed
		}
	}
	for _, detection := range detections {
		pixel, err := json.Marshal(detection.PixelGeometry)
		if err != nil {
			return err
		}
		attributes, err := json.Marshal(detection.Attributes)
		if err != nil {
			return err
		}
		var detectionID int64
		err = tx.QueryRowContext(ctx, `
			insert into detections (
			 project_id,team_id,algorithm_run_id,input_asset_id,task_run_id,detection_key,label,confidence,
			 pixel_geometry_json,location_quality,projection_method,transform_version,attributes_json,captured_at
			) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,'unavailable','image-only',$10,$11,$12)
			on conflict (algorithm_run_id,detection_key) do nothing returning id`,
			projectID, teamID, runID, assetID, nullableTaskRun(taskRunID), detection.DetectionKey, detection.Label, detection.Confidence,
			pixel, ProjectionVersionV1, attributes, capturedAt).Scan(&detectionID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if err := attachImageLevelGroup(ctx, tx, projectID, teamID, assetID, detectionID, detection.Label, capturedAt); err != nil {
			return err
		}
	}
	return nil
}

func attachImageLevelGroup(ctx context.Context, tx *sql.Tx, projectID, teamID, assetID int, detectionID int64, label string, capturedAt time.Time) error {
	if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", "detection-group:image:"+label+":"+strconv.Itoa(assetID)); err != nil {
		return err
	}
	var groupID int64
	err := tx.QueryRowContext(ctx, `
		select group_row.id from detection_groups group_row
		join detection_group_members member on member.detection_group_id=group_row.id and member.project_id=group_row.project_id
		join detections detection on detection.id=member.detection_id and detection.project_id=member.project_id
		where group_row.project_id=$1 and group_row.label=$2 and group_row.status='active'
		  and group_row.location_quality='unavailable' and detection.input_asset_id=$3
		order by group_row.id limit 1 for update of group_row`, projectID, label, assetID).Scan(&groupID)
	created := false
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `insert into detection_groups(project_id,team_id,label,location_quality,first_detected_at,last_detected_at)
			values($1,$2,$3,'unavailable',$4,$4) returning id`, projectID, teamID, label, capturedAt).Scan(&groupID)
		created = true
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `insert into detection_group_members(project_id,team_id,detection_group_id,detection_id) values($1,$2,$3,$4)`, projectID, teamID, groupID, detectionID); err != nil {
		return err
	}
	if !created {
		_, err = tx.ExecContext(ctx, `update detection_groups set member_count=member_count+1,first_detected_at=least(first_detected_at,$2),last_detected_at=greatest(last_detected_at,$2),updated_at=now() where id=$1`, groupID, capturedAt)
	}
	return err
}

func nullableTaskRun(value sql.NullInt64) any {
	if value.Valid {
		return value.Int64
	}
	return nil
}
