package telemetry

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
)

func TestPoseFieldsExistAtObservationInsertAndReplayIsIdempotent(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
insert into teams(id,name) values(1001,'inline position');
insert into projects(id,team_id,name) values(1001,1001,'inline position');
insert into device_adapters(id,project_id,team_id,name,adapter_type) values(1001,1001,1001,'position source','simulator');
insert into devices(id,project_id,adapter_id,name,type,device_type_id) values(1001,1001,1001,'position device','drone',(select id from device_types where type_key='dji.matrice3td' limit 1));
-- This would reject an INSERT followed by a separate pose UPDATE/INSERT.
alter table observations add constraint fixture_pose_atomic check (observation_type <> 'pose' or pose_spatial_quality is not null);`); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	batch := []Telemetry{
		{ProjectID: 1001, TeamID: 1001, AdapterID: 1001, DeviceID: 1001, EventID: "wgs-position", Type: "pose", CapturedAt: at, ReceivedAt: at, Payload: json.RawMessage(`{"deviceType":"drone","crs":"EPSG:4326","longitude":120,"latitude":30,"altitudeMeters":50,"orientation":{"x":0,"y":0,"z":0,"w":1},"velocityMetersPerSecond":{"x":1,"y":2,"z":3},"horizontalAccuracyMeters":0.5}`), Quality: json.RawMessage(`{}`)},
		{ProjectID: 1001, TeamID: 1001, AdapterID: 1001, DeviceID: 1001, EventID: "unknown-position", Type: "pose", CapturedAt: at.Add(time.Second), ReceivedAt: at.Add(time.Second), Payload: json.RawMessage(`{"deviceType":"drone","crs":"UNKNOWN","longitude":121,"latitude":31,"altitudeMeters":51}`), Quality: json.RawMessage(`{}`)},
	}
	ingestor := NewIngestor(db)
	if n, err := ingestor.IngestBatch(ctx, batch); err != nil || n != 2 {
		t.Fatalf("insert %d: %v", n, err)
	}
	if n, err := ingestor.IngestBatch(ctx, batch); err != nil || n != 0 {
		t.Fatalf("replay %d: %v", n, err)
	}
	for _, q := range []string{
		`select count(*)=2 from observations`,
		`select count(*)=2 from poses`,
		`select pose_device_id=device_id and pose_captured_at=captured_at and ST_Z(pose_standard_position)=50 and pose_orientation_w=1 and pose_velocity_z=3 and pose_horizontal_accuracy_m=0.5 and pose_transform_version='1' and pose_spatial_quality='usable' from observations where source_event_id='wgs-position'`,
		`select standard_geometry is null and pose_standard_position is null and ST_SRID(pose_original_position)=0 and ST_Z(pose_original_position)=51 and pose_spatial_quality='unusable' and validity='degraded' from observations where source_event_id='unknown-position'`,
	} {
		var ok bool
		if err := db.QueryRow(q).Scan(&ok); err != nil || !ok {
			t.Fatalf("%s: %v / %v", q, ok, err)
		}
	}
}
