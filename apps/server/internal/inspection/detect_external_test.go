package inspection

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/mission"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/testdb"
	"github.com/google/uuid"
)

func TestExternalDetectWaitsForWholeFrozenImageSet(t *testing.T) {
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
	uid := id("insert into users(name,email,password) values('external','external@example.com','unused') returning id")
	team := id("insert into teams(name) values('external') returning id")
	exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid)
	project := id("insert into projects(team_id,name) values($1,'external') returning id", team)
	provider := id("insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status) values($1,$2,'fixture','http-json','https://fixture.example','active') returning id", project, team)
	definition := id("insert into algorithm_definitions(project_id,team_id,provider_id,name,capability_code) values($1,$2,$3,'fixture','detection') returning id", project, team, provider)
	algorithmVersion := id("insert into algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process) values($1,$2,$3,1,'published','synchronous','fixture-model') returning id", project, team, definition)
	for _, name := range []string{"detections", "zero", "single failure", "invalid result", "canceled", "limit"} {
		t.Run(name, func(t *testing.T) {
			task := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,$3,'manual','typed-task-v2') returning id", project, team, name)
			version := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", project, team, task)
			prior := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses) values($1,$2,$3,1,'observe','observe','inspection.observe','inspection.observe') returning id", project, team, version)
			observationID := uuid.NewString()
			limit := 64
			if name == "limit" {
				limit = 1
			}
			parameters, _ := json.Marshal(map[string]any{"source": "external", "observationId": observationID, "algorithmDefinitionVersionId": algorithmVersion, "maxImages": limit})
			step := id(`insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses,parameters_json,output_schema_json) values($1,$2,$3,2,'detect','detect','inspection.detect','inspection.detect',$4,'{"type":"object","properties":{"evidenceSetId":{"type":"string"}},"required":["evidenceSetId"],"additionalProperties":false}') returning id`, project, team, version, parameters)
			run := id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status,created_by_user_id) values($1,$2,$3,$4,'manual','running',$5) returning id", project, team, task, version, uid)
			priorRunStep := id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,1,'succeeded') returning id", project, team, run, prior)
			runStep := id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,2,'running') returning id", project, team, run, step)
			scope := Scope{ProjectID: int(project), TeamID: int(team)}
			observation := Observation{ID: observationID, ContractVersion: ContractVersion, Run: RunRef{Scope: scope, RunID: run, StepID: priorRunStep}, Mode: Assets, Completeness: Complete, ScopeDescription: "two protocol fixtures", ObservedFrom: time.Now().UTC(), ObservedTo: time.Now().UTC()}
			for n := 0; n < 2; n++ {
				asset := id("insert into assets(project_id,team_id,kind,mime_type,storage_key,logical_key) values($1,$2,'image','image/jpeg',$3,$3) returning id", project, team, fmt.Sprintf("projects/%d/%d-%d.jpg", project, run, n))
				observation.Assets = append(observation.Assets, AssetRef{Scope: scope, AssetID: asset, Version: 1, ChecksumSHA256: strings.Repeat("a", 64)})
			}
			manifest, _ := json.Marshal(observation)
			exec("insert into inspection_observations(id,project_id,team_id,task_run_id,task_run_step_id,source_mode,completeness,scope_description,observed_from,observed_to,manifest_json,sealed_at) values($1,$2,$3,$4,$5,'assets','complete','fixtures',now(),now(),$6,now())", observationID, project, team, run, priorRunStep, manifest)
			for _, asset := range observation.Assets {
				exec("insert into inspection_observation_assets(observation_id,project_id,asset_id,asset_version) values($1,$2,$3,1)", observationID, project, asset.AssetID)
			}
			handler := mission.WithTaskStepFailurePolicy(NewDetectProcessor(algorithm.NewTrigger(algorithm.NewAssetURLSigner(strings.Repeat("s", 32), "https://fixture.example"))).Handler)
			raw, _ := json.Marshal(map[string]any{"taskRunId": run, "taskRunStepId": runStep})
			event := outbox.Event{ProjectID: int(project), TeamID: int(team), Payload: raw, Attempts: 1, MaxAttempts: 1}
			invoke := func() {
				t.Helper()
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
			invoke()
			invoke()
			var count int
			if err := db.QueryRow("select count(*) from algorithm_runs where task_run_step_id=$1", runStep).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if name == "limit" {
				if count != 0 {
					t.Fatal("over-limit task dispatched children")
				}
				return
			}
			if count != 2 {
				t.Fatal("child set duplicated", count)
			}
			canonical := `{"result":{"kind":"detection","detections":[]},"source":{"modelRevision":"fixture-v1"}}`
			if name == "detections" {
				canonical = `{"result":{"kind":"detection","detections":[{"detectionKey":"one","label":"person","confidence":0.8}]},"source":{"modelRevision":"fixture-v1"}}`
			}
			// These are labeled child-result fixtures; provider execution is tested separately.
			exec("update algorithm_runs set status='succeeded',canonical_result_json=$3 where task_run_step_id=$1 and input_asset_id=$2", runStep, observation.Assets[0].AssetID, canonical)
			invoke()
			var status string
			if err := db.QueryRow("select status from task_run_steps where id=$1", runStep).Scan(&status); err != nil || status != "running" {
				t.Fatal("first child advanced parent", status, err)
			}
			secondStatus := "succeeded"
			if name == "single failure" {
				secondStatus = "failed"
			}
			if name == "invalid result" {
				canonical = `{}`
			}
			exec("update algorithm_runs set status=$3,canonical_result_json=$4 where task_run_step_id=$1 and input_asset_id=$2", runStep, observation.Assets[1].AssetID, secondStatus, canonical)
			if name == "canceled" {
				exec("update task_runs set status='canceled' where id=$1", run)
			}
			invoke()
			invoke()
			if err := db.QueryRow("select count(*) from inspection_evidence_sets where task_run_step_id=$1", runStep).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if name == "single failure" || name == "invalid result" {
				var state, code string
				if err := db.QueryRow("select status,output_snapshot_json->>'errorCode' from task_run_steps where id=$1", runStep).Scan(&state, &code); err != nil {
					t.Fatal(err)
				}
				want := "INSPECTION_EXTERNAL_CHILD_FAILED"
				if name == "invalid result" {
					want = "INSPECTION_EXTERNAL_RESULT_INVALID"
				}
				if state != "failed" || code != want {
					t.Fatal("failure not finalized", state, code)
				}
			}
			if name == "single failure" || name == "invalid result" || name == "canceled" {
				if count != 0 {
					t.Fatal("failed or late input produced evidence")
				}
				return
			}
			if count != 1 {
				t.Fatal("evidence duplicate or missing", count)
			}
			var frozen []byte
			if err := db.QueryRow("select evidence_json from inspection_evidence_sets where task_run_step_id=$1", runStep).Scan(&frozen); err != nil {
				t.Fatal(err)
			}
			var evidence EvidenceSet
			if err := json.Unmarshal(frozen, &evidence); err != nil {
				t.Fatal(err)
			}
			if len(evidence.ExternalResults) != 2 || !evidence.CanConcludeNoIssue() {
				t.Fatalf("incomplete evidence %s", frozen)
			}
			expectedCandidates := 0
			if name == "detections" {
				expectedCandidates = 2
			}
			if len(evidence.Candidates) != expectedCandidates {
				t.Fatal("detection candidates lost")
			}
		})
	}
}
