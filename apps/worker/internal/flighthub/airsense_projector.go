package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"aerosight/worker/internal/connector"
)

const airSenseRuleName = "DJI FlightHub AirSense safety"

func airSenseSeverity(level int) string {
	switch level {
	case 3:
		return "critical"
	case 2:
		return "high"
	default:
		return "medium"
	}
}

func ensureAirSenseRule(ctx context.Context, tx *sql.Tx, projectID, teamID int, connectorID int64) (int64, error) {
	name := fmt.Sprintf("%s · %d", airSenseRuleName, connectorID)
	if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", fmt.Sprintf("flighthub-airsense-rule:%d:%d", projectID, connectorID)); err != nil {
		return 0, err
	}
	var ruleID int64
	err := tx.QueryRowContext(ctx, `select id from event_rules where project_id=$1 and name=$2`, projectID, name).Scan(&ruleID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `insert into event_rules(project_id,team_id,name,status) values($1,$2,$3,'active') returning id`, projectID, teamID, name).Scan(&ruleID)
	}
	if err != nil {
		return 0, err
	}
	var versionID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `select current_published_version_id from event_rules where project_id=$1 and id=$2`, projectID, ruleID).Scan(&versionID); err != nil {
		return 0, err
	}
	if versionID.Valid && versionID.Int64 > 0 {
		return versionID.Int64, nil
	}
	conditions, _ := json.Marshal(map[string]any{"source": "dji-flighthub-openapi", "kind": "air-sense", "connectorInstanceId": connectorID})
	var created int64
	err = tx.QueryRowContext(ctx, `insert into event_rule_versions(
		project_id,team_id,event_rule_id,version,status,label,minimum_confidence,severity,deduplication_window_seconds,conditions_json,published_at
	) values($1,$2,$3,1,'published','DJI FlightHub AirSense',0,'high',300,$4,now()) returning id`,
		projectID, teamID, ruleID, conditions).Scan(&created)
	if err == nil {
		_, err = tx.ExecContext(ctx, `update event_rules set current_published_version_id=$3,updated_at=now() where project_id=$1 and id=$2`, projectID, ruleID, created)
	}
	return created, err
}

