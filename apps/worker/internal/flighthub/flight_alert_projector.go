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

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/observability"
)

const flightAlertRuleName = "DJI FlightHub AI alerts"

type alertCanonical struct {
	ResourceID    int64
	RemoteVersion string
	TargetType    sql.NullString
	TargetID      sql.NullString
}

type alertProjection struct {
	EventID string
	GroupID int64
	IssueID int
	Status  string
}

func alertText(value string, limit int) string {
	value = observability.String(value)
	lower := strings.ToLower(value)
	if strings.Contains(lower, "https://") || strings.Contains(lower, "http://") {
		value = "[REDACTED]"
	}
	return boundedDiagnostic(value, limit)
}

func alertDigest(value any) (string, error) {
	serialized, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(serialized)
	return hex.EncodeToString(digest[:16]), nil
}

func validAIAlertLocation(location *AIAlertLocation) bool {
	if location == nil || location.Latitude == nil || location.Longitude == nil {
		return false
	}
	latitude, longitude := *location.Latitude, *location.Longitude
	return latitude > -90 && latitude < 90 && longitude >= -180 && longitude <= 180 &&
		!math.IsNaN(latitude) && !math.IsInf(latitude, 0) && !math.IsNaN(longitude) && !math.IsInf(longitude, 0)
}

func alertLabel(alert AIAlertRecord) string {
	for _, target := range alert.Targets {
		if label := strings.TrimSpace(target.Label); label != "" {
			return alertText(label, 96)
		}
	}
	for _, label := range alert.Labels {
		if label = strings.TrimSpace(label); label != "" {
			return alertText(label, 96)
		}
	}
	return fmt.Sprintf("algorithm-%d", alert.AlgorithmSource)
}

func alertConfidence(alert AIAlertRecord) float64 {
	confidence := 0.0
	for _, target := range alert.Targets {
		if target.Confidence > confidence {
			confidence = target.Confidence
		}
	}
	return confidence
}

func alertSeverity(confidence float64) string {
	switch {
	case confidence >= 0.9:
		return "high"
	case confidence >= 0.6:
		return "medium"
	default:
		return "low"
	}
}

func alertPriority(severity string) string {
	if severity == "high" || severity == "critical" {
		return "high"
	}
	return severity
}

func alertLabels(alert AIAlertRecord) []string {
	result := []string{"dji-flighthub", "ai-alert", fmt.Sprintf("algorithm-%d", alert.AlgorithmSource)}
	seen := map[string]struct{}{"dji-flighthub": {}, "ai-alert": {}, result[2]: {}}
	for _, value := range append([]string{alertLabel(alert)}, alert.Labels...) {
		value = alertText(value, 64)
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == 12 {
			break
		}
	}
	return result
}

