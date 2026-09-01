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

const historicalTopologyRelation = "historical_attached"

type SQLDeviceHealthProjector struct{ db *sql.DB }

func NewSQLDeviceHealthProjector(db *sql.DB) *SQLDeviceHealthProjector {
	return &SQLDeviceHealthProjector{db: db}
}

func (projector *SQLDeviceHealthProjector) Apply(ctx context.Context, instance connector.Instance, poll HealthPoll) error {
	if projector == nil || projector.db == nil || instance.ID <= 0 || instance.ProjectID <= 0 || poll.ReceivedAt.IsZero() {
		return errors.New("FlightHub health projection scope is invalid")
	}
	bySerial := make(map[string]connector.ManagedConnectorDevice, len(poll.Devices))
	for _, item := range poll.Devices {
		if item.DeviceID <= 0 || item.TeamID <= 0 || strings.TrimSpace(item.Serial) == "" {
			return errors.New("FlightHub health device scope is invalid")
		}
		bySerial[item.Serial] = item
	}
	tx, err := projector.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var adapterID int64
	err = tx.QueryRowContext(ctx, `select id from device_adapters where id=$1 and project_id=$2
		and status in('connecting','connected','degraded')
		and ($3='' or (lease_owner=$3 and connection_epoch=$4 and lease_expires_at>=now())) for share`,
		instance.ID, instance.ProjectID, instance.LeaseOwner, instance.LeaseEpoch).Scan(&adapterID)
	if errors.Is(err, sql.ErrNoRows) {
		return connector.ErrConnectorDisabled
	}
	if err != nil {
		return err
	}
	for _, deviceHMS := range poll.HMS {
		device, ok := bySerial[deviceHMS.SN]
		if !ok {
			return errors.New("FlightHub HMS device is outside managed scope")
		}
		for _, alert := range deviceHMS.Alerts.List {
			if err := projectHMSAlert(ctx, tx, instance, device, alert, poll.ReceivedAt); err != nil {
				return err
			}
		}
	}
	if err := projectHistoricalTopologies(ctx, tx, instance, bySerial, poll.Topologies, poll.ReceivedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func projectHMSAlert(ctx context.Context, tx *sql.Tx, instance connector.Instance, device connector.ManagedConnectorDevice, alert HMSAlert, receivedAt time.Time) error {
	identity := alert.StatusKey
	if strings.TrimSpace(identity) == "" {
		identity = alert.ID
	}
	businessKey := strconv.Itoa(device.DeviceID) + ":" + secureRemoteKey(identity)
	scopeKey := fmt.Sprintf("dji-flighthub:%d:hms", instance.ID)
	lockKey := scopeKey + ":" + businessKey
	if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", lockKey); err != nil {
		return err
	}
	firstSeen := vendorMilliseconds(alert.BeginTime, receivedAt)
	lastSeen := vendorMilliseconds(alert.DeviceDataUpdateTime, receivedAt)
	closedAt := vendorMilliseconds(alert.EndTime, lastSeen)
	closed := alert.Status == 2
	status := "open"
	if closed {
		status = "closed"
	}
	priority := hmsPriority(alert)
	title := fmt.Sprintf("司空设备 HMS · %s", boundedDiagnostic(alert.Code, 96))
	description := fmt.Sprintf("模块 %s；级别 %s", boundedDiagnostic(alert.Module, 64), boundedDiagnostic(alert.Level, 32))
	labels, _ := json.Marshal([]string{"dji-flighthub", "hms", boundedDiagnostic(alert.DomainType, 64)})
	var issueID int
	var previousStatus string
	err := tx.QueryRowContext(ctx, `select id,status from issues
		where project_id=$1 and task_version_id is null and condition_scope_key=$2 and business_object_key=$3 for update`,
		instance.ProjectID, scopeKey, businessKey).Scan(&issueID, &previousStatus)
	created := false
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", fmt.Sprintf("issue-number:%d", instance.ProjectID)); err != nil {
			return err
		}
		var number int
		if err := tx.QueryRowContext(ctx, `select coalesce(max(number),0)+1 from issues where project_id=$1`, instance.ProjectID).Scan(&number); err != nil {
			return err
		}
		var closedValue any
		if closed {
			closedValue = closedAt
		}
		err = tx.QueryRowContext(ctx, `insert into issues(
			project_id,number,title,description,source_type,status,priority,condition_scope_key,business_object_key,
			occurrence_count,first_seen_at,last_seen_at,labels_json,closed_at
		) values($1,$2,$3,$4,'dji-flighthub-hms',$5,$6,$7,$8,1,$9,$10,$11,$12) returning id`,
			instance.ProjectID, number, title, description, status, priority, scopeKey, businessKey,
			firstSeen, lastSeen, labels, closedValue).Scan(&issueID)
		created = true
	} else if err == nil {
		var closedValue any
		if closed {
			closedValue = closedAt
		}
		_, err = tx.ExecContext(ctx, `update issues set title=$2,description=$3,status=$4,priority=$5,
			first_seen_at=least(first_seen_at,$6),last_seen_at=greatest(last_seen_at,$7),labels_json=$8,
			closed_at=$9,state_version=state_version+case when status<>$4 then 1 else 0 end,updated_at=$7
			where id=$1`, issueID, title, description, status, priority, firstSeen, lastSeen, labels, closedValue)
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `insert into issue_links(project_id,issue_id,link_type,target_id)
		values($1,$2,'device',$3) on conflict(issue_id,link_type,target_id) do nothing`, instance.ProjectID, issueID, strconv.Itoa(device.DeviceID)); err != nil {
		return err
	}
	if created || previousStatus != status {
		eventType := "issue.created"
		if !created {
			eventType = "status.changed"
		}
		metadata, _ := json.Marshal(map[string]any{"source": "dji-flighthub-openapi", "status": status, "code": boundedDiagnostic(alert.Code, 96)})
		if _, err := tx.ExecContext(ctx, `insert into issue_events(project_id,issue_id,event_type,metadata_json) values($1,$2,$3,$4)`, instance.ProjectID, issueID, eventType, metadata); err != nil {
			return err
		}
	}
	for _, child := range alert.SubAlerts {
		if err := projectHMSAlert(ctx, tx, instance, device, child, receivedAt); err != nil {
			return err
		}
	}
	return nil
}

