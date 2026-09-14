package tasktrigger

import (
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/testdb"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestUpstreamDatabaseJSONOutputAndReplay(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	id := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	team := id("insert into teams(name) values('upstream') returning id")
	project := id("insert into projects(team_id,name) values($1,'upstream') returning id", team)
	source := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'source','manual','') returning id", project, team)
	run := id(`insert into task_runs(project_id,team_id,task_id,trigger_source,status,output_snapshot_json) values($1,$2,$3,'manual','succeeded','{"token":"fixture","nested":{"count":2}}') returning id`, project, team, source)
	target := id("insert into tasks(project_id,team_id,name,trigger_type,script,status) values($1,$2,'downstream','upstream','','active') returning id", project, team)
	trigger := fmt.Sprintf(`{"type":"upstream","taskId":%d,"statuses":["succeeded"]}`, source)
	version := id(`insert into task_versions(project_id,team_id,task_id,version,status,script,trigger_json,input_schema_json) values($1,$2,$3,1,'published','',$4,'{"type":"object","properties":{"token":{"type":"string"},"nested":{"type":"object"}}}') returning id`, project, team, target, trigger)
	if _, err := db.Exec("update tasks set current_published_version_id=$2 where id=$1", target, version); err != nil {
		t.Fatal(err)
	}
	scheduler := NewScheduler(db, nil, time.Second, nil)
	payload, _ := json.Marshal(map[string]any{"taskRunId": run, "to": "succeeded"})
	for i := 0; i < 2; i++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = scheduler.UpstreamHandler(ctx, tx, outbox.Event{ProjectID: project, TeamID: team, Payload: payload})
		if err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	var raw []byte
	if err := db.QueryRow("select count(*) from task_runs where task_id=$1", target).Scan(&count); err != nil || count != 1 {
		t.Fatal("replay duplicate", count, err)
	}
	if err := db.QueryRow("select input_snapshot_json from task_runs where task_id=$1", target).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	inputs := snapshot["inputs"].(map[string]any)
	if inputs["token"] != "fixture" || inputs["nested"].(map[string]any)["count"] != float64(2) {
		t.Fatal("upstream outputs lost", snapshot)
	}
}
