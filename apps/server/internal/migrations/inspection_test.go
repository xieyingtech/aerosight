package migrations

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"aerosight/server/internal/testdb"
)

func TestInspectionUpgradePreservesHistoryAndEnforcesScope(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	source, _ := fs.Sub(Files, "sql")
	list, err := Read(source)
	if err != nil {
		t.Fatal(err)
	}
	old := fstest.MapFS{}
	for _, m := range list {
		if m.Name < "0073_" {
			old[m.Name] = &fstest.MapFile{Data: []byte(m.SQL)}
		}
	}
	if _, err := Run(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	must := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	id := func(query string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	team := id("insert into teams(name) values('inspection test') returning id")
	project := id("insert into projects(team_id,name) values($1,'inspection test') returning id", team)
	other := id("insert into projects(team_id,name) values($1,'other inspection test') returning id", team)
	task := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'inspection','manual','legacy') returning id", project, team)
	version := id("insert into task_versions(project_id,team_id,task_id,version,status,definition_json,script) values($1,$2,$3,1,'published','{\"legacy\":true}','legacy') returning id", project, team, task)
	var before string
	if err := db.QueryRow("select definition_json::text||script from task_versions where id=$1", version).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, db, source); err != nil {
		t.Fatal(err)
	}
	if applied, err := Run(ctx, db, source); err != nil || len(applied) != 0 {
		t.Fatalf("repeat migration: %v %v", applied, err)
	}
	var after, dsl string
	var revision int
	if err := db.QueryRow("select definition_json::text||script,dsl_version,author_revision from task_versions where id=$1", version).Scan(&after, &dsl, &revision); err != nil {
		t.Fatal(err)
	}
	if before != after || dsl != "aerosight/v1" || revision != 0 {
		t.Fatal("changed legacy snapshot")
	}
	step := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action) values($1,$2,$3,1,'observe','observe','inspection.observe') returning id", project, team, version)
	makeRun := func() int64 {
		return id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source) values($1,$2,$3,$4,'manual') returning id", project, team, task, version)
	}
	runA, runB := makeRun(), makeRun()
	makeStep := func(run int64) int64 {
		return id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position) values($1,$2,$3,$4,1) returning id", project, team, run, step)
	}
	runStepA, runStepB := makeStep(runA), makeStep(runB)
	insertObservation := `insert into inspection_observations(project_id,team_id,task_run_id,task_run_step_id,source_mode,scope_description,observed_from,observed_to)
 values($1,$2,$3,$4,'assets','selected images',now(),now()) returning id::text`
	observation := func(run, step int64) string {
		var value string
		if err := db.QueryRow(insertObservation, project, team, run, step).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	first, second := observation(runA, runStepA), observation(runB, runStepB)
	if _, err := db.Exec(insertObservation, project, team, runA, runStepB); err == nil {
		t.Fatal("accepted a step from another run")
	}
	asset := id(`insert into assets(project_id,team_id,task_run_id,kind,storage_key,logical_key,version) values($1,$2,$3,'image','test-image','test-image',1) returning id`, project, team, runA)
	for _, obs := range []string{first, second} {
		must("insert into inspection_observation_assets(observation_id,project_id,asset_id,asset_version,source_run_id) values($1,$2,$3,1,$4)", obs, project, asset, runA)
	}
	var original int64
	if err := db.QueryRow("select task_run_id from assets where id=$1", asset).Scan(&original); err != nil || original != runA {
		t.Fatal("changed original asset run")
	}
	if _, err := db.Exec("insert into inspection_observation_assets(observation_id,project_id,asset_id,asset_version) values($1,$2,$3,1)", first, other, asset); err == nil {
		t.Fatal("accepted cross-project evidence")
	}
	if _, err := db.Exec("update assets set version=2 where id=$1", asset); err == nil {
		t.Fatal("allowed rewriting a referenced asset version")
	}
	must("insert into task_trigger_records(project_id,team_id,task_id,task_version_id,occurrence_key,outcome,reason) values($1,$2,$3,$4,'schedule:now','skipped','active-run')", project, team, task, version)
	if _, err := db.Exec("insert into task_trigger_records(project_id,team_id,task_id,task_version_id,occurrence_key,outcome,reason) values($1,$2,$3,$4,'schedule:now','skipped','active-run')", project, team, task, version); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatal("occurrence was not unique")
	}
}
