package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSQLDeviceStreamChannelProjectorCreatesIdempotentDockChannel(t *testing.T) {
	databaseURL := os.Getenv("AEROSIGHT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AEROSIGHT_TEST_DATABASE_URL is not configured")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	accountFingerprint := strings.Repeat("a", 64)
	var teamID, projectID, foreignProjectID, dockID, foreignDockID int
	var adapterID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("flighthub-stream-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-stream-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-stream-foreign-%d", suffix)).Scan(&foreignProjectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,discovery_scope_json
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected',jsonb_build_object('accountFingerprint',$5::text)) returning id`,
		projectID, teamID, fmt.Sprintf("flighthub-stream-%d", suffix), definitionID, accountFingerprint).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	createDock := func(project int, name string, adapter any) int {
		t.Helper()
		var id int
		if err := database.QueryRowContext(ctx, `insert into devices(
			project_id,adapter_id,device_type_id,name,type,status,device_model,firmware_version
		) select $1,$2,id,$3,'dock','online','3-2-0','01.00.0900'
			from device_types where type_key='dji.dock2' and status='active'
			order by version desc limit 1 returning id`, project, adapter, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	dockID = createDock(projectID, "stream dock", adapterID)
	foreignDockID = createDock(foreignProjectID, "foreign stream dock", nil)

	snapshot := DeviceStateSnapshot{
		SN: "DOCK_REDACTED", Model: DeviceModel{Key: "3-2-0", Class: "airport"},
		State: map[string]json.RawMessage{"live_status": json.RawMessage(`0`)},
	}
	poll := DeviceStatePoll{
		Device:   connector.ManagedConnectorDevice{DeviceID: dockID, TeamID: teamID, Serial: snapshot.SN},
		Snapshot: snapshot, Mapped: MapDeviceState(snapshot), ReceivedAt: time.Now().UTC(),
	}
	projector := NewSQLDeviceHealthProjector(database)
	instance := connector.Instance{ID: adapterID, ProjectID: projectID}
	if err := projector.ApplyDeviceStreamChannels(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	if err := projector.ApplyDeviceStreamChannels(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	var channelCount, capabilityCount int
	var channelKey, availability, stableID, sourceKey, sourceID string
	if err := database.QueryRowContext(ctx, `select count(*),max(channel_key),max(availability),max(stable_channel_id),
		max(source_json->>'connectorKey'),max(source_json->>'connectorInstanceId')
		from device_stream_channels where project_id=$1 and device_id=$2`, projectID, dockID).Scan(
		&channelCount, &channelKey, &availability, &stableID, &sourceKey, &sourceID,
	); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from device_capabilities
		where project_id=$1 and device_id=$2 and capability_code=any($3::text[])`, projectID, dockID,
		[]string{flightHubVideoReadCapability, flightHubVideoControlCapability}).Scan(&capabilityCount); err != nil {
		t.Fatal(err)
	}
	if channelCount != 1 || capabilityCount != 2 || channelKey != "165-0-7" || availability != "available" ||
		sourceKey != ConnectorKey || sourceID != fmt.Sprint(adapterID) || strings.Contains(stableID, snapshot.SN) {
		t.Fatalf("dock channel projection count=%d capability=%d key=%s availability=%s source=%s/%s stableHasSN=%v",
			channelCount, capabilityCount, channelKey, availability, sourceKey, sourceID, strings.Contains(stableID, snapshot.SN))
	}
	assertControlAvailability := func(wantAvailability, wantReason string) {
		t.Helper()
		var gotAvailability string
		var gotReason sql.NullString
		if err := database.QueryRowContext(ctx, `select availability,availability_reason from device_capabilities
			where project_id=$1 and device_id=$2 and capability_code=$3`, projectID, dockID,
			flightHubVideoControlCapability).Scan(&gotAvailability, &gotReason); err != nil {
			t.Fatal(err)
		}
		if gotAvailability != wantAvailability || gotReason.String != wantReason || gotReason.Valid != (wantReason != "") {
			t.Fatalf("live control availability=%s/%v want=%s/%s", gotAvailability, gotReason, wantAvailability, wantReason)
		}
	}
	assertControlAvailability("unavailable", flightHubLiveActionDisabledReason)

	if _, err := database.ExecContext(ctx, `insert into project_feature_flags(project_id,flighthub_action_flags_json)
		values($1,'{"live.control":true}'::jsonb)
		on conflict(project_id) do update set flighthub_action_flags_json=excluded.flighthub_action_flags_json`, projectID); err != nil {
		t.Fatal(err)
	}
	if err := projector.ApplyDeviceStreamChannels(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	assertControlAvailability("unavailable", flightHubLiveFieldAcceptanceRequiredReason)

	if _, err := database.ExecContext(ctx, `insert into connector_capability_snapshots(
		project_id,team_id,connector_instance_id,capability_code,status,evidence_level,region,deployment,
		account_fingerprint,device_model,firmware_version,verified_at,expires_at
	) values($1,$2,$3,'live.control','supported','field-write','cn','cn-public-cloud',$4,'3-2-0','01.00.0900',now(),now()+interval '1 hour')`,
		projectID, teamID, adapterID, accountFingerprint); err != nil {
		t.Fatal(err)
	}
	if err := projector.ApplyDeviceStreamChannels(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	assertControlAvailability("available", "")

	delete(snapshot.State, "live_status")
	poll.Mapped = MapDeviceState(snapshot)
	if err := projector.ApplyDeviceStreamChannels(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	var reason string
	if err := database.QueryRowContext(ctx, `select availability,availability_reason from device_stream_channels
		where project_id=$1 and device_id=$2 and channel_key='165-0-7'`, projectID, dockID).Scan(&availability, &reason); err != nil {
		t.Fatal(err)
	}
	if availability != "degraded" || reason != "live_status_unavailable" {
		t.Fatalf("missing live state did not degrade channel: %s/%s", availability, reason)
	}

	foreignPoll := poll
	foreignPoll.Device.DeviceID = foreignDockID
	if err := projector.ApplyDeviceStreamChannels(ctx, instance, foreignPoll); err == nil {
		t.Fatal("cross-project device unexpectedly accepted")
	}
	var foreignChannels int
	if err := database.QueryRowContext(ctx, `select count(*) from device_stream_channels where project_id=$1`, foreignProjectID).Scan(&foreignChannels); err != nil {
		t.Fatal(err)
	}
	if foreignChannels != 0 {
		t.Fatalf("cross-project stream channels=%d", foreignChannels)
	}
}

func TestSQLDeviceStreamChannelProjectorRejectsMalformedCameraIndexBeforeWrite(t *testing.T) {
	projector := &SQLDeviceHealthProjector{db: &sql.DB{}}
	poll := DeviceStatePoll{
		Device:   connector.ManagedConnectorDevice{DeviceID: 1, TeamID: 1, Serial: "DOCK_REDACTED"},
		Snapshot: DeviceStateSnapshot{SN: "DOCK_REDACTED"}, ReceivedAt: time.Now().UTC(),
		Mapped: MappedDeviceState{StreamChannels: []StreamChannelState{{
			CameraIndex: "../camera", DisplayName: "bad", Availability: "available",
		}}},
	}
	if err := projector.ApplyDeviceStreamChannels(context.Background(), connector.Instance{ID: 1, ProjectID: 1}, poll); err == nil || !strings.Contains(err.Error(), "projection is invalid") {
		t.Fatalf("malformed camera index error=%v", err)
	}
}
