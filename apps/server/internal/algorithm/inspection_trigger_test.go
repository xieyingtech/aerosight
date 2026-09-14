package algorithm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aerosight/server/internal/migrations"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/testdb"
	"github.com/google/uuid"
)

func TestInspectionAlgorithmChildrenKeepProvenanceAndParentPending(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	id := func(q string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	team := id("insert into teams(name) values('batch') returning id")
	project := id("insert into projects(team_id,name) values($1,'batch') returning id", team)
	task := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'batch','manual','typed-task-v2') returning id", project, team)
	version := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", project, team, task)
	run := id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status) values($1,$2,$3,$4,'manual','running') returning id", project, team, task, version)
	sourceRun := id("insert into task_runs(project_id,team_id,task_id,trigger_source,status) values($1,$2,$3,'manual','succeeded') returning id", project, team, task)
	prior := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses) values($1,$2,$3,1,'observe','observe','inspection.observe','inspection.observe') returning id", project, team, version)
	step := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses) values($1,$2,$3,2,'detect','detect','inspection.detect','inspection.detect') returning id", project, team, version)
	observedStep := id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,1,'succeeded') returning id", project, team, run, prior)
	runStep := id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status,input_snapshot_json) values($1,$2,$3,$4,2,'running','{\"prepared\":true}') returning id", project, team, run, step)
	provider := id("insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status) values($1,$2,'batch','http-json','https://fixture.example','active') returning id", project, team)
	definition := id("insert into algorithm_definitions(project_id,team_id,provider_id,name,capability_code) values($1,$2,$3,'batch','detection') returning id", project, team, provider)
	algorithmVersion := id("insert into algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process,output_mapping_json) values($1,$2,$3,1,'published','synchronous','fixture',$4) returning id", project, team, definition, `{"detectionsPath":"results","keyPath":"id","labelPath":"class","confidencePath":"score","geometryPath":"bbox"}`)
	assets := []int64{}
	manifestAssets := []map[string]any{}
	for n := 0; n < 2; n++ {
		asset := id("insert into assets(project_id,team_id,task_run_id,kind,mime_type,storage_key,logical_key) values($1,$2,$3,'image','image/jpeg',$4,$4) returning id", project, team, sourceRun, fmt.Sprintf("projects/fixture/%d.jpg", n))
		assets = append(assets, asset)
		manifestAssets = append(manifestAssets, map[string]any{"assetId": asset, "version": 1, "checksumSha256": strings.Repeat("a", 64)})
	}
	observation := uuid.NewString()
	manifest, _ := json.Marshal(map[string]any{"assets": manifestAssets})
	exec("insert into inspection_observations(id,project_id,team_id,task_run_id,task_run_step_id,source_mode,completeness,scope_description,observed_from,observed_to,manifest_json,sealed_at) values($1,$2,$3,$4,$5,'assets','complete','fixture',now(),now(),$6,now())", observation, project, team, run, observedStep, manifest)
	for _, asset := range assets {
		exec("insert into inspection_observation_assets(observation_id,project_id,asset_id,asset_version,source_run_id) values($1,$2,$3,1,$4)", observation, project, asset, sourceRun)
	}
	trigger := NewTrigger(NewAssetURLSigner(strings.Repeat("s", 32), "https://fixture.example"))
	for repeat := 0; repeat < 2; repeat++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, asset := range assets {
			if err = trigger.QueueInspectionAsset(ctx, tx, int(project), int(team), int(run), runStep, observation, int(asset), algorithmVersion, nil); err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow("select count(*) from algorithm_runs where task_run_step_id=$1", runStep).Scan(&count); err != nil || count != 2 {
		t.Fatal("child idempotency failed", count, err)
	}
	var input string
	if err := db.QueryRow("select input_snapshot_json::text from task_run_steps where id=$1", runStep).Scan(&input); err != nil || !strings.Contains(input, "prepared") {
		t.Fatal("prepared inputs overwritten")
	}
	rows, err := db.Query("select id::text from algorithm_runs where task_run_step_id=$1 order by input_asset_id", runStep)
	if err != nil {
		t.Fatal(err)
	}
	var children []string
	for rows.Next() {
		var child string
		if err = rows.Scan(&child); err != nil {
			t.Fatal(err)
		}
		children = append(children, child)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	// The dispatch gate rechecks identity before any provider or asset call.
	uid := id("insert into users(name,email,password) values('dispatch','dispatch@example.com','unused') returning id")
	exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid)
	exec("update task_runs set created_by_user_id=$2 where id=$1", run, uid)
	event := outbox.Event{ProjectID: int(project), TeamID: int(team)}
	checkDispatch := func(want bool) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		allowed, err := inspectionDispatchAllowed(ctx, tx, event, children[0])
		if err != nil || allowed != want {
			t.Fatalf("dispatch=%v want %v err=%v", allowed, want, err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	checkInspectionHTTPDispatch(t, db, project, team, provider, runStep, children[0])
	checkDispatch(true)
	exec("update task_runs set status='paused' where id=$1", run)
	checkDispatch(false)
	var childStatus string
	if err := db.QueryRow("select status from algorithm_runs where id=$1", children[0]).Scan(&childStatus); err != nil || childStatus != "queued" {
		t.Fatal("pause discarded child", childStatus, err)
	}
	exec("update task_runs set status='running',state_version=state_version+1 where id=$1", run)
	resumePayload, _ := json.Marshal(map[string]any{"taskRunId": run, "control": "resume"})
	for repeat := 0; repeat < 2; repeat++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = ResumeInspectionChildren(ctx, tx, outbox.Event{ProjectID: int(project), TeamID: int(team), Payload: resumePayload}); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow("select count(*) from outbox_events where event_id like 'inspection-resume:%' and project_id=$1", project).Scan(&count); err != nil || count != 2 {
		t.Fatal("resume duplicated or lost child wakes", count, err)
	}
	exec("delete from team_members where team_id=$1 and user_id=$2", team, uid)
	checkDispatch(false)
	if err := db.QueryRow("select status from algorithm_runs where id=$1", children[0]).Scan(&childStatus); err != nil || childStatus != "failed" {
		t.Fatal("revoked child still queued", childStatus, err)
	}
	exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid)
	exec("update algorithm_runs set status='queued',error_code=null where id=$1", children[0])
	exec("update task_runs set status='canceled' where id=$1", run)
	checkDispatch(false)
	if err := db.QueryRow("select status from algorithm_runs where id=$1", children[0]).Scan(&childStatus); err != nil || childStatus != "canceled" {
		t.Fatal("canceled parent left child dispatchable", childStatus, err)
	}
	// Restore fixture state for the independent child completion assertions below.
	exec("update task_runs set status='running' where id=$1", run)
	exec("update algorithm_runs set status='queued',error_code=null where id=$1", children[0])
	exec("delete from outbox_events where project_id=$1 and event_type='inspection.algorithm.completed'", project)
	for i, child := range children {
		outcome := "succeeded"
		if i == 1 {
			outcome = "failed"
		}
		for repeat := 0; repeat < 2; repeat++ {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err = completeTaskAlgorithmStep(ctx, tx, int(project), child, outcome, "fixture_error"); err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
		}
	}
	var status string
	if err := db.QueryRow("select status from task_run_steps where id=$1", runStep).Scan(&status); err != nil || status != "running" {
		t.Fatal("child completed parent", status, err)
	}
	if err := db.QueryRow("select count(*) from outbox_events where event_type='inspection.algorithm.completed' and project_id=$1", project).Scan(&count); err != nil || count != 2 {
		t.Fatal("completion event not idempotent", count, err)
	}
	if err := db.QueryRow("select count(*) from assets where task_run_id=$1", sourceRun).Scan(&count); err != nil || count != 2 {
		t.Fatal("asset provenance changed")
	}
	// Wrong observation and changed versions must not queue another child.
	for _, observationID := range []string{uuid.NewString(), observation} {
		if observationID == observation {
			exec("update assets set object_version='changed' where id=$1", assets[0])
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = trigger.QueueInspectionAsset(ctx, tx, int(project), int(team), int(run), runStep, observationID, int(assets[0]), algorithmVersion, nil)
		tx.Rollback()
		if err == nil {
			t.Fatal("unbound or changed input accepted")
		}
	}
	// Existing single-image algorithm steps still finish directly.
	legacyVersion := id("insert into task_versions(project_id,team_id,task_id,version,status,script) values($1,$2,$3,2,'published','legacy') returning id", project, team, task)
	legacyStep := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses) values($1,$2,$3,1,'algorithm','algorithm','algorithm.run','algorithm.run') returning id", project, team, legacyVersion)
	legacyRun := id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status) values($1,$2,$3,$4,'manual','running') returning id", project, team, task, legacyVersion)
	legacyRunStep := id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,1,'running') returning id", project, team, legacyRun, legacyStep)
	legacyChild := uuid.NewString()
	exec("insert into algorithm_runs(id,project_id,team_id,algorithm_definition_version_id,input_asset_id,task_run_id,task_run_step_id,idempotency_key) values($1,$2,$3,$4,$5,$6,$7,'legacy')", legacyChild, project, team, algorithmVersion, assets[0], legacyRun, legacyRunStep)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = completeTaskAlgorithmStep(ctx, tx, int(project), legacyChild, "succeeded", ""); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("select status from task_run_steps where id=$1", legacyRunStep).Scan(&status); err != nil || status != "succeeded" {
		t.Fatal("legacy completion changed", status, err)
	}
}