func resolveAlertTaskRun(ctx context.Context, tx *sql.Tx, instance connector.Instance, flightID string) (*int, error) {
	var runID int
	err := tx.QueryRowContext(ctx, `select run.id from connector_remote_resources resource
		join task_runs run on run.project_id=resource.project_id
		 and resource.canonical_target_type='task_run' and resource.canonical_target_id=run.id::text
		where resource.project_id=$1 and resource.connector_instance_id=$2
		 and resource.resource_kind='flight-task' and resource.remote_id=$3 and resource.status='active'`,
		instance.ProjectID, instance.ID, flightID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &runID, nil
}

func upsertFlightAlertAggregate(ctx context.Context, tx *sql.Tx, instance connector.Instance, teamID int, item FlightAlertSummary) error {
	runID, err := resolveAlertTaskRun(ctx, tx, instance, item.FlightID)
	if err != nil {
		return err
	}
	remoteVersion, err := alertDigest(map[string]any{
		"count": item.Count, "taskName": item.TaskName, "taskType": item.TaskType,
		"startTime": item.StartTime, "statusHint": item.Status, "commented": item.IsCommented,
	})
	if err != nil {
		return err
	}
	summary := map[string]any{
		"alertCount": item.Count, "taskName": alertText(item.TaskName, 160), "taskType": item.TaskType,
		"startedAt": time.Unix(item.StartTime, 0).UTC().Format(time.RFC3339Nano), "statusHintOnly": item.Status,
		"statusHintReliable": false, "isCommented": item.IsCommented,
	}
	if runID != nil {
		summary["taskRunId"] = *runID
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	var canonicalType, canonicalID any
	if runID != nil {
		canonicalType, canonicalID = "task_run", strconv.Itoa(*runID)
	}
	_, err = tx.ExecContext(ctx, `insert into connector_remote_resources(
		project_id,team_id,connector_instance_id,resource_kind,remote_id,remote_version,remote_updated_at,status,
		summary_json,canonical_target_type,canonical_target_id
	) values($1,$2,$3,'flight-alert',$4,$5,$6,'active',$7,$8,$9)
	 on conflict(project_id,connector_instance_id,resource_kind,remote_id) do update set
		team_id=excluded.team_id,remote_version=excluded.remote_version,remote_updated_at=excluded.remote_updated_at,
		status='active',summary_json=excluded.summary_json,
		canonical_target_type=coalesce(excluded.canonical_target_type,connector_remote_resources.canonical_target_type),
		canonical_target_id=coalesce(excluded.canonical_target_id,connector_remote_resources.canonical_target_id),
		last_seen_at=now(),missing_at=null,updated_at=now()`, instance.ProjectID, teamID, instance.ID, item.FlightID,
		remoteVersion, time.Unix(item.StartTime, 0).UTC(), summaryJSON, canonicalType, canonicalID)
	return err
}

func ensureFlightAlertRule(ctx context.Context, tx *sql.Tx, projectID, teamID int, connectorID int64) (int64, error) {
	name := fmt.Sprintf("%s · %d", flightAlertRuleName, connectorID)
	if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", fmt.Sprintf("flighthub-alert-rule:%d:%d", projectID, connectorID)); err != nil {
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
	var currentVersion sql.NullInt64
	err = tx.QueryRowContext(ctx, `select current_published_version_id from event_rules where project_id=$1 and id=$2`, projectID, ruleID).Scan(&currentVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, connector.ErrRemoteResourceUnavailable
	}
	if err == nil && currentVersion.Valid && currentVersion.Int64 > 0 {
		return currentVersion.Int64, nil
	}
	var versionID int64
	conditions, _ := json.Marshal(map[string]any{"source": "dji-flighthub-openapi", "connectorInstanceId": connectorID})
	err = tx.QueryRowContext(ctx, `insert into event_rule_versions(
		project_id,team_id,event_rule_id,version,status,label,minimum_confidence,severity,deduplication_window_seconds,conditions_json,published_at
	) values($1,$2,$3,1,'published','DJI FlightHub AI',0,'medium',31536000,$4,now()) returning id`,
		projectID, teamID, ruleID, conditions).Scan(&versionID)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `update event_rules set current_published_version_id=$3,updated_at=now() where project_id=$1 and id=$2`, projectID, ruleID, versionID)
	return versionID, err
}

func upsertAIAlertResource(ctx context.Context, tx *sql.Tx, instance connector.Instance, teamID int, alert AIAlertRecord, runID, deviceID, assetID *int, capturedAt time.Time, confidence float64) (alertCanonical, error) {
	versionSource := map[string]any{
		"status": alert.Status, "reason": alert.Reason, "algorithmSource": alert.AlgorithmSource,
		"timestamp": alert.Timestamp, "fileID": alert.FileID, "mediaIndex": alert.MediaIndex,
		"targets": alert.Targets, "labels": alert.Labels, "triggerActions": alert.TriggerActions, "intervalSeconds": alert.IntervalSeconds,
	}
	if alert.Location != nil {
		versionSource["location"] = alert.Location
	}
	remoteVersion, err := alertDigest(versionSource)
	if err != nil {
		return alertCanonical{}, err
	}
	summary := map[string]any{
		"source": "dji-flighthub-openapi", "processingStatus": alert.Status, "algorithmSource": alert.AlgorithmSource,
		"capturedAt": capturedAt.Format(time.RFC3339Nano), "label": alertLabel(alert), "confidence": confidence,
		"coordinateReference": "unverified", "locationAvailable": validAIAlertLocation(alert.Location),
		"hasMedia": assetID != nil, "targetCount": len(alert.Targets), "triggerActionCount": len(alert.TriggerActions),
	}
	if runID != nil {
		summary["taskRunId"] = *runID
	}
	if deviceID != nil {
		summary["deviceId"] = *deviceID
	}
	if assetID != nil {
		summary["assetId"] = *assetID
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return alertCanonical{}, err
	}
	var result alertCanonical
	err = tx.QueryRowContext(ctx, `insert into connector_remote_resources(
		project_id,team_id,connector_instance_id,resource_kind,remote_id,remote_version,remote_updated_at,status,summary_json
	) values($1,$2,$3,'ai-alert',$4,$5,$6,'active',$7)
	 on conflict(project_id,connector_instance_id,resource_kind,remote_id) do update set
		team_id=excluded.team_id,remote_version=excluded.remote_version,remote_updated_at=excluded.remote_updated_at,
		status='active',summary_json=excluded.summary_json,last_seen_at=now(),missing_at=null,updated_at=now()
	 returning id,coalesce(remote_version,''),canonical_target_type,canonical_target_id`, instance.ProjectID, teamID, instance.ID,
		alert.AlertUUID, remoteVersion, capturedAt, summaryJSON).Scan(&result.ResourceID, &result.RemoteVersion, &result.TargetType, &result.TargetID)
	return result, err
}

func insertAlertGroup(ctx context.Context, tx *sql.Tx, projectID, teamID int, label string, capturedAt time.Time, location *AIAlertLocation) (int64, error) {
	var groupID int64
	if validAIAlertLocation(location) {
		err := tx.QueryRowContext(ctx, `insert into detection_groups(
			project_id,team_id,label,geographic_geometry,location_quality,first_detected_at,last_detected_at,aggregation_version
		) values($1,$2,$3,st_buffer(st_setsrid(st_makepoint($4,$5),4326)::geography,1)::geometry,'low',$6,$6,'dji-flighthub-ai-alert/unverified-v1') returning id`,
			projectID, teamID, label, *location.Longitude, *location.Latitude, capturedAt).Scan(&groupID)
		return groupID, err
	}
	err := tx.QueryRowContext(ctx, `insert into detection_groups(
		project_id,team_id,label,location_quality,first_detected_at,last_detected_at,aggregation_version
	) values($1,$2,$3,'unavailable',$4,$4,'dji-flighthub-ai-alert/unverified-v1') returning id`,
		projectID, teamID, label, capturedAt).Scan(&groupID)
	return groupID, err
}

func updateAlertGroup(ctx context.Context, tx *sql.Tx, groupID int64, label string, capturedAt time.Time, location *AIAlertLocation) error {
	if validAIAlertLocation(location) {
		_, err := tx.ExecContext(ctx, `update detection_groups set label=$2,status='active',
			geographic_geometry=st_buffer(st_setsrid(st_makepoint($3,$4),4326)::geography,1)::geometry,
			location_quality='low',first_detected_at=least(first_detected_at,$5),last_detected_at=greatest(last_detected_at,$5),updated_at=now()
			where id=$1`, groupID, label, *location.Longitude, *location.Latitude, capturedAt)
		return err
	}
	_, err := tx.ExecContext(ctx, `update detection_groups set label=$2,status='active',
		first_detected_at=least(first_detected_at,$3),last_detected_at=greatest(last_detected_at,$3),updated_at=now()
		where id=$1`, groupID, label, capturedAt)
	return err
}

func createAlertIssue(ctx context.Context, tx *sql.Tx, instance connector.Instance, eventID string, runID, droneID, gatewayID, assetID *int, alert AIAlertRecord, severity string, capturedAt time.Time) (int, error) {
	if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", fmt.Sprintf("issue-number:%d", instance.ProjectID)); err != nil {
		return 0, err
	}
	var number int
	if err := tx.QueryRowContext(ctx, `select coalesce(max(number),0)+1 from issues where project_id=$1`, instance.ProjectID).Scan(&number); err != nil {
		return 0, err
	}
	labels, _ := json.Marshal(alertLabels(alert))
	title := fmt.Sprintf("司空 AI 告警 · %s", alertLabel(alert))
	description := alertText(alert.Reason, 512)
	if alert.Reason == "" {
		description = fmt.Sprintf("算法来源 %d；处理状态 %d", alert.AlgorithmSource, alert.Status)
	}
	var issueID int
	err := tx.QueryRowContext(ctx, `insert into issues(
		project_id,number,title,description,source_type,status,priority,task_run_id,condition_scope_key,business_object_key,
		occurrence_count,first_seen_at,last_seen_at,labels_json
	) values($1,$2,$3,$4,'dji-flighthub-ai','open',$5,$6,$7,$8,1,$9,$9,$10) returning id`, instance.ProjectID, number,
		title, description, alertPriority(severity), runID, fmt.Sprintf("dji-flighthub:%d:ai-alert", instance.ID),
		secureRemoteKey(alert.AlertUUID), capturedAt, labels).Scan(&issueID)
	if err != nil {
		return 0, err
	}
	links := []struct {
		kind string
		id   string
	}{{kind: "perception_event", id: eventID}}
	for _, link := range []struct {
		kind string
		id   *int
	}{{"device", droneID}, {"device", gatewayID}, {"asset", assetID}} {
		if link.id != nil {
			links = append(links, struct{ kind, id string }{kind: link.kind, id: strconv.Itoa(*link.id)})
		}
	}
	for _, link := range links {
		if _, err := tx.ExecContext(ctx, `insert into issue_links(project_id,issue_id,link_type,target_id)
			values($1,$2,$3,$4) on conflict(issue_id,link_type,target_id) do nothing`, instance.ProjectID, issueID, link.kind, link.id); err != nil {
			return 0, err
		}
	}
	metadata, _ := json.Marshal(map[string]any{"source": "dji-flighthub-openapi", "status": "open", "algorithmSource": alert.AlgorithmSource})
	if _, err := tx.ExecContext(ctx, `insert into issue_events(project_id,issue_id,event_type,metadata_json) values($1,$2,'issue.created',$3)`, instance.ProjectID, issueID, metadata); err != nil {
		return 0, err
	}
	if assetID != nil {
		if _, err := tx.ExecContext(ctx, `update assets set issue_id=$3 where project_id=$1 and id=$2`, instance.ProjectID, *assetID, issueID); err != nil {
			return 0, err
		}
	}
	return issueID, nil
}

