package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func algorithmDefinitionBody(provider int64) string {
	return fmt.Sprintf(`{"definition":{"providerId":%d,"name":" OCR ","capabilityCode":"perception.ocr"},"configuration":{"executionMode":"synchronous","modelOrProcess":" ocr-v2 ","inputSchema":{"type":"object"},"parametersSchema":{},"outputSchema":{},"protocolConfig":{},"outputMapping":{"kind":"ocr"}}}`, provider)
}

func TestAlgorithmDefinitionInput(t *testing.T) {
	body := algorithmDefinitionBody(4)
	var raw map[string]any
	json.Unmarshal([]byte(body), &raw)
	input, err := parseAlgorithmDefinition(raw)
	if err != nil || input.Name != "OCR" || input.ProviderID != 4 || input.Configuration["modelOrProcess"] != "ocr-v2" || input.Configuration["publishThreshold"] != float64(0) || input.Description.Valid {
		t.Fatalf("input %+v %v", input, err)
	}
	for _, body := range []string{strings.Replace(body, `"type":"object"`, `"type":7`, 1), strings.Replace(body, `"protocolConfig":{}`, `"protocolConfig":null`, 1), strings.Replace(body, `"outputSchema":{}`, `"outputSchema":{},"publishThreshold":2`, 1), strings.Replace(body, `"name":" OCR "`, `"name":""`, 1), strings.Replace(body, `perception.ocr`, `Perception.ocr`, 1), strings.Replace(body, `"name":" OCR "`, `"name":"OCR","unknown":true`, 1)} {
		var raw map[string]any
		json.Unmarshal([]byte(body), &raw)
		if _, err := parseAlgorithmDefinition(raw); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	var legacy map[string]any
	json.Unmarshal([]byte(strings.Replace(body, `"configuration":`, `"version":`, 1)), &legacy)
	if _, err := parseAlgorithmDefinition(legacy); err != nil {
		t.Fatalf("legacy configuration %v", err)
	}
}

func TestAlgorithmDefinitionSnapshots(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	var provider int64
	if err := f.db.QueryRow("insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status) values($1,$2,'Provider','http-json','https://algorithm.example','active') returning id", pid, team).Scan(&provider); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("/api/projects/%d/algorithm-definitions", pid)
	body := algorithmDefinitionBody(provider)
	call := func(method, path, body string, status int) map[string]any {
		t.Helper()
		res := f.request(t, method, path, body)
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("%d want %d %+v", res.StatusCode, status, data)
		}
		return data
	}
	count := func(query string, want int) {
		t.Helper()
		var n int
		if err := f.db.QueryRow(query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Fatalf("%s = %d want %d", query, n, want)
		}
	}
	data := call("GET", base, "", 200)
	if len(data["definitions"].([]any)) != 0 {
		t.Fatalf("empty %+v", data)
	}
	data = call("POST", base, body, 201)
	id := data["definitionId"].(string)
	first := data["configurationSnapshotId"].(string)
	data = call("GET", base, "", 200)
	entry := data["definitions"].([]any)[0].(map[string]any)
	if entry["id"] != id || entry["configurationSnapshotId"] != first || entry["name"] != "OCR" || entry["description"] != nil || entry["provider"].(map[string]any)["available"] != true {
		t.Fatalf("catalog %+v", entry)
	}
	item := base + "/" + id
	data = call("PUT", item, body, 200)
	numericID, _ := strconv.ParseFloat(id, 64)
	if data["definitionId"] != numericID || data["configurationSnapshotId"] == first {
		t.Fatalf("update %+v", data)
	}
	count("select count(*) from algorithm_definition_versions where status='published'", 1)
	count("select count(*) from algorithm_definition_versions where status='retired'", 1)
	if _, err := f.db.Exec(`create function fail_algorithm_snapshot() returns trigger language plpgsql as $$ begin raise exception 'private database detail'; end $$; create trigger fail_algorithm_snapshot before insert on algorithm_definition_versions for each row execute function fail_algorithm_snapshot()`); err != nil {
		t.Fatal(err)
	}
	failed := call("PUT", item, strings.Replace(body, " OCR ", "Changed", 1), 400)
	if failed["error"] != "ALGORITHM_DEFINITION_FAILED" {
		t.Fatalf("leak %+v", failed)
	}
	call("POST", base, strings.Replace(body, " OCR ", "New", 1), 400)
	count("select count(*) from algorithm_definitions where name='OCR'", 1)
	count("select count(*) from algorithm_definitions", 1)
	count("select count(*) from algorithm_definition_versions where status='published' and version=2", 1)
	count("select count(*) from audit_events where action='algorithm_definition.save'", 2)
	if _, err := f.db.Exec("drop trigger fail_algorithm_snapshot on algorithm_definition_versions"); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 4)
	for range 4 {
		go func() {
			req, err := http.NewRequest("PUT", f.host.URL+item, strings.NewReader(body))
			if err != nil {
				results <- err
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://frontend.test")
			req.Header.Set("X-CSRF-Token", f.csrf)
			res, err := f.client.Do(req)
			if err != nil {
				results <- err
				return
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				results <- fmt.Errorf("status %d", res.StatusCode)
				return
			}
			results <- nil
		}()
	}
	for range 4 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	count("select count(distinct version) from algorithm_definition_versions", 6)
	count("select count(*) from algorithm_definition_versions where status='published' and version=6", 1)
	count("select count(*) from algorithm_definitions d join algorithm_definition_versions v on d.current_published_version_id=v.id where v.version=6", 1)
	_, other := f.project(t)
	call("POST", fmt.Sprintf("/api/projects/%d/algorithm-definitions", other), body, 400)
	data = call("PUT", base+"/2147483647", body, 400)
	if data["error"] != "ALGORITHM_DEFINITION_NOT_FOUND" {
		t.Fatalf("missing %+v", data)
	}
	if _, err := f.db.Exec("update algorithm_providers set status='disabled' where id=$1", provider); err != nil {
		t.Fatal(err)
	}
	data = call("GET", base, "", 200)
	if data["definitions"].([]any)[0].(map[string]any)["provider"].(map[string]any)["available"] != false {
		t.Fatalf("disabled %+v", data)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	call("PUT", item, body, 403)
	call("POST", base, body, 403)
	call("GET", base, "", 200)
	if _, err := f.db.Exec("delete from team_members where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	call("GET", base, "", 404)
}
