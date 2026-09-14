package httpapi

import (
	"aerosight/server/internal/flighthub"
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestInspectionFlightBindingRejectsMisassociation(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	cid, did := f.device(t, team, pid)
	id := func(q string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := f.db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	task := id(`insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'binding','manual','typed-task-v2') returning id`, pid, team)
	version := id(`insert into task_versions(project_id,team_id,task_id,version,definition_json,script) values($1,$2,$3,1,'{}','typed-task-v2') returning id`, pid, team, task)
	step := id(`insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action) values($1,$2,$3,1,'observe','observe','inspection.observe') returning id`, pid, team, version)
	run := func() int64 {
		return id(`insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status) values($1,$2,$3,$4,'manual','running') returning id`, pid, team, task, version)
	}
	business, flight, otherRun := run(), run(), run()
	runStep := func(r int64) int64 {
		return id(`insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position) values($1,$2,$3,$4,1) returning id`, pid, team, r, step)
	}
	businessStep, otherStep, flightStep := runStep(business), runStep(otherRun), runStep(flight)
	wayline := id(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id) values($1,$2,$3,'wayline','binding') returning id`, pid, team, cid)
	user := id(`select id from users where email='admin@example.com'`)
	var approval string
	if err := f.db.QueryRow(`insert into approval_requests(id,project_id,team_id,resource_type,resource_id,action,requested_by_user_id,expires_at) values(gen_random_uuid(),$1,$2,'task_run',$3,'flighthub.flight-task.create',$4,now()+interval '1 hour') returning id::text`, pid, team, fmt.Sprint(flight), user).Scan(&approval); err != nil {
		t.Fatal(err)
	}
	var job string
	if err := f.db.QueryRow(`insert into connector_action_jobs(project_id,team_id,connector_instance_id,task_run_id,device_id,wayline_resource_id,approval_request_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json) values($1,$2,$3,$4,$5,$6,$7,$8,'flight-task-create','binding-test-key',$9,'{}') returning id::text`, pid, team, cid, flight, did, wayline, approval, user, strings.Repeat("a", 64)).Scan(&job); err != nil {
		t.Fatal(err)
	}
	query := `insert into inspection_flight_bindings(project_id,team_id,business_run_id,business_step_id,connector_instance_id,flight_run_id,action_job_id) values($1,$2,$3,$4,$5,$6,$7)`
	for _, bad := range []struct {
		name                   string
		project, connector     int
		business, step, flight int64
	}{
		{"wrong step", pid, cid, business, otherStep, flight},
		{"same physical and business run", pid, cid, flight, flightStep, flight},
		{"wrong flight run", pid, cid, business, businessStep, otherRun},
		{"wrong connector", pid, cid + 999, business, businessStep, flight},
		{"wrong project", pid + 999, cid, business, businessStep, flight},
	} {
		t.Run(bad.name, func(t *testing.T) {
			if _, err := f.db.Exec(query, bad.project, team, bad.business, bad.step, bad.connector, bad.flight, job); err == nil {
				t.Fatal("invalid binding accepted")
			}
		})
	}
	if _, err := f.db.Exec(query, pid, team, business, businessStep, cid, flight, job); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(query, pid, team, otherRun, otherStep, cid, flight, job); err == nil {
		t.Fatal("shared flight action accepted")
	}
	if _, err := f.db.Exec(`update task_runs set status='succeeded',finished_at=now() where id=$1`, flight); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := f.db.QueryRow(`select status from task_runs where id=$1`, business).Scan(&status); err != nil || status != "running" {
		t.Fatal("flight completion changed business run", status, err)
	}
	// A late ACK binds evidence ownership to the business Run while the canonical
	// remote projection remains the independent physical flight Run.
	for _, q := range []string{
		`update device_adapters set adapter_type='dji-flighthub2' where id=$1`,
	} {
		if _, err := f.db.Exec(q, cid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.db.Exec(`update connector_action_jobs set status='reconciling',attempt_count=1 where id=$1`, job); err != nil {
		t.Fatal(err)
	}
	store := flighthub.NewSQLFlightActionStore(f.db)
	action := flighthub.FlightActionJob{ID: job, ProjectID: pid, TeamID: team, ConnectorInstanceID: int64(cid), TaskRunID: int(flight)}
	if _, err := f.db.Exec(`insert into inspection_flight_ownership(project_id,connector_instance_id,remote_flight_id,ownership) values($1,$2,'legacy-conflict','legacy')`, pid, cid); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAccepted(context.Background(), action, "legacy-conflict"); err == nil {
		t.Fatal("legacy ownership overwritten")
	}
	if n := id(`select count(*) from connector_remote_resources where project_id=$1 and remote_id='legacy-conflict'`, pid); n != 0 {
		t.Fatal("partial projection on conflict")
	}

	if err := store.RecordAccepted(context.Background(), action, "remote-binding"); err != nil {
		t.Fatal(err)
	}
	var owner, canonical int64
	if err := f.db.QueryRow(`select task_run_id from inspection_flight_ownership where project_id=$1 and remote_flight_id='remote-binding'`, pid).Scan(&owner); err != nil || owner != business {
		t.Fatal("wrong owner", owner, err)
	}
	if err := f.db.QueryRow(`select canonical_target_id::bigint from connector_remote_resources where project_id=$1 and remote_id='remote-binding'`, pid).Scan(&canonical); err != nil || canonical != flight {
		t.Fatal("wrong projection", canonical, err)
	}
	if err := store.RecordAccepted(context.Background(), action, "remote-binding"); err != nil {
		t.Fatal("ACK replay", err)
	}

	if err := store.RecordAccepted(context.Background(), action, "different-identity"); err == nil || err.Error() != "INSPECTION_FLIGHT_IDENTITY_CHANGED" {
		t.Fatal("remote identity changed", err)
	}

	if err := store.Complete(context.Background(), action, flighthub.FlightTask{UUID: "remote-binding", Status: "success"}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`select status from task_runs where id=$1`, business).Scan(&status); err != nil || status != "running" {
		t.Fatal("reconciliation completed business workflow", status, err)
	}

}
