package database

import (
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/text/language"
	"os"
	"strings"
	"testing"
)

func TestLegacyAuditHashFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../../../contracts/go-migration/audit-hashes.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Input  any    `json:"input"`
		Hash   string `json:"hash"`
		Locale string `json:"locale"`
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for i, f := range fixtures {
		hash, err := auditHashLocale(f.Input, language.Make(f.Locale))
		if err != nil || hash != f.Hash {
			t.Errorf("fixture %d got %s want %s (%v)", i, hash, f.Hash, err)
		}
		if matches, err := matchesAuditHash(f.Input, f.Hash); err != nil || !matches {
			t.Errorf("fixture %d old locale rejected: %v", i, err)
		}
	}
}

func TestAuditedIdempotentWriteAtomicity(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	var uid, team, pid int32
	if err := db.QueryRow("insert into users(name,email) values('audit test','audit@test.local') returning id").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("insert into teams(name) values('audit test') returning id").Scan(&team); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("insert into projects(team_id,name) values($1,'before') returning id", team).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid); err != nil {
		t.Fatal(err)
	}
	authorize := func(ctx context.Context, w *WriteTx) error {
		var exists bool
		return w.Tx.QueryRowContext(ctx, "select true from team_members where user_id=$1 and team_id=$2 for share", uid, team).Scan(&exists)
	}
	audit := AuditContext{ProjectID: pid, TeamID: team, ActorUserID: uid, RequestID: "atomic-test", Action: "test.write", ResourceType: "project", Input: map[string]any{"name": "after", "é": 1, "e": 2}}
	run := func(stage string) (IdempotentResult[map[string]any], error) {
		return AuditedWrite(ctx, db, audit, authorize, func(w *WriteTx) (IdempotentResult[map[string]any], error) {
			return Idempotent(ctx, w, IdempotencyContext{ProjectID: pid, TeamID: team, ActorKey: fmt.Sprintf("user:%d", uid), Operation: "test.write", Key: "same-key", Request: audit.Input}, func() (map[string]any, error) {
				if _, err := w.Tx.ExecContext(ctx, "update projects set name='after' where id=$1", pid); err != nil {
					return nil, err
				}
				if stage == "business" {
					return nil, errors.New("injected business failure")
				}
				if _, err := w.Publish(ctx, ProjectEvent{ProjectID: pid, TeamID: team, EventID: "atomic-event", EventType: "test.updated", Payload: map[string]any{"projectId": pid}}); err != nil {
					return nil, err
				}
				if stage == "event" {
					return nil, errors.New("injected event failure")
				}
				return map[string]any{"ok": true}, nil
			})
		})
	}
	assertEmpty := func() {
		t.Helper()
		var name string
		if err := db.QueryRow("select name from projects where id=$1", pid).Scan(&name); err != nil || name != "before" {
			t.Fatalf("business escaped rollback: %s %v", name, err)
		}
		for _, table := range []string{"audit_events", "idempotency_records", "project_events", "outbox_events"} {
			var count int
			if err := db.QueryRow("select count(*) from "+table+" where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
				t.Fatalf("%s escaped rollback: %d %v", table, count, err)
			}
		}
	}
	for _, stage := range []string{"business", "event"} {
		if _, err := run(stage); err == nil {
			t.Fatalf("%s accepted failure", stage)
		}
		assertEmpty()
	}
	// Real database faults at the queue and final audit update verify SQL errors also roll back everything.
	for _, point := range []struct{ table, when string }{{"outbox_events", "insert"}, {"audit_events", "update"}, {"idempotency_records", "update"}} {
		ddl := fmt.Sprintf("create function injected_failure() returns trigger language plpgsql as $$ begin raise exception 'injected failure'; end $$; create trigger inject_failure before %s on %s for each row execute function injected_failure()", point.when, point.table)
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := run(""); err == nil {
			t.Fatalf("%s failure committed", point.table)
		}
		assertEmpty()
		if _, err := db.Exec("drop trigger inject_failure on " + point.table + "; drop function injected_failure()"); err != nil {
			t.Fatal(err)
		}
	}
	type outcome struct {
		result IdempotentResult[map[string]any]
		err    error
	}
	outcomes := make(chan outcome, 8)
	for i := 0; i < 8; i++ {
		go func() { result, err := run(""); outcomes <- outcome{result, err} }()
	}
	executed := 0
	for i := 0; i < 8; i++ {
		out := <-outcomes
		if out.err != nil || out.result.Value["ok"] != true {
			t.Fatalf("concurrent write %+v %v", out.result, out.err)
		}
		if !out.result.Replayed {
			executed++
		}
	}
	if executed != 1 {
		t.Fatalf("operation executed %d times", executed)
	}
	legacyHash, err := auditHashLocale(audit.Input, language.SimplifiedChinese)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("update idempotency_records set request_hash=$1 where project_id=$2", legacyHash, pid); err != nil {
		t.Fatal(err)
	}
	result, err := run("")
	if err != nil || !result.Replayed {
		t.Fatalf("replay %+v %v", result, err)
	}
	for _, table := range []string{"project_events", "outbox_events", "idempotency_records"} {
		var count int
		if err := db.QueryRow("select count(*) from "+table+" where project_id=$1", pid).Scan(&count); err != nil || count != 1 {
			t.Fatalf("duplicate %s %d %v", table, count, err)
		}
	}
	audit.Input = map[string]any{"name": "changed"}
	if _, err = run(""); err == nil || !strings.Contains(err.Error(), "DIFFERENT_REQUEST") {
		t.Fatalf("key reuse %v", err)
	}
	if _, err = db.Exec("delete from team_members where team_id=$1 and user_id=$2", team, uid); err != nil {
		t.Fatal(err)
	}
	audit.Input = map[string]any{"name": "after", "é": 1, "e": 2}
	if _, err = run(""); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("revoked actor replayed %v", err)
	}
}
