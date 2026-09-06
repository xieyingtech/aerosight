package httpapi

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"aerosight/server/internal/agent"
)

func TestQueuedJobDatabaseReauthorization(t *testing.T) {
	for _, tc := range []struct {
		name, permission, change string
		allowed                  bool
	}{
		{"current grant", "agent:use", "", true},
		{"revoked grant", "agent:use", "delete from project_permissions", false},
		{"removed member", "agent:use", "delete from team_members", false},
		{"closed session", "agent:use", "update agent_sessions set status='closed'", false},
		{"missing session user", "agent:use", "update agent_sessions set started_by_user_id=null", false},
		{"expired context", "agent:use", "update agent_tool_jobs set created_at=now()-interval '2 hours',context_expires_at=now()-interval '1 hour'", false},
		{"event alias", "issue:handle", "", true},
		{"alias does not assign", "issue:assign", "", false},
		{"alias does not approve", "mission:approve", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAPIFixture(t)
			team, project := f.project(t)
			var uid, session int
			if err := f.db.QueryRow("select id from users where email='admin@example.com'").Scan(&uid); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec("update team_members set role='member' where team_id=$1 and user_id=$2", team, uid); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) values($1,$2,$3,'agent:use'),($1,$2,$3,'event:handle')", project, team, uid); err != nil {
				t.Fatal(err)
			}
			if err := f.db.QueryRow("insert into agent_sessions(project_id,started_by_user_id) values($1,$2) returning id", project, uid).Scan(&session); err != nil {
				t.Fatal(err)
			}
			var job string
			if err := f.db.QueryRow("insert into agent_tool_jobs(project_id,team_id,session_id,requested_by_user_id,tool_name,required_permission,context_expires_at) values($1,$2,$3,$4,'issue_copilot',$5,now()+interval '1 hour') returning id", project, team, session, uid, tc.permission).Scan(&job); err != nil {
				t.Fatal(err)
			}
			if tc.change != "" {
				if _, err := f.db.Exec(tc.change); err != nil {
					t.Fatal(err)
				}
			}
			ctx := context.Background()
			if tc.allowed {
				tx, err := f.db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				if err := agent.RevalidateQueuedJob(ctx, tx, job, time.Now()); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
			} else {
				// Exercise the real worker claim path: denied jobs must finalize,
				// without an upstream call, and must not poison subsequent polling.
				processor := agent.JobProcessor{Database: f.db}
				worked, err := processor.ProcessNext(ctx)
				if err != nil || !worked {
					t.Fatalf("worker claim: worked=%v err=%v", worked, err)
				}
				worked, err = processor.ProcessNext(ctx)
				if err != nil || worked {
					t.Fatalf("denied job reclaimed: worked=%v err=%v", worked, err)
				}
			}
			var status string
			var failure sql.NullString
			var started, finished, checked sql.NullTime
			if err := f.db.QueryRow("select status,failure_code,started_at,finished_at,authorization_checked_at from agent_tool_jobs where id=$1", job).Scan(&status, &failure, &started, &finished, &checked); err != nil {
				t.Fatal(err)
			}
			if !checked.Valid {
				t.Fatal("authorization check not recorded")
			}
			if tc.allowed {
				if status != "running" || !started.Valid || finished.Valid || failure.Valid {
					t.Fatalf("allowed state: %s %+v", status, failure)
				}
			} else if status != "failed" || failure.String != "AUTHORIZATION_REVALIDATION_FAILED" || started.Valid || !finished.Valid {
				t.Fatalf("denied state: %s %+v started=%v finished=%v", status, failure, started, finished)
			}
		})
	}
}
