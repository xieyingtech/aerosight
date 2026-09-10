package httpapi

import (
	"aerosight/server/internal/algorithm"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAlgorithmRunInputAndDiagnostics(t *testing.T) {
	for _, body := range []string{`{"configurationSnapshotId":"31","assetId":9}`, `{"definitionVersionId":31,"assetId":"9","parameters":{}}`, `{"configurationSnapshotId":31,"definitionVersionId":30,"assetId":9}`} {
		var raw map[string]any
		json.Unmarshal([]byte(body), &raw)
		input, err := parseAlgorithmRunInput(raw)
		if err != nil || input.SnapshotID != 31 || input.AssetID != 9 || input.Parameters == nil {
			t.Fatalf("%s %+v %v", body, input, err)
		}
	}
	for _, body := range []string{`{"assetId":9}`, `{"configurationSnapshotId":0,"assetId":9}`, `{"configurationSnapshotId":31,"assetId":1.5}`, `{"configurationSnapshotId":31,"assetId":9,"parameters":null}`, `{"configurationSnapshotId":31,"assetId":9,"extra":true}`, `{"configurationSnapshotId":31,"definitionVersionId":0,"assetId":9}`} {
		var raw map[string]any
		json.Unmarshal([]byte(body), &raw)
		if _, err := parseAlgorithmRunInput(raw); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	run := gin.H{"status": "failed", "inputSnapshot": map[string]any{"inputAsset": map[string]any{"assetId": float64(41), "version": float64(3), "accessUrl": "signed-secret"}, "definition": map[string]any{"definitionVersionId": float64(8)}, "callback": map[string]any{"token": "callback-secret"}}, "canonicalResult": map[string]any{"mappingDiagnostics": []any{"mapping failed", 9}}, "startedAt": "2026-08-27T01:00:01Z", "finishedAt": "2026-08-27T01:00:04Z"}
	view := algorithmRunDiagnostics(run, map[string]bool{"algorithm:manage": true}, time.Now())
	encoded, _ := json.Marshal(view)
	if view["durationMs"] != int64(3000) || view["retryAllowed"] != true || strings.Contains(string(encoded), "secret") {
		t.Fatalf("view %s", encoded)
	}
	if algorithmRunDiagnostics(run, map[string]bool{}, time.Now())["retryAllowed"] != false {
		t.Fatal("viewer retry")
	}
}

func TestAlgorithmRunHTTPTransactions(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	var provider, definition, version int64
	var asset int
	if err := f.db.QueryRow("insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status) values($1,$2,'Provider','http-json','https://algorithm.example','active') returning id", pid, team).Scan(&provider); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into algorithm_definitions(project_id,team_id,provider_id,name,capability_code) values($1,$2,$3,'OCR','ocr') returning id", pid, team, provider).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process) values($1,$2,$3,1,'published','synchronous','ocr-v1') returning id", pid, team, definition).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("update algorithm_definitions set current_published_version_id=$2 where id=$1", definition, version); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into assets(project_id,team_id,kind,storage_key,logical_key,checksum_sha256,mime_type) values($1,$2,'image','ocr.jpg','ocr.jpg',$3,'image/jpeg') returning id", pid, team, strings.Repeat("a", 64)).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("/api/projects/%d/algorithm-runs", pid)
	body := fmt.Sprintf(`{"configurationSnapshotId":%d,"assetId":%d,"parameters":{"language":"zh-CN"}}`, version, asset)
	count := func(query string, want int) {
		t.Helper()
		var got int
		if err := f.db.QueryRow(query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s = %d want %d", query, got, want)
		}
	}
	call := func(path, body string, status int) map[string]any {
		t.Helper()
		res := f.request(t, "POST", path, body)
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("status %d want %d %+v", res.StatusCode, status, data)
		}
		return data
	}
	data := call(base, body, 202)
	id := data["runId"].(string)
	var raw []byte
	if err := f.db.QueryRow("select input_snapshot_json from algorithm_runs where id=$1", id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var input algorithm.Input
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	if input.RunID != id || input.ProjectID != pid || input.InputAsset.AssetID != asset || input.Definition.ConfigurationSnapshotID != version || input.Parameters["language"] != "zh-CN" {
		t.Fatalf("worker input %+v", input)
	}
	count("select count(*) from outbox_events where event_type='algorithm.run.requested'", 1)
	call(base+"/"+id+"/retry", "", 409)
	// Private snapshot fields previously stayed server-side. The new read APIs
	// must deliver only the browser view, even with a populated callback snapshot.
	if _, err := f.db.Exec(`update algorithm_runs set status='failed',started_at='2026-08-27T01:00:01Z',finished_at='2026-08-27T01:00:04Z',input_snapshot_json=jsonb_set(input_snapshot_json,'{callback}','{"token":"callback-secret"}') where id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`insert into algorithm_run_attempts(project_id,team_id,algorithm_run_id,attempt,status,request_hash,response_status,duration_ms) values($1,$2,$3,1,'failed','hash',502,3000)`, pid, team, id); err != nil {
		t.Fatal(err)
	}
	res := f.request(t, "GET", base+"/"+id, "")
	detail := decodedResponse(t, res)
	encoded, _ := json.Marshal(detail)
	if res.StatusCode != 200 || strings.Contains(string(encoded), "callback-secret") || detail["view"].(map[string]any)["durationMs"] != float64(3000) || len(detail["attempts"].([]any)) != 1 {
		t.Fatalf("detail %d %s", res.StatusCode, encoded)
	}
	res = f.request(t, "GET", base, "")
	var rows []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 || len(rows) != 1 || rows[0]["inputSnapshot"] != nil {
		t.Fatalf("list %d %+v", res.StatusCode, rows)
	}
	retry := call(base+"/"+id+"/retry", "", 200)["runId"].(string)
	if err := f.db.QueryRow("select input_snapshot_json from algorithm_runs where id=$1", retry).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	input = algorithm.Input{}
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	if input.RunID != retry || input.ProjectID != pid || input.InputAsset.AssetID != asset || input.Definition.ConfigurationSnapshotID != version || input.Callback != nil || input.InputAsset.AccessURL != "" {
		t.Fatalf("retry worker input %+v", input)
	}
	count("select count(*) from algorithm_runs", 2)
	// A failure publishing the work must roll back the inserted run and audit.
	if _, err := f.db.Exec(`create function fail_algorithm_outbox() returns trigger language plpgsql as $$ begin raise exception 'private database detail'; end $$; create trigger fail_algorithm_outbox before insert on outbox_events for each row execute function fail_algorithm_outbox()`); err != nil {
		t.Fatal(err)
	}
	call(base, body, 400)
	call(base+"/"+id+"/retry", "", 409)
	count("select count(*) from algorithm_runs", 2)
	count("select count(*) from audit_events where action like 'algorithm_run.%'", 2)
	if _, err := f.db.Exec("drop trigger fail_algorithm_outbox on outbox_events"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("update algorithm_providers set status='disabled' where id=$1", provider); err != nil {
		t.Fatal(err)
	}
	data = call(base, body, 400)
	if data["error"] != "ALGORITHM_RUN_SOURCE_NOT_AVAILABLE" {
		t.Fatalf("disabled %+v", data)
	}
	// Retry pins the old snapshot and defers provider availability to the worker.
	call(base+"/"+id+"/retry", "", 200)
	if _, err := f.db.Exec("update algorithm_providers set status='active' where id=$1", provider); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("update assets set checksum_sha256=null,checksum=null where id=$1", asset); err != nil {
		t.Fatal(err)
	}
	data = call(base, body, 400)
	if data["error"] != "ALGORITHM_INPUT_ASSET_CHECKSUM_REQUIRED" {
		t.Fatalf("checksum %+v", data)
	}
	_, other := f.project(t)
	call(fmt.Sprintf("/api/projects/%d/algorithm-runs", other), body, 400)
	call(fmt.Sprintf("/api/projects/%d/algorithm-runs/%s/retry", other, id), "", 409)
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	call(base, body, 403)
	call(base+"/"+id+"/retry", "", 403)
	res = f.request(t, "GET", base+"/"+id, "")
	detail = decodedResponse(t, res)
	if detail["view"].(map[string]any)["retryAllowed"] != false {
		t.Fatalf("viewer %+v", detail)
	}
	if _, err := f.db.Exec("delete from team_members where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", base+"/"+id, "")
	detail = decodedResponse(t, res)
	if res.StatusCode != 404 {
		t.Fatalf("revoked %d %+v", res.StatusCode, detail)
	}
}