func loadAlertProjection(ctx context.Context, tx *sql.Tx, projectID int, canonical alertCanonical) (alertProjection, error) {
	if !canonical.TargetType.Valid || canonical.TargetType.String != "perception_event" || !canonical.TargetID.Valid {
		return alertProjection{}, errors.New("FlightHub AI alert canonical link is invalid")
	}
	var result alertProjection
	result.EventID = canonical.TargetID.String
	err := tx.QueryRowContext(ctx, `select event.detection_group_id,event.status,link.issue_id
		from perception_events event join issue_links link
		 on link.project_id=event.project_id and link.link_type='perception_event' and link.target_id=event.id::text
		where event.project_id=$1 and event.id=$2 for update of event`, projectID, result.EventID).Scan(&result.GroupID, &result.Status, &result.IssueID)
	return result, err
}

func insertPerceptionCreatedEvent(ctx context.Context, tx *sql.Tx, projectID, teamID int, eventID string, issueID int, capturedAt time.Time) error {
	payload, _ := json.Marshal(map[string]any{"perceptionEventId": eventID, "issueId": issueID, "source": "dji-flighthub-openapi"})
	_, err := tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json,occurred_at)
		values($1,$2,$3,'perception_event.created',$4,$5) on conflict(event_id) do nothing`, projectID, teamID,
		fmt.Sprintf("flighthub-ai-alert:%d:%s:created", projectID, eventID), payload, capturedAt)
	return err
}

func (projector *SQLFlightCatalogProjector) projectAIAlert(ctx context.Context, tx *sql.Tx, instance connector.Instance, teamID int, ruleVersionID int64, alert AIAlertRecord) error {
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil || alert.ProjectID != scope.ProjectUUID {
		return errors.New("FlightHub AI alert project scope is invalid")
	}
	capturedAt, ok := flightTrackTime(alert.Timestamp)
	if !ok {
		return schemaError()
	}
	runID, err := resolveAlertTaskRun(ctx, tx, instance, alert.FlightID)
	if err != nil {
		return err
	}
	droneID, err := selectedTaskDevice(ctx, tx, instance, alert.DroneSN)
	if err != nil || droneID == nil {
		return errors.New("FlightHub AI alert device is outside managed scope")
	}
	var gatewayID *int
	if alert.GatewaySN != "" {
		gatewayID, err = selectedTaskDevice(ctx, tx, instance, alert.GatewaySN)
		if err != nil {
			return err
		}
	}
	var assetID *int
	if alert.FileID > 0 || alert.MediaIndex > 0 {
		mediaIdentity := secureRemoteKey(strings.Join([]string{alert.AlertUUID, strconv.FormatInt(alert.FileID, 10), strconv.FormatInt(alert.MediaIndex, 10)}, ":"))
		mediaVersion, digestErr := alertDigest(map[string]any{"timestamp": alert.Timestamp, "fileID": alert.FileID, "mediaIndex": alert.MediaIndex, "status": alert.Status})
		if digestErr != nil {
			return digestErr
		}
		value, assetErr := projector.upsertExternalAsset(ctx, tx, instance, teamID, externalAssetInput{
			ResourceKind: "flight-media", RemoteID: mediaIdentity, RemoteVersion: mediaVersion, RemoteUpdatedAt: &capturedAt,
			TaskRunID: runID, DeviceID: droneID, AssetKind: "image", MIMEType: "image/*", Status: "available", CapturedAt: &capturedAt,
			Summary:  map[string]any{"sourceKind": "ai-alert-media", "capturedAt": capturedAt.Format(time.RFC3339Nano)},
			Metadata: map[string]any{"source": "dji-flighthub-openapi", "sourceKind": "ai-alert-media", "remoteReference": true, "temporaryAccess": true},
		})
		if assetErr != nil {
			return assetErr
		}
		assetID = &value
	}
	confidence := alertConfidence(alert)
	canonical, err := upsertAIAlertResource(ctx, tx, instance, teamID, alert, runID, droneID, assetID, capturedAt, confidence)
	if err != nil {
		return err
	}
	label, severity := alertLabel(alert), alertSeverity(confidence)
	if !canonical.TargetType.Valid && !canonical.TargetID.Valid {
		groupID, err := insertAlertGroup(ctx, tx, instance.ProjectID, teamID, label, capturedAt, alert.Location)
		if err != nil {
			return err
		}
		var eventID string
		deduplicationKey := fmt.Sprintf("dji-flighthub:%d:ai:%s", instance.ID, secureRemoteKey(alert.AlertUUID))
		title := fmt.Sprintf("司空 AI 告警 · %s", label)
		err = tx.QueryRowContext(ctx, `insert into perception_events(
			id,project_id,team_id,event_rule_version_id,detection_group_id,deduplication_key,title,severity,status,
			occurrence_count,state_version,first_detected_at,last_detected_at
		) values(gen_random_uuid(),$1,$2,$3,$4,$5,$6,$7,'open',1,0,$8,$8) returning id`, instance.ProjectID, teamID,
			ruleVersionID, groupID, deduplicationKey, title, severity, capturedAt).Scan(&eventID)
		if err != nil {
			return err
		}
		issueID, err := createAlertIssue(ctx, tx, instance, eventID, runID, droneID, gatewayID, assetID, alert, severity, capturedAt)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `insert into issue_links(project_id,issue_id,link_type,target_id)
			values($1,$2,'spatial_group',$3) on conflict(issue_id,link_type,target_id) do nothing`, instance.ProjectID, issueID, strconv.FormatInt(groupID, 10)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `update connector_remote_resources set canonical_target_type='perception_event',canonical_target_id=$4,updated_at=now()
			where project_id=$1 and connector_instance_id=$2 and id=$3`, instance.ProjectID, instance.ID, canonical.ResourceID, eventID); err != nil {
			return err
		}
		return insertPerceptionCreatedEvent(ctx, tx, instance.ProjectID, teamID, eventID, issueID, capturedAt)
	}
	projection, err := loadAlertProjection(ctx, tx, instance.ProjectID, canonical)
	if err != nil {
		return err
	}
	if err := updateAlertGroup(ctx, tx, projection.GroupID, label, capturedAt, alert.Location); err != nil {
		return err
	}
	reopening := projection.Status == "resolved" || projection.Status == "dismissed"
	_, err = tx.ExecContext(ctx, `update perception_events set title=$3,severity=$4,
		status=case when status in('resolved','dismissed') then 'open' else status end,
		occurrence_count=occurrence_count+case when status in('resolved','dismissed') then 1 else 0 end,
		state_version=state_version+case when status in('resolved','dismissed') then 1 else 0 end,
		first_detected_at=least(first_detected_at,$5),last_detected_at=greatest(last_detected_at,$5),
		resolved_at=case when status in('resolved','dismissed') then null else resolved_at end,updated_at=now()
		where project_id=$1 and id=$2`, instance.ProjectID, projection.EventID, fmt.Sprintf("司空 AI 告警 · %s", label), severity, capturedAt)
	if err != nil {
		return err
	}
	labels, _ := json.Marshal(alertLabels(alert))
	_, err = tx.ExecContext(ctx, `update issues set title=$3,description=$4,priority=$5,task_run_id=coalesce($6,task_run_id),
		status=case when status='closed' then 'open' else status end,
		occurrence_count=occurrence_count+case when status='closed' then 1 else 0 end,
		state_version=state_version+case when status='closed' then 1 else 0 end,
		first_seen_at=least(first_seen_at,$7),last_seen_at=greatest(last_seen_at,$7),labels_json=$8,
		closed_at=case when status='closed' then null else closed_at end,updated_at=now()
		where project_id=$1 and id=$2`, instance.ProjectID, projection.IssueID, fmt.Sprintf("司空 AI 告警 · %s", label),
		alertText(alert.Reason, 512), alertPriority(severity), runID, capturedAt, labels)
	if err != nil {
		return err
	}
	for _, link := range []struct {
		kind string
		id   *int
	}{{"device", droneID}, {"device", gatewayID}, {"asset", assetID}} {
		if link.id != nil {
			if _, err := tx.ExecContext(ctx, `insert into issue_links(project_id,issue_id,link_type,target_id)
				values($1,$2,$3,$4) on conflict(issue_id,link_type,target_id) do nothing`, instance.ProjectID, projection.IssueID, link.kind, strconv.Itoa(*link.id)); err != nil {
				return err
			}
		}
	}
	if assetID != nil {
		if _, err := tx.ExecContext(ctx, `update assets set issue_id=$3 where project_id=$1 and id=$2`, instance.ProjectID, *assetID, projection.IssueID); err != nil {
			return err
		}
	}
	if reopening {
		metadata, _ := json.Marshal(map[string]any{"source": "dji-flighthub-openapi", "status": "open"})
		_, err = tx.ExecContext(ctx, `insert into issue_events(project_id,issue_id,event_type,metadata_json) values($1,$2,'status.changed',$3)`, instance.ProjectID, projection.IssueID, metadata)
	}
	return err
}

func resolveMissingAIAlerts(ctx context.Context, tx *sql.Tx, instance connector.Instance, seen []string, recoveredAt time.Time) error {
	seenJSON, err := json.Marshal(seen)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `select resource.id,resource.canonical_target_id,event.detection_group_id,link.issue_id
		from connector_remote_resources resource
		join perception_events event on event.project_id=resource.project_id and resource.canonical_target_type='perception_event' and resource.canonical_target_id=event.id::text
		join issue_links link on link.project_id=resource.project_id and link.link_type='perception_event' and link.target_id=event.id::text
		where resource.project_id=$1 and resource.connector_instance_id=$2 and resource.resource_kind='ai-alert' and resource.status='active'
		and not exists(select 1 from jsonb_array_elements_text($3::jsonb) seen(id) where seen.id=resource.remote_id)
		for update of resource,event`, instance.ProjectID, instance.ID, seenJSON)
	if err != nil {
		return err
	}
	type missingAlert struct {
		resourceID int64
		eventID    string
		groupID    int64
		issueID    int
	}
	missing := make([]missingAlert, 0)
	for rows.Next() {
		var item missingAlert
		if err := rows.Scan(&item.resourceID, &item.eventID, &item.groupID, &item.issueID); err != nil {
			rows.Close()
			return err
		}
		missing = append(missing, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range missing {
		if _, err := tx.ExecContext(ctx, `update connector_remote_resources set status='missing',missing_at=coalesce(missing_at,$2),updated_at=now() where id=$1`, item.resourceID, recoveredAt); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `update perception_events set status='resolved',state_version=state_version+1,resolved_at=$3,updated_at=now()
			where project_id=$1 and id=$2 and status<>'resolved'`, instance.ProjectID, item.eventID, recoveredAt)
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `update detection_groups set status='superseded',updated_at=now() where project_id=$1 and id=$2`, instance.ProjectID, item.groupID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `update issues set status='closed',closed_at=($3::timestamptz at time zone 'UTC'),last_seen_at=greatest(last_seen_at,$3::timestamptz),state_version=state_version+1,updated_at=now()
			where project_id=$1 and id=$2 and status<>'closed'`, instance.ProjectID, item.issueID, recoveredAt); err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]any{"source": "dji-flighthub-openapi", "status": "closed"})
		if _, err := tx.ExecContext(ctx, `insert into issue_events(project_id,issue_id,event_type,metadata_json) values($1,$2,'status.changed',$3)`, instance.ProjectID, item.issueID, metadata); err != nil {
			return err
		}
	}
	return nil
}

func markMissingFlightAlertAggregates(ctx context.Context, tx *sql.Tx, instance connector.Instance, seen []string, observedAt time.Time) error {
	seenJSON, err := json.Marshal(seen)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `update connector_remote_resources set status='missing',missing_at=coalesce(missing_at,$4),updated_at=now()
		where project_id=$1 and connector_instance_id=$2 and resource_kind='flight-alert' and status='active'
		and not exists(select 1 from jsonb_array_elements_text($3::jsonb) seen(id) where seen.id=connector_remote_resources.remote_id)`,
		instance.ProjectID, instance.ID, seenJSON, observedAt)
	return err
}

func (projector *SQLFlightCatalogProjector) ApplyFlightAlerts(ctx context.Context, instance connector.Instance, poll FlightAlertPoll) (returnedErr error) {
	if projector == nil || projector.db == nil || poll.ReceivedAt.IsZero() || poll.Aggregates == nil || poll.Alerts == nil {
		return errors.New("FlightHub flight alert projection is invalid")
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
	ruleVersionID, err := ensureFlightAlertRule(ctx, tx, instance.ProjectID, teamID, instance.ID)
	if err != nil {
		return err
	}
	flights := make(map[string]struct{}, len(poll.Aggregates))
	flightIDs := make([]string, 0, len(poll.Aggregates))
	for _, aggregate := range poll.Aggregates {
		if strings.TrimSpace(aggregate.FlightID) == "" || aggregate.Count < 0 || aggregate.StartTime <= 0 {
			return schemaError()
		}
		if _, duplicate := flights[aggregate.FlightID]; duplicate {
			return schemaError()
		}
		flights[aggregate.FlightID] = struct{}{}
		flightIDs = append(flightIDs, aggregate.FlightID)
		if err := upsertFlightAlertAggregate(ctx, tx, instance, teamID, aggregate); err != nil {
			return err
		}
	}
	seenAlerts := make(map[string]struct{}, len(poll.Alerts))
	alertIDs := make([]string, 0, len(poll.Alerts))
	for _, alert := range poll.Alerts {
		if _, ok := flights[alert.FlightID]; !ok {
			return errors.New("FlightHub AI alert is outside synchronized flight scope")
		}
		if _, duplicate := seenAlerts[alert.AlertUUID]; duplicate {
			return schemaError()
		}
		seenAlerts[alert.AlertUUID] = struct{}{}
		alertIDs = append(alertIDs, alert.AlertUUID)
		if err := projector.projectAIAlert(ctx, tx, instance, teamID, ruleVersionID, alert); err != nil {
			return err
		}
	}
	if poll.CompleteSnapshot {
		if err := resolveMissingAIAlerts(ctx, tx, instance, alertIDs, poll.ReceivedAt.UTC()); err != nil {
			return err
		}
		if err := markMissingFlightAlertAggregates(ctx, tx, instance, flightIDs, poll.ReceivedAt.UTC()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
