package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"

	"aerosight/server/internal/migrations"
	"aerosight/server/internal/mission"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/testdb"
)

func TestObserveAssetsSealsReadableVersionsWithoutChangingProvenance(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	id := func(query string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := db.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	uid := id("insert into users(name,email,password) values('observe','observe@example.com','unused') returning id")
	team := id("insert into teams(name) values('observe') returning id")
	if _, err := db.Exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid); err != nil {
		t.Fatal(err)
	}
	project := id("insert into projects(team_id,name) values($1,'observe') returning id", team)
	other := id("insert into projects(team_id,name) values($1,'other') returning id", team)
	sourceTask := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'source','manual','source') returning id", project, team)
	sourceRun := id("insert into task_runs(project_id,team_id,task_id,trigger_source,status) values($1,$2,$3,'manual','succeeded') returning id", project, team, sourceTask)
	adapter := id("insert into device_adapters(project_id,team_id,name,adapter_type,protocol_version,status) values($1,$2,'observe','dji-flighthub2','2','connected') returning id", project, team)
	asset := id("insert into assets(project_id,team_id,task_run_id,kind,storage_key,logical_key) values($1,$2,$3,'image','photo.jpg','photo') returning id", project, team, sourceRun)
	foreign := id("insert into assets(project_id,team_id,kind,storage_key,logical_key) values($1,$2,'image','private.jpg','private') returning id", other, team)
	bad := id("insert into assets(project_id,team_id,kind,storage_key,logical_key,checksum_sha256) values($1,$2,'image','bad.jpg','bad',$3) returning id", project, team, strings.Repeat("0", 64))
	for n, tc := range []struct {
		name                 string
		ids                  []int64
		limit                int
		unreadable, canceled bool
		success              bool
	}{
		{"flight callback", []int64{asset}, 64, false, false, true}, {"first", []int64{asset}, 64, false, false, true}, {"same photo another run", []int64{asset}, 64, false, false, true},
		{"foreign", []int64{foreign}, 64, false, false, false}, {"duplicate", []int64{asset, asset}, 64, false, false, false},
		{"too many", []int64{asset, bad}, 1, false, false, false}, {"unreadable", []int64{asset}, 64, true, false, false},
		{"checksum", []int64{bad}, 64, false, false, false}, {"canceled", []int64{asset}, 64, false, true, false}, {"simulator associated", []int64{asset}, 64, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "simulator associated" {
				sim := id("insert into device_adapters(project_id,team_id,name,adapter_type) values($1,$2,'sample simulator','simulator') returning id", project, team)
				device := id("insert into devices(project_id,name,type,adapter_id,device_type_id) select $1,'sample','drone',$2,id from device_types where type_key='legacy.device' returning id", project, sim)
				if _, err := db.Exec("update assets set device_id=$2 where id=$1", asset, device); err != nil {
					t.Fatal(err)
				}
			}
			task := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,$3,'manual','typed-task-v2') returning id", project, team, fmt.Sprintf("analysis-%d", n))
			version := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", project, team, task)
			mode := "assets"
			if tc.name == "flight callback" {
				mode = "existing-flight"
			}
			params, _ := json.Marshal(map[string]any{"mode": mode, "assetIds": tc.ids, "maxImages": tc.limit})
			definitionStep := id(`insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses,parameters_json,input_schema_json,output_schema_json)
   values($1,$2,$3,1,'observe','observe','inspection.observe','inspection.observe',$4,'{"type":"object"}','{"type":"object","properties":{"observationId":{"type":"string"}},"required":["observationId"],"additionalProperties":false}') returning id`, project, team, version, params)
			status := "running"
			if tc.canceled {
				status = "canceled"
			}
			run := id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status,created_by_user_id) values($1,$2,$3,$4,'manual',$5,$6) returning id", project, team, task, version, status, uid)
			runStep := id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,1,'running') returning id", project, team, run, definitionStep)
			reads := 0
			processor := NewObserveProcessor(func(context.Context, string) ([]byte, error) {
				reads++
				if tc.unreadable {
					return nil, fmt.Errorf("unreadable")
				}
				return []byte("authorized-image-fixture"), nil
			})
			if tc.name == "flight callback" {
				processor.flight = func(_ context.Context, _ *sql.Tx, step mission.PreparedStep, _ ObserveInput) (Observation, error) {
					reads++
					scope := Scope{ProjectID: step.ProjectID, TeamID: step.TeamID}
					flight := &FlightRef{Scope: scope, ConnectorID: adapter, FlightUUID: "fixture-flight", ProjectedRunID: &sourceRun}
					return Observation{ID: uuid.NewString(), ContractVersion: ContractVersion, Run: RunRef{Scope: scope, RunID: int64(step.RunID), StepID: step.StepID}, Mode: ExistingFlight, Flight: flight, Completeness: Complete, ScopeDescription: "fixture media", ObservedFrom: time.Now().UTC(), ObservedTo: time.Now().UTC(), Assets: []AssetRef{{Scope: scope, AssetID: asset, Version: 1, ChecksumSHA256: strings.Repeat("a", 64), SourceRunID: &sourceRun, Flight: flight}}}, nil
				}
			}
			handler := mission.WithTaskStepFailurePolicy(processor.Handler)
			raw, _ := json.Marshal(map[string]any{"taskRunId": run, "taskRunStepId": runStep})
			event := outbox.Event{ProjectID: int(project), TeamID: int(team), Payload: raw, Attempts: 1, MaxAttempts: 1}
			for range 2 {
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err = handler(ctx, tx, event); err != nil {
					tx.Rollback()
					t.Fatal(err)
				}
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
			}
			var count int
			if err := db.QueryRow("select count(*) from inspection_observations where task_run_id=$1", run).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if !tc.success {
				if count != 0 {
					t.Fatal("failed/canceled selection sealed observation")
				}
				return
			}
			if count != 1 || reads != 1 {
				t.Fatalf("observation duplicate or reread: count %d reads %d", count, reads)
			}
			var manifest []byte
			var sealed bool
			if err := db.QueryRow("select manifest_json,sealed_at is not null from inspection_observations where task_run_id=$1", run).Scan(&manifest, &sealed); err != nil {
				t.Fatal(err)
			}
			var observed Observation
			if err := json.Unmarshal(manifest, &observed); err != nil {
				t.Fatal(err)
			}
			if tc.name != "flight callback" {
				wantSource := "unverified"
				if tc.name == "simulator associated" {
					wantSource = "simulator"
				}
				if observed.Assets[0].SourceDeviceMode != wantSource {
					t.Fatal("incorrect source device mode", observed.Assets[0].SourceDeviceMode)
				}
			}
			if !sealed || observed.Completeness != Complete || len(observed.Assets) != 1 || observed.Assets[0].SourceRunID == nil || *observed.Assets[0].SourceRunID != sourceRun || len(observed.Assets[0].ChecksumSHA256) != 64 {
				t.Fatalf("bad manifest %s", manifest)
			}
		})
	}
	if id("select count(*) from inspection_observations where project_id=$1 and manifest_json->'assets'->0->>'sourceDeviceMode'='unverified'", project) != 2 {
		t.Fatal("later device association rewrote older manifests")
	}
	var original sql.NullInt64
	if err := db.QueryRow("select task_run_id from assets where id=$1", asset).Scan(&original); err != nil || original.Int64 != sourceRun {
		t.Fatal("asset provenance changed")
	}
}
