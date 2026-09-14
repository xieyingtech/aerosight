package tasktrigger

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
)

func TestTaskConcurrencyAndReplayAcrossVersions(t *testing.T) {
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
	team := id("insert into teams(name) values('trigger test') returning id")
	uid := id("insert into users(name,email,password) values('trigger','trigger@example.com','unused') returning id")
	if _, err := db.Exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid); err != nil {
		t.Fatal(err)
	}
	project := id("insert into projects(team_id,name) values($1,'trigger test') returning id", team)
	task := id("insert into tasks(project_id,team_id,name,trigger_type,script,authorized_by_user_id) values($1,$2,'trigger test','schedule','',$3) returning id", project, team, uid)
	makeVersion := func(n int) int64 {
		return id("insert into task_versions(project_id,team_id,task_id,version,status,script) values($1,$2,$3,$4,'published','') returning id", project, team, task, n)
	}
	version1 := makeVersion(1)
	setVersion := func(v int64) {
		t.Helper()
		if _, err := db.Exec("update tasks set current_published_version_id=$1 where id=$2", v, task); err != nil {
			t.Fatal(err)
		}
	}
	setVersion(version1)
	item := candidate{ProjectID: int(project), TeamID: int(team), TaskID: int(task), TaskVersionID: version1, ConcurrencyLimit: 1, InputSchemaJSON: []byte(`{"type":"object"}`)}
	type outcome struct {
		run      int
		inserted bool
		err      error
	}
	invoke := func(c candidate, key string) outcome {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return outcome{err: err}
		}
		defer tx.Rollback()
		run, inserted, err := createRun(ctx, tx, c, "schedule", key, map[string]any{"inputs": map[string]any{}})
		if err == nil {
			err = tx.Commit()
		}
		return outcome{run, inserted, err}
	}
	result := make(chan outcome, 2)
	for range 2 {
		go func() { result <- invoke(item, "schedule:first") }()
	}
	a, b := <-result, <-result
	if a.err != nil || b.err != nil || a.run == 0 || a.run != b.run || a.inserted == b.inserted {
		t.Fatalf("duplicate occurrence: %+v %+v", a, b)
	}
	version2 := makeVersion(2)
	setVersion(version2)
	newer := item
	newer.TaskVersionID = version2
	replay := invoke(newer, "schedule:first")
	if replay.err != nil || replay.inserted || replay.run != a.run {
		t.Fatalf("cross-version replay: %+v", replay)
	}
	blocked := invoke(newer, "schedule:second")
	if blocked.err != nil || blocked.inserted || blocked.run != 0 {
		t.Fatalf("new version bypassed active run: %+v", blocked)
	}
	q := sqlcgen.New(db)
	active, err := q.CountActiveTriggeredRuns(ctx, sqlcgen.CountActiveTriggeredRunsParams{ProjectID: int32(project), TaskID: int32(task)})
	if err != nil || active != 1 {
		t.Fatalf("HTTP active count: %d %v", active, err)
	}
	existing, err := q.ReadTriggeredRun(ctx, sqlcgen.ReadTriggeredRunParams{ProjectID: int32(project), TaskID: int32(task), TriggerKey: sql.NullString{String: "schedule:first", Valid: true}})
	if err != nil || int(existing.ID) != a.run {
		t.Fatalf("HTTP replay: %+v %v", existing, err)
	}
	if _, err := db.Exec("update task_runs set status='succeeded' where id=$1", a.run); err != nil {
		t.Fatal(err)
	}
	// A discovered old version cannot submit after another version is published.
	stale := invoke(item, "schedule:stale")
	if stale.err != nil || stale.inserted {
		t.Fatalf("stale candidate submitted: %+v", stale)
	}
	if _, err := db.Exec("update tasks set status='disabled' where id=$1", task); err != nil {
		t.Fatal(err)
	}
	disabled := invoke(newer, "schedule:disabled")
	if disabled.err != nil || disabled.inserted {
		t.Fatalf("disabled candidate submitted: %+v", disabled)
	}
	if _, err := db.Exec("update tasks set status='active' where id=$1", task); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		go func(n int) { result <- invoke(newer, fmt.Sprintf("schedule:race:%d", n)) }(i)
	}
	a, b = <-result, <-result
	if a.err != nil || b.err != nil || a.inserted == b.inserted {
		t.Fatalf("concurrent occurrence limit: %+v %+v", a, b)
	}
}
