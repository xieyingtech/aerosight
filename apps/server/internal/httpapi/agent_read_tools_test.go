package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestChatToolInputAndFormatting(t *testing.T) {
	if _, err := parseChatToolInput("query_assets", []byte(`{"":true}`)); err == nil {
		t.Fatal("empty unknown property accepted")
	}
	for _, test := range []struct{ name, raw string }{{"query_devices", `{"deviceIds":[3]}`}, {"query_tasks", `{"limit":1}`}, {"query_issues", `{"limit":20}`}, {"query_assets", `{}`}, {"query_tracks", `{}`}, {"query_map_context", `{}`}} {
		if _, err := parseChatToolInput(test.name, []byte(test.raw)); err != nil {
			t.Fatalf("%s %v", test.name, err)
		}
	}
	for _, raw := range []string{`null`, `[]`, `{"limit":0}`, `{"limit":1.5}`, `{"limit":101}`, `{"window":{"userId":999}}`, `{"projectId":2}`, `{"limit":null}`, `{"other":true}`} {
		if _, err := parseChatToolInput("query_issues", []byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if _, err := parseChatToolInput("device_command", []byte(`{}`)); err == nil || err.Error() != "AGENT_TOOL_NOT_READ_ONLY" {
		t.Fatalf("write tool %v", err)
	}
	now := time.Date(2026, 8, 27, 0, 1, 30, 0, time.UTC)
	rows := make([]gin.H, 105)
	for i := range rows {
		rows[i] = gin.H{"id": float64(12345678 + i), "observedAt": "2026-08-27T00:00:00.000Z", "quality": "usable"}
	}
	result, err := formatChatToolResult(17, "query_devices", rows, 100, now)
	if err != nil || result["truncated"] != true || result["freshnessSeconds"] != int64(90) || len(result["items"].([]gin.H)) != 100 {
		t.Fatalf("format %+v %v", result, err)
	}
	ref := result["items"].([]gin.H)[0]["reference"].(gin.H)
	if ref["id"] != "12345678" || ref["href"] != "/projects/devices/?projectId=17&selected=12345678" {
		t.Fatalf("ref %+v", ref)
	}
	for _, name := range []string{"query_tasks", "query_issues", "query_assets", "query_tracks", "query_map_context"} {
		ref := chatEvidenceReference(17, name, "a&b")
		u, err := url.Parse(ref["href"].(string))
		if err != nil || u.Query().Get("projectId") != "17" || strings.Contains(u.RawQuery, "a&b") {
			t.Fatalf("URL %+v %v", ref, err)
		}
	}
	result, err = formatChatToolResult(17, "query_assets", []gin.H{{"id": 1, "content": strings.Repeat("x", 70*1024)}}, 100, now)
	if err != nil || result["truncated"] != true || len(result["items"].([]gin.H)) != 0 || result["freshnessSeconds"] != nil {
		t.Fatalf("byte cap %+v %v", result, err)
	}
}

func TestChatReadToolsProjectScope(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	otherTeam, otherPID := f.project(t)
	var uid int32
	if err := f.db.QueryRow("select id from users where email='admin@example.com'").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	query := func(name, input string) gin.H {
		t.Helper()
		result, err := f.server.executeChatReadTool(context.Background(), uid, int32(pid), name, json.RawMessage(input))
		if err != nil {
			t.Fatalf("%s %v", name, err)
		}
		return result
	}
	for _, name := range []string{"query_devices", "query_tasks", "query_issues", "query_assets", "query_tracks"} {
		if r := query(name, `{}`); len(r["items"].([]gin.H)) != 0 || r["freshnessSeconds"] != nil {
			t.Fatalf("empty %s %+v", name, r)
		}
	}
	ownDevice := 0
	for _, scope := range []struct{ team, pid int }{{team, pid}, {otherTeam, otherPID}} {
		adapter, device := f.device(t, scope.team, scope.pid)
		if scope.pid == pid {
			ownDevice = device
		}
		f.missionRun(t, scope.pid, scope.team, "running")
		if _, err := f.db.Exec("insert into issues(project_id,number,title,source_type) values($1,1,'Tool issue','manual'),($1,2,'Second issue','manual')", scope.pid); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec("insert into assets(project_id,team_id,kind,storage_key,logical_key,status) values($1,$2,'image','secret/key','asset','available'),($1,$2,'image','secret/pending','pending','pending')", scope.pid, scope.team); err != nil {
			t.Fatal(err)
		}
		for n := 0; n < 2; n++ {
			var observation int
			if err := f.db.QueryRow("insert into observations(project_id,team_id,adapter_id,device_id,observation_type,source_event_id,captured_at,received_at) values($1,$2,$3,$4,'pose',$5,now(),now()) returning id", scope.pid, scope.team, adapter, device, fmt.Sprintf("tool-pose-%d", n)).Scan(&observation); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec("insert into poses(observation_id,project_id,device_id,captured_at,standard_position) values($1,$2,$3,now()+$4*interval '1 second',ST_SetSRID(ST_MakePoint($5,30,50),4326))", observation, scope.pid, device, n, 120+float64(n)/10); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range []string{"query_devices", "query_tasks", "query_assets", "query_tracks"} {
		r := query(name, `{}`)
		items := r["items"].([]gin.H)
		if len(items) != 1 {
			t.Fatalf("%s %+v", name, r)
		}
		encoded, _ := json.Marshal(r)
		if strings.Contains(string(encoded), "secret/key") {
			t.Fatal("storage key leaked")
		}
	}
	tracks := query("query_tracks", `{}`)["items"].([]gin.H)
	geometry := tracks[0]["geometry"].(map[string]any)
	if tracks[0]["pointCount"] != float64(2) || geometry["type"] != "LineString" || len(geometry["coordinates"].([]any)) != 2 {
		t.Fatalf("track %+v", tracks)
	}
	if r := query("query_devices", fmt.Sprintf(`{"deviceIds":[%d]}`, ownDevice)); len(r["items"].([]gin.H)) != 1 {
		t.Fatalf("filter %+v", r)
	}
	if r := query("query_devices", `{"deviceIds":[]}`); len(r["items"].([]gin.H)) != 0 {
		t.Fatalf("empty filter %+v", r)
	}
	if r := query("query_issues", `{"limit":1}`); len(r["items"].([]gin.H)) != 1 || r["truncated"] != true {
		t.Fatalf("issues limit %+v", r)
	}
	counts := query("query_map_context", `{}`)["items"].([]gin.H)[0]
	if counts["deviceCount"] != float64(1) || counts["openIssueCount"] != float64(2) || counts["activeMissionCount"] != float64(1) {
		t.Fatalf("counts %+v", counts)
	}
	if _, err := f.server.executeChatReadTool(context.Background(), uid, int32(pid), "query_assets", []byte(`{"nested":[{"project_id":999}]}`)); err == nil || !strings.Contains(err.Error(), "AGENT_TOOL_SCOPE_ARGUMENT_FORBIDDEN") {
		t.Fatalf("scope injection %v", err)
	}
	if _, err := f.db.Exec("delete from team_members where team_id=$1 and user_id=$2", team, uid); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"query_devices", "query_tasks", "query_issues", "query_assets", "query_tracks", "query_map_context"} {
		if _, err := f.server.executeChatReadTool(context.Background(), uid, int32(pid), name, []byte(`{}`)); err == nil || err.Error() != "PROJECT_ACCESS_DENIED" {
			t.Fatalf("revoked %s %v", name, err)
		}
	}
}
