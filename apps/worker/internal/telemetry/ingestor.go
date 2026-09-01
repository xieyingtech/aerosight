package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"aerosight/worker/internal/adapter"
)

var partitionNamePattern = regexp.MustCompile(`^device_telemetry_\d{6}$`)

type Telemetry struct {
	ProjectID            int
	TeamID               int
	AdapterID            int64
	DeviceID             int
	EventID              string
	Type                 string
	Sequence             *int64
	CapturedAt           time.Time
	ReceivedAt           time.Time
	Payload              json.RawMessage
	Quality              json.RawMessage
	RequireActiveAdapter bool
	AdapterLeaseOwner    string
	AdapterLeaseEpoch    int64
}

type Ingestor struct {
	db        *sql.DB
	clockSkew time.Duration
	lateAfter time.Duration
}

func NewIngestor(db *sql.DB) *Ingestor {
	return &Ingestor{db: db, clockSkew: 10 * time.Minute, lateAfter: 2 * time.Minute}
}

func (ingestor *Ingestor) IngestBatch(ctx context.Context, batch []Telemetry) (int, error) {
	months := map[time.Time]bool{}
	for _, item := range batch {
		if err := item.Validate(); err != nil {
			return 0, err
		}
		month := time.Date(item.CapturedAt.Year(), item.CapturedAt.Month(), 1, 0, 0, 0, 0, time.UTC)
		months[month] = true
	}
	for month := range months {
		if err := ingestor.ensurePartition(ctx, month); err != nil {
			return 0, err
		}
	}

	tx, err := ingestor.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	inserted := 0
	validatedAdapters := map[int64]bool{}
	for _, item := range batch {
		if item.RequireActiveAdapter && !validatedAdapters[item.AdapterID] {
			var adapterID int64
			err := tx.QueryRowContext(ctx, `select id from device_adapters where id=$1 and project_id=$2
				and status in('connecting','connected','degraded')
				and ($3='' or (lease_owner=$3 and connection_epoch=$4 and lease_expires_at>=now())) for share`,
				item.AdapterID, item.ProjectID, item.AdapterLeaseOwner, item.AdapterLeaseEpoch).Scan(&adapterID)
			if errors.Is(err, sql.ErrNoRows) {
				return 0, errors.New("connector adapter is disabled or its lease is no longer active")
			}
			if err != nil {
				return 0, err
			}
			validatedAdapters[item.AdapterID] = true
		}
		result, err := tx.ExecContext(ctx, `
			insert into telemetry_event_dedup (adapter_id, event_id, project_id, captured_at, received_at)
			values ($1, $2, $3, $4, $5)
			on conflict (adapter_id, event_id) do nothing`,
			item.AdapterID, item.EventID, item.ProjectID, item.CapturedAt, item.ReceivedAt)
		if err != nil {
			return 0, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		if rows == 0 {
			continue
		}

		if _, err := tx.ExecContext(ctx, `
			insert into device_telemetry (
			  project_id, team_id, adapter_id, device_id, event_id, telemetry_type,
			  sequence_number, captured_at, received_at, payload_json, quality_json
			) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			item.ProjectID, item.TeamID, item.AdapterID, item.DeviceID, item.EventID,
			item.Type, item.Sequence, item.CapturedAt, item.ReceivedAt, item.Payload, item.Quality); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
			insert into device_latest_telemetry (
			  device_id, project_id, adapter_id, event_id, telemetry_type, sequence_number,
			  captured_at, received_at, payload_json, quality_json
			) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			on conflict (device_id) do update set
			  adapter_id = excluded.adapter_id, event_id = excluded.event_id,
			  telemetry_type = excluded.telemetry_type, sequence_number = excluded.sequence_number,
			  captured_at = excluded.captured_at, received_at = excluded.received_at,
			  payload_json = excluded.payload_json, quality_json = excluded.quality_json, updated_at = now()
			where excluded.captured_at > device_latest_telemetry.captured_at
			   or (excluded.captured_at = device_latest_telemetry.captured_at
			       and coalesce(excluded.sequence_number, -1) > coalesce(device_latest_telemetry.sequence_number, -1))`,
			item.DeviceID, item.ProjectID, item.AdapterID, item.EventID, item.Type,
			item.Sequence, item.CapturedAt, item.ReceivedAt, item.Payload, item.Quality); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
			update devices set last_seen_at = $3, updated_at = now()
			where id = $1 and project_id = $2 and (last_seen_at is null or last_seen_at < $3)`,
			item.DeviceID, item.ProjectID, item.CapturedAt); err != nil {
			return 0, err
		}
		if item.Type == "pose" || item.Type == "telemetry.pose" {
			if err := ingestor.insertPoseObservation(ctx, tx, item); err != nil {
				return 0, err
			}
		}
		inserted++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

func (ingestor *Ingestor) insertPoseObservation(ctx context.Context, tx *sql.Tx, item Telemetry) error {
	var pose adapter.Pose
	if err := json.Unmarshal(item.Payload, &pose); err != nil {
		return fmt.Errorf("decode pose telemetry: %w", err)
	}
	if err := pose.Validate(); err != nil {
		return err
	}
	timeQuality, validity := classifyTiming(item.CapturedAt, item.ReceivedAt, ingestor.clockSkew, ingestor.lateAfter)
	supportedCRS, spatialQuality := classifySpatialReference(pose.CRS)
	if !supportedCRS {
		if validity == "valid" {
			validity = "degraded"
		}
	}

	var crsID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		select id from coordinate_references
		where project_id = $1 and code = $2
		order by is_project_standard desc, id limit 1`, item.ProjectID, pose.CRS).Scan(&crsID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, insertErr := tx.ExecContext(ctx, `
			insert into coordinate_references (
			  project_id, team_id, code, name, authority, vertical_datum
			) values ($1, $2, $3, $3, $4, $5)
			on conflict (project_id, code, transform_version) do nothing`,
			item.ProjectID, item.TeamID, pose.CRS, authorityForCRS(pose.CRS), pose.VerticalDatum)
		if insertErr != nil {
			return insertErr
		}
		if err := tx.QueryRowContext(ctx, `
			select id from coordinate_references
			where project_id = $1 and code = $2 order by id limit 1`,
			item.ProjectID, pose.CRS).Scan(&crsID); err != nil {
			return err
		}
	}

	properties, _ := json.Marshal(map[string]any{
		"deviceType":        pose.DeviceType,
		"rawLongitude":      pose.Longitude,
		"rawLatitude":       pose.Latitude,
		"rawAltitudeMeters": pose.AltitudeMeters,
	})
	quality, _ := json.Marshal(map[string]any{
		"timeQuality":              timeQuality,
		"spatialQuality":           spatialQuality,
		"horizontalAccuracyMeters": pose.HorizontalAccuracyMeters,
		"verticalAccuracyMeters":   pose.VerticalAccuracyMeters,
		"attitudeAccuracyDegrees":  pose.AttitudeAccuracyDegrees,
	})
	var observationID int64
	var observationQuery string
	var observationArgs []any
	altitude := valueOrZero(pose.AltitudeMeters)
	if supportedCRS {
		observationQuery = `
			insert into observations (
			  project_id, team_id, adapter_id, device_id, observation_type, source_event_id,
			  captured_at, received_at, time_quality, original_crs_id,
			  original_geometry, standard_geometry, properties_json, quality_json, validity
			) values (
			  $1, $2, $3, $4, 'pose', $5, $6, $7, $8, $9,
			  ST_SetSRID(ST_MakePoint($10, $11, $12), 4326),
			  ST_SetSRID(ST_MakePoint($10, $11, $12), 4326), $13, $14, $15
			) returning id`
		observationArgs = []any{
			item.ProjectID, item.TeamID, item.AdapterID, item.DeviceID, item.EventID,
			item.CapturedAt, item.ReceivedAt, timeQuality, nullableInt64(crsID),
			pose.Longitude, pose.Latitude, altitude, properties, quality, validity,
		}
	} else {
		observationQuery = `
			insert into observations (
			  project_id, team_id, adapter_id, device_id, observation_type, source_event_id,
			  captured_at, received_at, time_quality, original_crs_id,
			  original_geometry, properties_json, quality_json, validity
			) values ($1, $2, $3, $4, 'pose', $5, $6, $7, $8, $9,
			  ST_MakePoint($10, $11, $12), $13, $14, $15)
			returning id`
		observationArgs = []any{
			item.ProjectID, item.TeamID, item.AdapterID, item.DeviceID, item.EventID,
			item.CapturedAt, item.ReceivedAt, timeQuality, nullableInt64(crsID),
			pose.Longitude, pose.Latitude, altitude, properties, quality, validity,
		}
	}
	if err := tx.QueryRowContext(ctx, observationQuery, observationArgs...).Scan(&observationID); err != nil {
		return err
	}

	if supportedCRS {
		transformVersion := pose.TransformVersion
		if transformVersion == "" {
			transformVersion = "1"
		}
		_, err := tx.ExecContext(ctx, `
			insert into poses (
			  observation_id, project_id, device_id, captured_at,
			  standard_position, original_position,
			  orientation_x, orientation_y, orientation_z, orientation_w,
			  velocity_x, velocity_y, velocity_z,
			  horizontal_accuracy_m, vertical_accuracy_m, attitude_accuracy_deg,
			  vertical_datum, transform_version, spatial_quality
			) values (
			  $1, $2, $3, $4,
			  ST_SetSRID(ST_MakePoint($5, $6, $7), 4326),
			  ST_SetSRID(ST_MakePoint($5, $6, $7), 4326),
			  $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
			)`, observationID, item.ProjectID, item.DeviceID, item.CapturedAt,
			pose.Longitude, pose.Latitude, altitude,
			quaternionValue(pose.Orientation, "x"), quaternionValue(pose.Orientation, "y"),
			quaternionValue(pose.Orientation, "z"), quaternionValue(pose.Orientation, "w"),
			vectorValue(pose.VelocityMetersPerSecond, "x"), vectorValue(pose.VelocityMetersPerSecond, "y"),
			vectorValue(pose.VelocityMetersPerSecond, "z"), pose.HorizontalAccuracyMeters,
			pose.VerticalAccuracyMeters, pose.AttitudeAccuracyDegrees, pose.VerticalDatum, transformVersion, spatialQuality)
		return err
	}
	transformVersion := pose.TransformVersion
	if transformVersion == "" {
		transformVersion = "1"
	}
	_, err := tx.ExecContext(ctx, `
		insert into poses (
		  observation_id, project_id, device_id, captured_at, original_position,
		  orientation_x, orientation_y, orientation_z, orientation_w,
		  velocity_x, velocity_y, velocity_z,
		  horizontal_accuracy_m, vertical_accuracy_m, attitude_accuracy_deg,
		  vertical_datum, transform_version, spatial_quality
		) values ($1, $2, $3, $4, ST_MakePoint($5, $6, $7),
		  $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, 'unusable')`,
		observationID, item.ProjectID, item.DeviceID, item.CapturedAt,
		pose.Longitude, pose.Latitude, altitude,
		quaternionValue(pose.Orientation, "x"), quaternionValue(pose.Orientation, "y"),
		quaternionValue(pose.Orientation, "z"), quaternionValue(pose.Orientation, "w"),
		vectorValue(pose.VelocityMetersPerSecond, "x"), vectorValue(pose.VelocityMetersPerSecond, "y"),
		vectorValue(pose.VelocityMetersPerSecond, "z"), pose.HorizontalAccuracyMeters,
		pose.VerticalAccuracyMeters, pose.AttitudeAccuracyDegrees, pose.VerticalDatum, transformVersion)
	return err
}

func classifyTiming(capturedAt, receivedAt time.Time, clockSkew, lateAfter time.Duration) (string, string) {
	delta := receivedAt.Sub(capturedAt)
	absolute := delta
	if absolute < 0 {
		absolute = -absolute
	}
	timeQuality := "trusted"
	if absolute > clockSkew {
		timeQuality = "uncertain"
	}
	if delta > lateAfter {
		return timeQuality, "late"
	}
	if delta < -clockSkew {
		return "uncertain", "degraded"
	}
	return timeQuality, "valid"
}

func classifySpatialReference(crs string) (bool, string) {
	if crs == "EPSG:4326" || crs == "urn:ogc:def:crs:EPSG::4326" {
		return true, "usable"
	}
	return false, "unusable"
}

func authorityForCRS(crs string) any {
	if crs == "EPSG:4326" || crs == "urn:ogc:def:crs:EPSG::4326" {
		return "EPSG"
	}
	return nil
}

func valueOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func nullableInt64(value sql.NullInt64) any {
	if value.Valid {
		return value.Int64
	}
	return nil
}

func quaternionValue(value *adapter.Quaternion, axis string) any {
	if value == nil {
		return nil
	}
	switch axis {
	case "x":
		return value.X
	case "y":
		return value.Y
	case "z":
		return value.Z
	default:
		return value.W
	}
}

func vectorValue(value *adapter.Vector3, axis string) any {
	if value == nil {
		return nil
	}
	switch axis {
	case "x":
		return value.X
	case "y":
		return value.Y
	default:
		return value.Z
	}
}

func (ingestor *Ingestor) ensurePartition(ctx context.Context, month time.Time) error {
	month = month.UTC()
	if month.Day() != 1 || month.Hour() != 0 || month.Minute() != 0 || month.Second() != 0 {
		return errors.New("telemetry partition boundary must be the start of a UTC month")
	}
	name := fmt.Sprintf("device_telemetry_%04d%02d", month.Year(), month.Month())
	if !partitionNamePattern.MatchString(name) {
		return errors.New("invalid telemetry partition name")
	}
	next := month.AddDate(0, 1, 0)
	statement := fmt.Sprintf(
		"create table if not exists %s partition of device_telemetry for values from ('%s') to ('%s')",
		name, month.Format(time.RFC3339), next.Format(time.RFC3339),
	)
	_, err := ingestor.db.ExecContext(ctx, statement)
	return err
}

func (item Telemetry) Validate() error {
	if item.ProjectID <= 0 || item.TeamID <= 0 || item.AdapterID <= 0 || item.DeviceID <= 0 {
		return errors.New("telemetry scope is required")
	}
	if item.EventID == "" || item.Type == "" || item.CapturedAt.IsZero() {
		return errors.New("telemetry eventId, type, and capturedAt are required")
	}
	if item.ReceivedAt.IsZero() {
		return errors.New("telemetry receivedAt is required")
	}
	if item.Sequence != nil && *item.Sequence < 0 {
		return errors.New("telemetry sequence must be non-negative")
	}
	if !json.Valid(item.Payload) || !json.Valid(item.Quality) {
		return errors.New("telemetry payload and quality must be valid JSON")
	}
	return nil
}
