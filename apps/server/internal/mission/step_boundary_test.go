package mission

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"aerosight/server/internal/migrations"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/testdb"
)

func TestV2StepBoundaryProtectsInputsDependenciesOutputsAndCancellation(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	id := func(query string, args ...any) int64 {
		t.Helper()
		var v int64
		if err := db.QueryRow(query, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	uid := id("insert into users(name,email,password) values('boundary','boundary@example.com','unused') returning id")
	team := id("insert into teams(name) values('boundary') returning id")
	exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid)
	project := id("insert into projects(team_id,name) values($1,'boundary') returning id", team)
	unusedTask := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'unrun definition','manual','typed-task-v2') returning id", project, team)
	unusedVersion := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", project, team, unusedTask)
	exec("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses) values($1,$2,$3,1,'unrun','unrun','report.generate','report.generate')", project, team, unusedVersion)
	for _, tc := range []struct {
		name, runStatus, priorStatus, priorOutput, condition, parameters, handlerOutput, wantStep, wantRun string
		calls                                                                                              int
	}{
		{"success", "running", "succeeded", `{"count":2}`, `null`, `{"count":"steps.prior.outputs.count"}`, `{"reportId":"report"}`, "succeeded", "running", 1},
		{"input type", "running", "succeeded", `{"count":2}`, `null`, `{"count":"bad"}`, `{"reportId":"report"}`, "failed", "failed", 0},
		{"skipped dependency", "running", "skipped", `{}`, `null`, `{"count":2}`, `{"reportId":"report"}`, "failed", "failed", 0},
		{"bad prior output", "running", "succeeded", `{"count":"bad"}`, `null`, `{"count":2}`, `{"reportId":"report"}`, "failed", "failed", 0},
		{"false condition", "running", "succeeded", `{"count":2}`, `{"op":"eq","left":{"value":1},"right":{"value":2}}`, `{"count":2}`, `{"reportId":"report"}`, "skipped", "running", 0},
		{"bad output rollback", "running", "succeeded", `{"count":2}`, `null`, `{"count":2}`, `{"reportId":false}`, "failed", "failed", 1},
		{"canceled", "canceled", "succeeded", `{"count":2}`, `null`, `{"count":2}`, `{"reportId":"report"}`, "running", "canceled", 0},
		{"paused", "paused", "succeeded", `{"count":2}`, `null`, `{"count":2}`, `{"reportId":"report"}`, "running", "paused", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,$3,'manual','typed-task-v2') returning id", project, team, tc.name)
			version := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", project, team, task)
			prior := id(`insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,capability_code,action,uses,output_schema_json) values($1,$2,$3,1,'prior','prior','report.generate','report.generate','report.generate','{"type":"object","properties":{"count":{"type":"integer"}},"required":["count"]}') returning id`, project, team, version)
			step := id(`insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,capability_code,action,uses,parameters_json,condition_json,depends_on_json,input_schema_json,output_schema_json)
   values($1,$2,$3,2,'report','report','report.generate','report.generate','report.generate',$4,nullif($5::jsonb,'null'::jsonb),'["prior"]','{"type":"object","properties":{"count":{"type":"integer"}},"required":["count"],"additionalProperties":false}','{"type":"object","properties":{"reportId":{"type":"string"}},"required":["reportId"],"additionalProperties":false}') returning id`, project, team, version, tc.parameters, tc.condition)
			run := id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status,created_by_user_id,input_snapshot_json) values($1,$2,$3,$4,'manual',$5,$6,'{\"inputs\":{}}') returning id", project, team, task, version, tc.runStatus, uid)
			exec("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status,output_snapshot_json) values($1,$2,$3,$4,1,$5,$6)", project, team, run, prior, tc.priorStatus, tc.priorOutput)
			runStep := id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,2,'running') returning id", project, team, run, step)
			if tc.name == "success" {
				exec("update task_run_steps set status='pending' where id=$1", runStep)
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(map[string]any{"taskRunId": run})
				err = NewProcessor(nil).Handler(ctx, tx, outbox.Event{ProjectID: int(project), TeamID: int(team), EventType: "task_run.triggered", Payload: raw})
				if err != nil {
					tx.Rollback()
					t.Fatal(err)
				}
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
				var attempts int
				if err = db.QueryRow("select max_attempts from outbox_events where project_id=$1 and event_type='task.step.report.requested' and payload_json->>'taskRunStepId'=$2", project, fmt.Sprint(runStep)).Scan(&attempts); err != nil || attempts != 1 {
					t.Fatalf("wrong template/run step association: %d %v", attempts, err)
				}
			}
			calls := 0
			handler := WithTaskStepFailurePolicy(func(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
				calls++
				prepared, ok := StepExecution(ctx)
				if !ok || prepared.UserID != int32(uid) || prepared.Parameters["count"] != float64(2) {
					return fmt.Errorf("TASK_TEST_PREPARED_CONTEXT_INVALID")
				}
				if _, err := tx.ExecContext(ctx, "update tasks set description='business-side-effect' where id=$1", task); err != nil {
					return err
				}
				_, err := tx.ExecContext(ctx, "update task_run_steps set status='succeeded',output_snapshot_json=$2 where id=$1", runStep, tc.handlerOutput)
				return err
			})
			raw, _ := json.Marshal(map[string]any{"taskRunId": run, "taskRunStepId": runStep})
			event := outbox.Event{ProjectID: int(project), TeamID: int(team), Payload: raw, Attempts: 1, MaxAttempts: 1}
			invoke := func() {
				t.Helper()
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				if err = handler(ctx, tx, event); err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
			}
			invoke()
			invoke()
			var stepState, runState string
			var description sql.NullString
			if err := db.QueryRow("select rs.status,r.status,t.description from task_run_steps rs join task_runs r on r.id=rs.task_run_id join tasks t on t.id=r.task_id where rs.id=$1", runStep).Scan(&stepState, &runState, &description); err != nil {
				t.Fatal(err)
			}
			if calls != tc.calls || stepState != tc.wantStep || runState != tc.wantRun {
				t.Fatalf("calls=%d step=%s run=%s want %d/%s/%s", calls, stepState, runState, tc.calls, tc.wantStep, tc.wantRun)
			}
			if tc.wantStep != "succeeded" && description.Valid && description.String == "business-side-effect" {
				t.Fatal("failed output committed business writes")
			}
			if tc.name == "paused" {
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(map[string]any{"taskRunId": run, "control": "resume"})
				if err = NewProcessor(nil).Handler(ctx, tx, outbox.Event{ProjectID: int(project), TeamID: int(team), EventType: "mission.control", Payload: raw}); err != nil {
					tx.Rollback()
					t.Fatal(err)
				}
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
				var wakes int
				if err := db.QueryRow("select count(*) from outbox_events where project_id=$1 and event_type='task_run.transitioned' and payload_json->>'taskRunId'=$2 and payload_json->>'reason'='operator_resumed'", project, fmt.Sprint(run)).Scan(&wakes); err != nil || wakes != 1 {
					t.Fatal("resume omitted execution wake", wakes, err)
				}
			}
		})
	}
}
