package inspection

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"aerosight/server/internal/migrations"
	"aerosight/server/internal/mission"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/testdb"
	"github.com/google/uuid"
)

func TestNativeDetectFreezesScopeAndDoesNotInferNoIssue(t *testing.T) {
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
	uid := id("insert into users(name,email,password) values('detect','detect@example.com','unused') returning id")
	team := id("insert into teams(name) values('detect') returning id")
	exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid)
	project := id("insert into projects(team_id,name) values($1,'detect') returning id", team)
	adapter := id("insert into device_adapters(project_id,team_id,name,adapter_type,protocol_version,status) values($1,$2,'native','dji-flighthub2','2','connected') returning id", project, team)
	for _, tc := range []struct {
		name                                   string
		alerts                                 int
		missing, wrongRun, canceled, badOutput bool
	}{
		{name: "native", alerts: 1}, {name: "zero"}, {name: "missing", alerts: 1, missing: true}, {name: "wrong run", alerts: 1, wrongRun: true}, {name: "canceled", alerts: 1, canceled: true}, {name: "bad output", alerts: 1, badOutput: true}, {name: "limit", alerts: 1001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,$3,'manual','typed-task-v2') returning id", project, team, tc.name)
			version := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", project, team, task)
			observationID := uuid.NewString()
			params, _ := json.Marshal(map[string]any{"source": "flighthub-ai", "observationId": observationID})
			outputSchema := `{"type":"object","properties":{"evidenceSetId":{"type":"string"}},"required":["evidenceSetId"],"additionalProperties":false}`
			if tc.badOutput {
				outputSchema = `{"type":"object","required":["invalid"]}`
			}
			observeDef := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses) values($1,$2,$3,1,'observe','observe','inspection.observe','inspection.observe') returning id", project, team, version)
			detectDef := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses,parameters_json,output_schema_json) values($1,$2,$3,2,'detect','detect','inspection.detect','inspection.detect',$4,$5) returning id", project, team, version, params, outputSchema)
			runStatus := "running"
			if tc.canceled {
				runStatus = "canceled"
			}
			run := id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status,created_by_user_id) values($1,$2,$3,$4,'manual',$5,$6) returning id", project, team, task, version, runStatus, uid)
			observedStep := id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,1,'succeeded') returning id", project, team, run, observeDef)
			detectStep := id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,2,'running') returning id", project, team, run, detectDef)
			scope := Scope{ProjectID: int(project), TeamID: int(team)}
			flight := fmt.Sprintf("fixture-flight-%d", run)
			observation := Observation{ID: observationID, ContractVersion: ContractVersion, Run: RunRef{Scope: scope, RunID: run, StepID: observedStep}, Mode: ExistingFlight, Flight: &FlightRef{Scope: scope, ConnectorID: adapter, FlightUUID: flight}, Assets: []AssetRef{}, Completeness: AlertOnly, ScopeDescription: "Native alert fixture", ObservedFrom: time.Now().UTC(), ObservedTo: time.Now().UTC()}
			manifest, _ := json.Marshal(observation)
			exec(`insert into inspection_observations(id,project_id,team_id,task_run_id,task_run_step_id,source_mode,connector_instance_id,remote_flight_id,completeness,scope_description,observed_from,observed_to,manifest_json,sealed_at)
   values($1,$2,$3,$4,$5,'existing-flight',$6,$7,'alert-only','Native fixture',now(),now(),$8,now())`, observationID, project, team, run, observedStep, adapter, flight, manifest)
			if tc.wrongRun {
				another := id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status,created_by_user_id) values($1,$2,$3,$4,'manual','running',$5) returning id", project, team, task, version, uid)
				detectStep = id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,2,'running') returning id", project, team, another, detectDef)
				run = another
			}
			native := `{"algorithmSource":1,"timestamp":1789000000000,"reason":"person","status":0,"targets":[{"targetType":1,"targetValue":3,"label":"person"}],"location":{"latitude":30,"longitude":120},"signedUrl":"SECRET","remoteId":"SECRET"}`
			// Include another flight's alert; the snapshot must never consume it.
			for n := 0; n < tc.alerts+1; n++ {
				resource := id("insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id) values($1,$2,$3,'ai-alert',$4) returning id", project, team, adapter, fmt.Sprintf("%s-alert-%d", flight, n))
				alertFlight := flight
				if n == tc.alerts {
					alertFlight = "other-" + flight
				}
				exec("insert into inspection_alert_sources(project_id,connector_instance_id,remote_resource_id,remote_flight_id,evidence_json) values($1,$2,$3,$4,$5)", project, adapter, resource, alertFlight, native)
				if tc.missing {
					exec("update connector_remote_resources set status='missing',missing_at=now() where id=$1", resource)
				}
			}
			handler := mission.WithTaskStepFailurePolicy(NewDetectProcessor().Handler)
			payload, _ := json.Marshal(map[string]any{"taskRunId": run, "taskRunStepId": detectStep})
			event := outbox.Event{ProjectID: int(project), TeamID: int(team), Payload: payload, Attempts: 1, MaxAttempts: 1}
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
			if err := db.QueryRow("select count(*) from inspection_evidence_sets where task_run_step_id=$1", detectStep).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if tc.wrongRun || tc.canceled || tc.badOutput || tc.alerts > 1000 {
				if count != 0 {
					t.Fatal("invalid/canceled step wrote evidence")
				}
				return
			}
			if count != 1 {
				t.Fatal("evidence not idempotent")
			}
			var frozen []byte
			if err := db.QueryRow("select evidence_json from inspection_evidence_sets where task_run_step_id=$1", detectStep).Scan(&frozen); err != nil {
				t.Fatal(err)
			}
			var evidence EvidenceSet
			if err := json.Unmarshal(frozen, &evidence); err != nil {
				t.Fatal(err)
			}
			if evidence.CanConcludeNoIssue() || len(evidence.NativeAlerts) != tc.alerts || strings.Contains(string(frozen), "SECRET") || strings.Contains(string(frozen), "confidence") {
				t.Fatalf("unsafe evidence: %s", frozen)
			}
			want := AlertOnly
			if tc.alerts == 0 {
				want = Unavailable
			}
			if tc.missing {
				want = Partial
			}
			if evidence.Completeness != want {
				t.Fatalf("got %s want %s", evidence.Completeness, want)
			}
			if tc.alerts > 0 {
				if evidence.NativeAlerts[0].AlgorithmSource == nil || *evidence.NativeAlerts[0].AlgorithmSource != 1 || evidence.NativeAlerts[0].Position.Source != "unknown" || !strings.Contains(string(evidence.NativeAlerts[0].Targets), `"targetValue": 3`) {
					t.Fatalf("semantics lost: %s", frozen)
				}
			}
			exec("update inspection_alert_sources set evidence_json='{}' where project_id=$1 and remote_flight_id=$2", project, flight)
			var after []byte
			if err := db.QueryRow("select evidence_json from inspection_evidence_sets where task_run_step_id=$1", detectStep).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if string(after) != string(frozen) {
				t.Fatal("provider update rewrote sealed evidence")
			}
		})
	}
}
