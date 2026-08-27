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
		groupID, err := attachImageLevelGroup(ctx, tx, projectID, teamID, assetID, detectionID, detection.Label, capturedAt)
		if err != nil {
			return err
		}
		if err := evaluatePublishedRules(ctx, tx, projectID, teamID, groupID, detection.Label, detection.Confidence, capturedAt); err != nil {
			return err
		}
	}
	return nil
}

func attachImageLevelGroup(ctx context.Context, tx *sql.Tx, projectID, teamID, assetID int, detectionID int64, label string, capturedAt time.Time) (int64, error) {
	if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", "detection-group:image:"+label+":"+strconv.Itoa(assetID)); err != nil {
		return 0, err
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
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `insert into detection_group_members(project_id,team_id,detection_group_id,detection_id) values($1,$2,$3,$4)`, projectID, teamID, groupID, detectionID); err != nil {
		return 0, err
	}
	if !created {
		_, err = tx.ExecContext(ctx, `update detection_groups set member_count=member_count+1,first_detected_at=least(first_detected_at,$2),last_detected_at=greatest(last_detected_at,$2),updated_at=now() where id=$1`, groupID, capturedAt)
	}
	return groupID, err
}

func evaluatePublishedRules(ctx context.Context, tx *sql.Tx, projectID, teamID int, groupID int64, label string, confidence float64, detectedAt time.Time) error {
	rows, err := tx.QueryContext(ctx, `select version.id,version.minimum_confidence,version.severity,version.deduplication_window_seconds
		from event_rules rule join event_rule_versions version on version.id=rule.current_published_version_id and version.project_id=rule.project_id
		where rule.project_id=$1 and rule.status='active' and version.status='published' and version.label=$2`, projectID, label)
	if err != nil {
		return err
	}
	defer rows.Close()
	type ruleRow struct {
		id       int64
		minimum  float64
		severity string
		window   int
	}
	var rules []ruleRow
	for rows.Next() {
		var rule ruleRow
		if err := rows.Scan(&rule.id, &rule.minimum, &rule.severity, &rule.window); err != nil {
			return err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, rule := range rules {
		if confidence < rule.minimum {
			continue
		}
		key := "rule-version:" + strconv.FormatInt(rule.id, 10) + ":group:" + strconv.FormatInt(groupID, 10)
		if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", "perception-event:"+strconv.Itoa(projectID)+":"+key); err != nil {
			return err
		}
		var eventID string
		err := tx.QueryRowContext(ctx, `select id from perception_events where project_id=$1 and deduplication_key=$2 and status in('open','acknowledged','investigating') for update`, projectID, key).Scan(&eventID)
		created := false
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, `insert into perception_events(id,project_id,team_id,event_rule_version_id,detection_group_id,deduplication_key,severity,first_detected_at,last_detected_at)
				values(gen_random_uuid(),$1,$2,$3,$4,$5,$6,$7,$7) returning id`, projectID, teamID, rule.id, groupID, key, rule.severity, detectedAt).Scan(&eventID)
			created = true
		} else if err == nil {
			_, err = tx.ExecContext(ctx, `update perception_events set occurrence_count=occurrence_count+1,last_detected_at=greatest(last_detected_at,$2),severity=$3,updated_at=now() where id=$1`, eventID, detectedAt, rule.severity)
		}
		if err != nil {
			return err
		}
		eventType := "perception.event.updated"
		if created {
			eventType = "perception.event.created"
		}
		projectEventID := eventType + ":" + eventID + ":" + strconv.FormatInt(detectedAt.UnixNano(), 10)
		if _, err = tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json) values($1,$2,$3,$4,jsonb_build_object('eventId',$5::text,'detectionGroupId',$6::bigint)) on conflict(event_id) do nothing`, projectID, teamID, projectEventID, eventType, eventID, groupID); err != nil {
			return err
		}
	}
	return nil
}

func nullableTaskRun(value sql.NullInt64) any {
	if value.Valid {
		return value.Int64
	}
	return nil
}