func resolveAirSenseTaskRun(ctx context.Context, tx *sql.Tx, projectID, deviceID int) (*int, error) {
	var runID int
	err := tx.QueryRowContext(ctx, `select id from task_runs where project_id=$1 and selected_device_id=$2
		and status in('dispatching','running','paused','canceling') order by created_at desc,id desc limit 1`, projectID, deviceID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &runID, nil
}

type airSenseProjection struct {
	resourceID int64
	eventID    sql.NullString
	groupID    sql.NullInt64
	issueID    sql.NullInt64
}

func loadAirSenseProjection(ctx context.Context, tx *sql.Tx, instance connector.Instance, remoteID string) (airSenseProjection, error) {
	var projection airSenseProjection
	err := tx.QueryRowContext(ctx, `select resource.id,resource.canonical_target_id,event.detection_group_id,link.issue_id
		from connector_remote_resources resource
		left join perception_events event on event.project_id=resource.project_id
		 and resource.canonical_target_type='perception_event' and resource.canonical_target_id=event.id::text
		left join issue_links link on link.project_id=resource.project_id and link.link_type='perception_event' and link.target_id=event.id::text
		where resource.project_id=$1 and resource.connector_instance_id=$2 and resource.resource_kind='air-sense-warning' and resource.remote_id=$3
		for update of resource`, instance.ProjectID, instance.ID, remoteID).Scan(&projection.resourceID, &projection.eventID, &projection.groupID, &projection.issueID)
	return projection, err
}

func createAirSenseProjection(ctx context.Context, tx *sql.Tx, instance connector.Instance, teamID int, ruleVersionID int64,
	projection airSenseProjection, deviceID int, runID *int, event AirSenseWarningEvent, capturedAt time.Time, active bool,
) error {
	severity := airSenseSeverity(event.WarningLevel)
	status := "resolved"
	groupStatus := "superseded"
	issueStatus := "closed"
	if active {
		status, groupStatus, issueStatus = "open", "active", "open"
	}
	var groupID int64
	err := tx.QueryRowContext(ctx, `insert into detection_groups(
		project_id,team_id,label,status,geographic_geometry,location_quality,first_detected_at,last_detected_at,aggregation_version
	) values($1,$2,'AirSense 空域目标',$3,st_buffer(st_setsrid(st_makepoint($4,$5),4326)::geography,1)::geometry,
		'low',$6,$6,'dji-flighthub-airsense/unverified-v1') returning id`, instance.ProjectID, teamID, groupStatus,
		event.Longitude, event.Latitude, capturedAt).Scan(&groupID)
	if err != nil {
		return err
	}
	var eventID string
	err = tx.QueryRowContext(ctx, `insert into perception_events(
		id,project_id,team_id,event_rule_version_id,detection_group_id,deduplication_key,title,severity,status,
		occurrence_count,state_version,first_detected_at,last_detected_at,resolved_at
	) values(gen_random_uuid(),$1,$2,$3,$4,$5,'司空 AirSense 空域告警',$6,$7,1,0,$8,$8,
		case when $7='resolved' then $8 else null end) returning id`, instance.ProjectID, teamID, ruleVersionID, groupID,
		fmt.Sprintf("dji-flighthub:%d:airsense:%d", instance.ID, projection.resourceID), severity, status, capturedAt).Scan(&eventID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", fmt.Sprintf("issue-number:%d", instance.ProjectID)); err != nil {
		return err
	}
	var number int
	if err := tx.QueryRowContext(ctx, `select coalesce(max(number),0)+1 from issues where project_id=$1`, instance.ProjectID).Scan(&number); err != nil {
		return err
	}
	labels, _ := json.Marshal([]string{"dji-flighthub", "air-sense", severity})
	var issueID int
	err = tx.QueryRowContext(ctx, `insert into issues(
		project_id,number,title,description,source_type,status,priority,task_run_id,condition_scope_key,business_object_key,
		occurrence_count,first_seen_at,last_seen_at,labels_json,closed_at
	) values($1,$2,'司空 AirSense 空域告警','来自 DJI FlightHub AirSense 的空域安全目标','dji-flighthub-airsense',
		$3,$4,$5,$6,$7,1,$8,$8,$9,case when $3='closed' then $8 else null end) returning id`, instance.ProjectID,
		number, issueStatus, alertPriority(severity), runID, fmt.Sprintf("dji-flighthub:%d:air-sense", instance.ID),
		strconv.FormatInt(projection.resourceID, 10), capturedAt, labels).Scan(&issueID)
	if err != nil {
		return err
	}
	for _, link := range []struct{ kind, id string }{
		{"perception_event", eventID}, {"device", strconv.Itoa(deviceID)}, {"spatial_group", strconv.FormatInt(groupID, 10)},
	} {
		if _, err := tx.ExecContext(ctx, `insert into issue_links(project_id,issue_id,link_type,target_id) values($1,$2,$3,$4)
			on conflict(issue_id,link_type,target_id) do nothing`, instance.ProjectID, issueID, link.kind, link.id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `update connector_remote_resources set canonical_target_type='perception_event',canonical_target_id=$4,updated_at=now()
		where project_id=$1 and connector_instance_id=$2 and id=$3`, instance.ProjectID, instance.ID, projection.resourceID, eventID); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"perceptionEventId": eventID, "issueId": issueID, "source": "dji-flighthub-openapi", "kind": "air-sense"})
	_, err = tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json,occurred_at)
		values($1,$2,$3,'perception_event.created',$4,$5) on conflict(event_id) do nothing`, instance.ProjectID, teamID,
		fmt.Sprintf("flighthub-airsense:%d:%d:created", instance.ProjectID, projection.resourceID), payload, capturedAt)
	return err
}

func updateAirSenseProjection(ctx context.Context, tx *sql.Tx, instance connector.Instance, projection airSenseProjection,
	deviceID int, runID *int, event AirSenseWarningEvent, capturedAt time.Time, active bool,
) error {
	if !projection.eventID.Valid || !projection.groupID.Valid || !projection.issueID.Valid {
		return errors.New("FlightHub AirSense canonical projection is invalid")
	}
	severity := airSenseSeverity(event.WarningLevel)
	status, groupStatus, issueStatus := "resolved", "superseded", "closed"
	if active {
		status, groupStatus, issueStatus = "open", "active", "open"
	}
	_, err := tx.ExecContext(ctx, `update detection_groups set status=$3,
		geographic_geometry=st_buffer(st_setsrid(st_makepoint($4,$5),4326)::geography,1)::geometry,
		location_quality='low',first_detected_at=least(first_detected_at,$6),last_detected_at=greatest(last_detected_at,$6),updated_at=now()
		where project_id=$1 and id=$2`, instance.ProjectID, projection.groupID.Int64, groupStatus, event.Longitude, event.Latitude, capturedAt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `update perception_events set title='司空 AirSense 空域告警',severity=$3,status=$4,
		occurrence_count=occurrence_count+case when status in('resolved','dismissed') and $4='open' then 1 else 0 end,
		state_version=state_version+case when status<>$4 then 1 else 0 end,
		first_detected_at=least(first_detected_at,$5),last_detected_at=greatest(last_detected_at,$5),
		resolved_at=case when $4='resolved' then $5 else null end,updated_at=now() where project_id=$1 and id=$2`,
		instance.ProjectID, projection.eventID.String, severity, status, capturedAt)
	if err != nil {
		return err
	}
	labels, _ := json.Marshal([]string{"dji-flighthub", "air-sense", severity})
	_, err = tx.ExecContext(ctx, `update issues set title='司空 AirSense 空域告警',priority=$3,status=$4,
		task_run_id=coalesce($5,task_run_id),occurrence_count=occurrence_count+case when status='closed' and $4='open' then 1 else 0 end,
		state_version=state_version+case when status<>$4 then 1 else 0 end,last_seen_at=greatest(last_seen_at,$6),labels_json=$7,
		closed_at=case when $4='closed' then $6 else null end,updated_at=now() where project_id=$1 and id=$2`,
		instance.ProjectID, projection.issueID.Int64, alertPriority(severity), issueStatus, runID, capturedAt, labels)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `insert into issue_links(project_id,issue_id,link_type,target_id) values($1,$2,'device',$3)
		on conflict(issue_id,link_type,target_id) do nothing`, instance.ProjectID, projection.issueID.Int64, strconv.Itoa(deviceID))
	return err
}

func (projector *SQLFlightCatalogProjector) ApplyAirSense(ctx context.Context, instance connector.Instance, poll AirSensePoll) (returnedErr error) {
	if projector == nil || projector.db == nil || poll.ReceivedAt.IsZero() || poll.Devices == nil || poll.Warnings == nil {
		return errors.New("FlightHub AirSense projection is invalid")
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
	ruleVersionID, err := ensureAirSenseRule(ctx, tx, instance.ProjectID, teamID, instance.ID)
	if err != nil {
		return err
	}
	bySerial := make(map[string]int, len(poll.Devices))
	for _, device := range poll.Devices {
		bySerial[device.Serial] = device.DeviceID
	}
	for warningIndex := range poll.Warnings {
		warning := poll.Warnings[warningIndex]
		deviceID := bySerial[warning.DeviceSN]
		if deviceID <= 0 {
			return errors.New("FlightHub AirSense device is outside managed scope")
		}
		runID, err := resolveAirSenseTaskRun(ctx, tx, instance.ProjectID, deviceID)
		if err != nil {
			return err
		}
		for eventIndex := range warning.Events {
			event := warning.Events[eventIndex]
			remoteID := airSenseRemoteID(warning.DeviceSN, event.ICAO)
			projection, err := loadAirSenseProjection(ctx, tx, instance, remoteID)
			if err != nil {
				return err
			}
			active := warning.Enabled && warning.ExpiresAt.After(poll.ReceivedAt.UTC()) && !warning.Expired
			if !projection.eventID.Valid {
				err = createAirSenseProjection(ctx, tx, instance, teamID, ruleVersionID, projection, deviceID, runID, event, warning.CapturedAt.UTC(), active)
			} else {
				err = updateAirSenseProjection(ctx, tx, instance, projection, deviceID, runID, event, warning.CapturedAt.UTC(), active)
			}
			if err != nil {
				return err
			}
		}
	}
	if poll.CompleteSnapshot {
		rows, err := tx.QueryContext(ctx, `select resource.remote_id from connector_remote_resources resource
			where resource.project_id=$1 and resource.connector_instance_id=$2 and resource.resource_kind='air-sense-warning'
			and resource.status='missing' and resource.canonical_target_type='perception_event'`, instance.ProjectID, instance.ID)
		if err != nil {
			return err
		}
		var missing []string
		for rows.Next() {
			var remoteID string
			if err := rows.Scan(&remoteID); err != nil {
				rows.Close()
				return err
			}
			missing = append(missing, remoteID)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, remoteID := range missing {
			projection, err := loadAirSenseProjection(ctx, tx, instance, remoteID)
			if err != nil {
				return err
			}
			if !projection.eventID.Valid || !projection.groupID.Valid || !projection.issueID.Valid {
				return errors.New("FlightHub AirSense canonical projection is invalid")
			}
			_, err = tx.ExecContext(ctx, `update perception_events set status='resolved',state_version=state_version+case when status='resolved' then 0 else 1 end,
				resolved_at=coalesce(resolved_at,$3),updated_at=now() where project_id=$1 and id=$2`, instance.ProjectID, projection.eventID.String, poll.ReceivedAt.UTC())
			if err == nil {
				_, err = tx.ExecContext(ctx, `update detection_groups set status='superseded',updated_at=now() where project_id=$1 and id=$2`, instance.ProjectID, projection.groupID.Int64)
			}
			if err == nil {
				_, err = tx.ExecContext(ctx, `update issues set status='closed',state_version=state_version+case when status='closed' then 0 else 1 end,
					closed_at=coalesce(closed_at,$3),updated_at=now() where project_id=$1 and id=$2`, instance.ProjectID, projection.issueID.Int64, poll.ReceivedAt.UTC())
			}
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
