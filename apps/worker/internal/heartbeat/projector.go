package heartbeat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"aerosight/worker/internal/device"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Projection struct {
	Status string
	Reason string
}

type Signal struct {
	ProjectID               int
	TeamID                  int
	AdapterID               int64
	DeviceID                int
	SessionKey              string
	ReceivedAt              time.Time
	HeartbeatIntervalSecond int
	LinkQuality             *float64
	ReportedDegraded        bool
	RawStatusReference      string
}

type Projector struct {
	db    *sql.DB
	clock Clock
}

func NewProjector(db *sql.DB, clock Clock) *Projector {
	if clock == nil {
		clock = SystemClock{}
	}
	return &Projector{db: db, clock: clock}
}

func Evaluate(now time.Time, lastHeartbeat, closedAt *time.Time, interval time.Duration, linkQuality *float64, reportedDegraded bool) Projection {
	status := device.EvaluateStatus(now, lastHeartbeat, closedAt, interval, linkQuality, reportedDegraded, "")
	return Projection{Status: string(status.Status), Reason: status.Reason}
}

func (projector *Projector) Record(ctx context.Context, signal Signal) error {
	if signal.ProjectID <= 0 || signal.TeamID <= 0 || signal.AdapterID <= 0 || signal.DeviceID <= 0 || signal.SessionKey == "" {
		return errors.New("heartbeat signal is missing scoped device identity")
	}
	if signal.ReceivedAt.IsZero() {
		return errors.New("heartbeat signal requires receivedAt")
	}
	if signal.HeartbeatIntervalSecond == 0 {
		signal.HeartbeatIntervalSecond = 30
	}
	if signal.HeartbeatIntervalSecond < 5 || signal.HeartbeatIntervalSecond > 3600 {
		return errors.New("heartbeat interval must be between 5 and 3600 seconds")
	}
	if signal.LinkQuality != nil && (*signal.LinkQuality < 0 || *signal.LinkQuality > 1) {
		return errors.New("link quality must be between zero and one")
	}

	tx, err := projector.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	projection := Evaluate(projector.clock.Now(), &signal.ReceivedAt, nil,
		time.Duration(signal.HeartbeatIntervalSecond)*time.Second, signal.LinkQuality, signal.ReportedDegraded)
	statusProjection := device.EvaluateStatus(projector.clock.Now(), &signal.ReceivedAt, nil,
		time.Duration(signal.HeartbeatIntervalSecond)*time.Second, signal.LinkQuality,
		signal.ReportedDegraded, signal.RawStatusReference)
	var connectionID int64
	err = tx.QueryRowContext(ctx, `
		insert into device_connections (
		  project_id, team_id, adapter_id, device_id, session_key, status, link_quality,
		  status_reason, last_heartbeat_at, heartbeat_interval_seconds, status_projected_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		on conflict (adapter_id, session_key) do update set
		  status = excluded.status, link_quality = excluded.link_quality,
		  status_reason = excluded.status_reason, last_heartbeat_at = excluded.last_heartbeat_at,
		  heartbeat_interval_seconds = excluded.heartbeat_interval_seconds,
		  status_projected_at = excluded.status_projected_at
		where device_connections.project_id = excluded.project_id
		  and device_connections.device_id = excluded.device_id
		  and device_connections.closed_at is null
		  and (device_connections.last_heartbeat_at is null
		       or excluded.last_heartbeat_at > device_connections.last_heartbeat_at)
		returning id`, signal.ProjectID, signal.TeamID, signal.AdapterID, signal.DeviceID,
		signal.SessionKey, projection.Status, signal.LinkQuality, projection.Reason,
		signal.ReceivedAt, signal.HeartbeatIntervalSecond, projector.clock.Now()).Scan(&connectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("heartbeat is stale or conflicts with the bound session identity")
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		update devices set status = $3, status_reason = $4,
		  last_seen_at = greatest(coalesce(last_seen_at, $5), $5),
		  status_observed_at = greatest(coalesce(status_observed_at, $5), $5),
		  status_projected_at = $6, data_freshness = $7, raw_status_ref = $8,
		  updated_at = now()
		where id = $1 and project_id = $2`, signal.DeviceID, signal.ProjectID,
		projection.Status, projection.Reason, signal.ReceivedAt, statusProjection.ProjectedAt,
		statusProjection.Freshness, nullableRawReference(signal.RawStatusReference)); err != nil {
		return err
	}
	return tx.Commit()
}

func nullableRawReference(reference string) any {
	if reference == "" {
		return nil
	}
	return reference
}

func (projector *Projector) Sweep(ctx context.Context) error {
	now := projector.clock.Now()
	rows, err := projector.db.QueryContext(ctx, `
		select id, project_id, device_id, last_heartbeat_at, closed_at,
		       heartbeat_interval_seconds, link_quality,
		       coalesce((metadata_json->>'reportedDegraded')::boolean, false)
		from device_connections
		where closed_at is null or status <> 'offline'`)
	if err != nil {
		return err
	}
	type pending struct {
		id, projectID, deviceID int
		last, closed            sql.NullTime
		interval                int
		quality                 sql.NullFloat64
		reportedDegraded        bool
	}
	var connections []pending
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.id, &item.projectID, &item.deviceID, &item.last, &item.closed,
			&item.interval, &item.quality, &item.reportedDegraded); err != nil {
			rows.Close()
			return err
		}
		connections = append(connections, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range connections {
		var last, closed *time.Time
		var quality *float64
		if item.last.Valid {
			last = &item.last.Time
		}
		if item.closed.Valid {
			closed = &item.closed.Time
		}
		if item.quality.Valid {
			quality = &item.quality.Float64
		}
		statusProjection := device.EvaluateStatus(now, last, closed, time.Duration(item.interval)*time.Second, quality, item.reportedDegraded, "")
		projection := Projection{Status: string(statusProjection.Status), Reason: statusProjection.Reason}
		result, err := projector.db.ExecContext(ctx, `
			update device_connections set status = $2, status_reason = $3, status_projected_at = $4
			where id = $1 and (status <> $2 or status_reason is distinct from $3)`,
			item.id, projection.Status, projection.Reason, now)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			continue
		}
		if _, err := projector.db.ExecContext(ctx, `
			update devices set status = $3, status_reason = $4,
			  status_projected_at = $6, data_freshness = $7, updated_at = now()
			where id = $1 and project_id = $2
			  and not exists (
			    select 1 from device_connections newer
			    where newer.device_id = $1 and newer.project_id = $2
			      and newer.id <> $5 and newer.opened_at >
			          (select opened_at from device_connections where id = $5)
			      and newer.closed_at is null
			  )`, item.deviceID, item.projectID, projection.Status, projection.Reason, item.id,
			statusProjection.ProjectedAt, statusProjection.Freshness); err != nil {
			return err
		}
	}
	return nil
}

func (projector *Projector) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("heartbeat sweep interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := projector.Sweep(ctx); err != nil {
				return err
			}
		}
	}
}
