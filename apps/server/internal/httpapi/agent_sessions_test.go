package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAgentSessions(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	base := fmt.Sprintf("/api/projects/%d/agent-sessions", pid)
	var uid, other int32
	if err := f.db.QueryRow("select id from users where email='admin@example.com'").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into users(name,email,role) values('Other','other-chat@example.com','user') returning id").Scan(&other); err != nil {
		t.Fatal(err)
	}
	list := func() []map[string]any {
		t.Helper()
		res := f.request(t, "GET", base, "")
		defer res.Body.Close()
		var rows []map[string]any
		if err := json.NewDecoder(res.Body).Decode(&rows); err != nil || res.StatusCode != 200 {
			t.Fatalf("list %d %v", res.StatusCode, err)
		}
		return rows
	}
	if rows := list(); rows == nil || len(rows) != 0 {
		t.Fatalf("empty %+v", rows)
	}
	res := f.request(t, "POST", base, "")
	created := decodedResponse(t, res)
	if res.StatusCode != 201 {
		t.Fatalf("create %+v", created)
	}
	sid := int32(created["id"].(float64))
	rows := list()
	if len(rows) != 1 || rows[0]["summary"] != nil || rows[0]["status"] != "open" || len(rows[0]["messages"].([]any)) != 0 {
		t.Fatalf("session %+v", rows)
	}
	ctx := context.Background()
	appendMessage := func(user, project, session int32, role, content string) error {
		_, err := f.server.appendAgentMessage(ctx, user, project, session, role, content, nil, "chat-test")
		return err
	}
	if err := appendMessage(uid, int32(pid), sid, "user", "Bearer private-token"); err != nil {
		t.Fatal(err)
	}
	if err := appendMessage(uid, int32(pid), sid, "assistant", "回答"); err != nil {
		t.Fatal(err)
	}
	rows = list()
	messages := rows[0]["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages %+v", messages)
	}
	first := messages[0].(map[string]any)
	if first["content"] != "[authorization-redacted]" || first["sessionId"] != float64(sid) || len(first["toolCalls"].([]any)) != 0 {
		t.Fatalf("message %+v", first)
	}
	// A different user is a project member with explicit agent permission, yet
	// cannot append to or retrieve this user's session history.
	if _, err := f.db.Exec("insert into team_members(team_id,user_id,role) values($1,$2,'member');", team, other); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) values($1,$2,$3,'agent:use')", pid, team, other); err != nil {
		t.Fatal(err)
	}
	if err := appendMessage(other, int32(pid), sid, "user", "attack"); err == nil || err.Error() != "AGENT_SESSION_NOT_FOUND" {
		t.Fatalf("cross-user %v", err)
	}
	_, otherPID := f.project(t)
	if err := appendMessage(uid, int32(otherPID), sid, "user", "attack"); err == nil || err.Error() != "AGENT_SESSION_NOT_FOUND" {
		t.Fatalf("cross-project %v", err)
	}
	for i := 0; i < 23; i++ {
		if err := appendMessage(uid, int32(pid), sid, "user", fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.db.Exec("insert into agent_messages(session_id,role,content) values($1,'tool','excluded')", sid); err != nil {
		t.Fatal(err)
	}
	history, err := f.server.queries.RecentChatHistory(ctx, sqlcgen.RecentChatHistoryParams{ID: sid, ProjectID: int32(pid), StartedByUserID: sql.NullInt32{Int32: uid, Valid: true}})
	if err != nil || len(history) != 20 || history[0].Content != "3" || history[19].Content != "22" {
		t.Fatalf("history %+v %v", history, err)
	}
	hidden, err := f.server.queries.RecentChatHistory(ctx, sqlcgen.RecentChatHistoryParams{ID: sid, ProjectID: int32(pid), StartedByUserID: sql.NullInt32{Int32: other, Valid: true}})
	if err != nil || len(hidden) != 0 {
		t.Fatalf("hidden %+v %v", hidden, err)
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := f.db.QueryRow("select count(*) from " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	beforeMessages, beforeAudits := count("agent_messages"), count("audit_events")
	if _, err := f.db.Exec(`create function fail_chat_append() returns trigger language plpgsql as $$ begin raise exception 'private failure'; end $$; create trigger fail_chat_append before insert on agent_messages for each row execute function fail_chat_append()`); err != nil {
		t.Fatal(err)
	}
	if err := appendMessage(uid, int32(pid), sid, "assistant", "rollback"); err == nil {
		t.Fatal("expected append failure")
	}
	if count("agent_messages") != beforeMessages || count("audit_events") != beforeAudits {
		t.Fatal("failed append not atomic")
	}
	if _, err := f.db.Exec("drop trigger fail_chat_append on agent_messages"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("update agent_sessions set status='closed' where id=$1", sid); err != nil {
		t.Fatal(err)
	}
	if err := appendMessage(uid, int32(pid), sid, "assistant", "closed"); err == nil || err.Error() != "AGENT_SESSION_NOT_FOUND" {
		t.Fatalf("closed %v", err)
	}
	if _, err := f.db.Exec("insert into agent_sessions(project_id,started_by_user_id,summary) select $1,$2,'owned' from generate_series(1,52)", pid, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into agent_sessions(project_id,started_by_user_id,summary) values($1,$2,'hidden')", pid, other); err != nil {
		t.Fatal(err)
	}
	rows = list()
	if len(rows) != 50 {
		t.Fatalf("list limit %d", len(rows))
	}
	for _, row := range rows {
		if row["summary"] != "owned" {
			t.Fatalf("session scope/order %+v", row)
		}
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1 and user_id=$2", team, uid); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "POST"} {
		res = f.request(t, method, base, "")
		denied := decodedResponse(t, res)
		if res.StatusCode != 403 {
			t.Fatalf("denied %d %+v", res.StatusCode, denied)
		}
	}
	if err := appendMessage(uid, int32(pid), sid, "assistant", "revoked"); err == nil || !strings.Contains(err.Error(), "PROJECT_ACCESS_DENIED") {
		t.Fatalf("revoked %v", err)
	}
}