func hmsPriority(alert HMSAlert) string {
	if alert.Imminent {
		return "critical"
	}
	switch strings.ToLower(alert.Level) {
	case "warning":
		return "high"
	case "reminder":
		return "medium"
	default:
		return "low"
	}
}

func boundedDiagnostic(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func vendorMilliseconds(value int64, fallback time.Time) time.Time {
	if value <= 0 {
		return fallback.UTC()
	}
	return time.UnixMilli(value).UTC()
}

type topologyPair struct {
	fromID      int
	toID        int
	teamID      int
	topologyKey string
}

func projectHistoricalTopologies(ctx context.Context, tx *sql.Tx, instance connector.Instance, bySerial map[string]connector.ManagedConnectorDevice, topologies []HistoricalTopology, observedAt time.Time) error {
	pairs := make(map[string]topologyPair)
	for _, topology := range topologies {
		if topology.Host == nil {
			continue
		}
		host, ok := bySerial[topology.Host.SN]
		if !ok {
			return errors.New("FlightHub topology host is outside managed scope")
		}
		for _, parent := range topology.Parents {
			parentDevice, ok := bySerial[parent.SN]
			if !ok {
				return errors.New("FlightHub topology parent is outside managed scope")
			}
			key := fmt.Sprintf("%d/%d", parentDevice.DeviceID, host.DeviceID)
			if parentDevice.TeamID != host.TeamID {
				return errors.New("FlightHub topology devices cross team scope")
			}
			pairs[key] = topologyPair{fromID: parentDevice.DeviceID, toID: host.DeviceID, teamID: parentDevice.TeamID, topologyKey: secureRemoteKey(topology.Index)}
		}
	}
	rows, err := tx.QueryContext(ctx, `select id,from_device_id,to_device_id from device_relationships
		where project_id=$1 and relation_type=$2 and source_type='driver' and valid_until is null
		  and metadata_json->>'connectorInstanceId'=$3`, instance.ProjectID, historicalTopologyRelation, strconv.FormatInt(instance.ID, 10))
	if err != nil {
		return err
	}
	active := make(map[string]int64)
	for rows.Next() {
		var id int64
		var fromID, toID int
		if err := rows.Scan(&id, &fromID, &toID); err != nil {
			rows.Close()
			return err
		}
		active[fmt.Sprintf("%d/%d", fromID, toID)] = id
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for key, id := range active {
		if _, present := pairs[key]; present {
			continue
		}
		if _, err := tx.ExecContext(ctx, `update device_relationships set valid_until=$2
			where id=$1 and valid_until is null and valid_from<$2`, id, observedAt); err != nil {
			return err
		}
	}
	for key, pair := range pairs {
		if _, present := active[key]; present {
			continue
		}
		metadata, _ := json.Marshal(map[string]any{
			"source": "dji-flighthub-openapi", "connectorInstanceId": instance.ID,
			"topologyKey": pair.topologyKey, "schemaVersion": "dji-flighthub-topology/v1",
		})
		if _, err := tx.ExecContext(ctx, `insert into device_relationships(
			project_id,team_id,from_device_id,to_device_id,relation_type,source_type,valid_from,metadata_json
		) values($1,$2,$3,$4,$5,'driver',$6,$7)`, instance.ProjectID, pair.teamID, pair.fromID, pair.toID, historicalTopologyRelation, observedAt, metadata); err != nil {
			return err
		}
	}
	return nil
}
