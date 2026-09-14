package tasktrigger

import (
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestScheduleV2InputsAuthorizationRecoveryAndIsolation(t *testing.T) {
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
	uid := id("insert into users(name,email,password) values('schedule','schedule@example.com','unused') returning id")
	team := id("insert into teams(name) values('schedule') returning id")
	exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid)
	project := id("insert into projects(team_id,name) values($1,'schedule') returning id", team)
	sequence := 0
	makeTask := func(cron, timezone string) int64 {
		sequence++
		task := id("insert into tasks(project_id,team_id,name,trigger_type,script,authorized_by_user_id) values($1,$2,$4,'schedule','typed-task-v2',$3) returning id", project, team, uid, fmt.Sprintf("schedule-%d", sequence))
		trigger, _ := json.Marshal(map[string]any{"type": "schedule", "cron": cron, "timezone": timezone, "inputs": map[string]any{"ids": []any{7}}})
		schema := `{"type":"object","properties":{"count":{"type":"integer","default":2},"ids":{"type":"array","items":{"type":"integer"},"minItems":1}},"required":["ids"],"additionalProperties":false}`
		version := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version,trigger_json,input_schema_json) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2',$4,$5) returning id", project, team, task, trigger, schema)
		exec("update tasks set current_published_version_id=$1 where id=$2", version, task)
		return task
	}
	good := makeTask("* * * * *", "Asia/Shanghai")
	bad := makeTask("0 0 * * 99", "UTC") // Invalid last field must be caught even when minute does not match.
	noDelegate := makeTask("* * * * *", "UTC")
	exec("update tasks set authorized_by_user_id=null where id=$1", noDelegate)
	now := time.Date(2026, 9, 11, 0, 2, 30, 0, time.UTC)
	scheduler := NewScheduler(db, func() time.Time { return now }, time.Second, nil)
	reconcile := func(want int) {
		t.Helper()
		got, err := scheduler.ReconcileOnce(ctx)
		if err != nil || got != want {
			t.Fatalf("created %d want %d: %v", got, want, err)
		}
	}
	reconcile(1)
	reconcile(0)
	var inputs []byte
	var owner int64
	if err := db.QueryRow("select input_snapshot_json,created_by_user_id from task_runs where task_id=$1", good).Scan(&inputs, &owner); err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(inputs, &snapshot); err != nil {
		t.Fatal(err)
	}
	actual := snapshot["inputs"].(map[string]any)
	if owner != uid || actual["count"] != float64(2) || actual["ids"].([]any)[0] != float64(7) {
		t.Fatalf("snapshot %s owner %d", inputs, owner)
	}
	var errorsCount int
	if err := db.QueryRow("select count(*) from task_trigger_records where task_id in ($1,$2) and outcome='error'", bad, noDelegate).Scan(&errorsCount); err != nil || errorsCount != 2 {
		t.Fatalf("errors %d %v", errorsCount, err)
	}
	// Concurrent occurrence is audited once and never retried after a run ends.
	now = now.Add(time.Minute)
	reconcile(0)
	exec("update task_runs set status='succeeded' where task_id=$1", good)
	reconcile(0)
	// Simulate a restart after hours offline: only the current eligible minute runs.
	now = now.Add(2 * time.Hour)
	scheduler = NewScheduler(db, func() time.Time { return now }, time.Second, nil)
	reconcile(1)
	var missed int
	if err := db.QueryRow("select count(*) from task_trigger_records where task_id=$1 and outcome='missed' and interval_end is not null", good).Scan(&missed); err != nil || missed < 1 {
		t.Fatalf("missing downtime audit: %d %v", missed, err)
	}
	exec("update task_runs set status='succeeded' where task_id=$1", good)
	exec("delete from team_members where team_id=$1", team)
	now = now.Add(time.Minute)
	reconcile(0)
	var reason string
	if err := db.QueryRow("select reason from task_trigger_records where task_id=$1 and outcome='error' order by id desc limit 1", good).Scan(&reason); err != nil || reason != "TASK_TRIGGER_DELEGATE_ACCESS_DENIED" {
		t.Fatalf("revocation %s %v", reason, err)
	}
}

func TestCronValidatesEveryFieldAndSundayRanges(t *testing.T) {
	moment := time.Date(2026, 9, 13, 8, 5, 0, 0, time.UTC)
	for _, expression := range []string{"0 0 * * 99", "5,invalid * * * *", "5 8 * * 7-8"} {
		if _, err := CronMatches(expression, moment); err == nil {
			t.Fatalf("accepted %s", expression)
		}
	}
	for _, expression := range []string{"5 8 * * 7", "5 8 * * 5-7", "0/5 8 * * 0"} {
		if match, err := CronMatches(expression, moment); err != nil || !match {
			t.Fatalf("rejected %s %v", expression, err)
		}
	}
}

func TestScheduleV2SixtySecondBoundary(t *testing.T) {
	for _, late := range []time.Duration{60 * time.Second, 60*time.Second + time.Nanosecond} {
		t.Run(late.String(), func(t *testing.T) {
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
			uid := id("insert into users(name,email,password) values('window','window@example.com','unused') returning id")
			team := id("insert into teams(name) values('window') returning id")
			if _, err := db.Exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid); err != nil {
				t.Fatal(err)
			}
			project := id("insert into projects(team_id,name) values($1,'window') returning id", team)
			occurrence := time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC)
			task := id("insert into tasks(project_id,team_id,name,trigger_type,script,authorized_by_user_id,schedule_evaluated_at) values($1,$2,'window','schedule','typed-task-v2',$3,$4) returning id", project, team, uid, occurrence.Add(-time.Second))
			version := id(`insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version,trigger_json,input_schema_json) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2','{"type":"schedule","cron":"0 8 * * *","timezone":"UTC"}','{"type":"object"}') returning id`, project, team, task)
			if _, err := db.Exec("update tasks set current_published_version_id=$1 where id=$2", version, task); err != nil {
				t.Fatal(err)
			}
			scheduler := NewScheduler(db, func() time.Time { return occurrence.Add(late) }, time.Second, nil)
			count, err := scheduler.ReconcileOnce(ctx)
			want := 0
			if late == 60*time.Second {
				want = 1
			}
			if err != nil || count != want {
				t.Fatalf("created %d want %d at %v: %v", count, want, late, err)
			}
		})
	}
}
