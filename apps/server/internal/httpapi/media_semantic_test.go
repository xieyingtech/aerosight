package httpapi

import (
	"aerosight/server/internal/semantic"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

func TestMediaSearchInputRejectsScopeAndUntrustedParameters(t *testing.T) {
	for _, raw := range []string{`null`, `{"query":""}`, `{"query":"x","limit":0}`, `{"query":"x","limit":null}`, `{"query":"x","start":null}`, `{"query":"x","limit":0,"projectId":1}`, `{"query":"x","projectId":2}`, `{"query":"x","window":{"team_id":1}}`, `{"query":"x","limit":21}`, `{"query":"x","start":"2026-10-09T00:00:00Z"}`, `{"query":"x"} {}`} {
		if _, err := parseMediaSearch(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if _, err := parseMediaSearch([]byte(`{"query":"林间石阶","start":"2026-10-09T00:00:00Z","end":"2026-10-10T00:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range chatTools() {
		if tool.OfFunction != nil && tool.OfFunction.Name == "search_media" {
			found = true
		}
	}
	if !found {
		t.Fatal("agent tool missing")
	}
}

func TestMediaSearchAgentEvidenceAndRevokedAccess(t *testing.T) {
	f, team, pid, uid, _, path := newChatFixture(t)
	var aid int32
	checksum := strings.Repeat("a", 64)
	if err := f.db.QueryRow(`INSERT INTO assets(project_id,team_id,kind,mime_type,storage_key,logical_key,checksum_sha256) VALUES($1,$2,'video','video/mp4',$3,$3,$4) RETURNING id`, pid, team, fmt.Sprintf("projects/%d/test.mp4", pid), checksum).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	cfg := semantic.Config{Model: "e5", Revision: "rev", Dimension: 2}
	var jobID int64
	if err := f.db.QueryRow(`INSERT INTO media_index_jobs(project_id,asset_id,source_version,source_checksum,space,state) VALUES($1,$2,1,$3,$4,'indexed') RETURNING id`, pid, aid, checksum, cfg.Space()).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	segment := uuid.NewString()
	if _, err := f.db.Exec(`INSERT INTO media_index_segments(id,job_id,project_id,asset_id,start_ms,end_ms,description,time_quality) VALUES($1,$2,$3,$4,10000,20000,'林间石阶','unknown')`, segment, jobID, pid, aid); err != nil {
		t.Fatal(err)
	}
	revoke := false
	index := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/embeddings" {
			w.Write([]byte(`{"model":"e5","revision":"rev","data":[{"index":0,"embedding":[1,0]}]}`))
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		must := body["filter"].(map[string]any)["must"].([]any)
		if must[0].(map[string]any)["match"].(map[string]any)["value"] != float64(pid) {
			t.Error("project filter missing")
		}
		if revoke {
			if _, err := f.db.Exec(`DELETE FROM team_members WHERE team_id=$1 AND user_id=$2`, team, uid); err != nil {
				t.Error(err)
			}
		}
		json.NewEncoder(w).Encode(gin.H{"result": gin.H{"points": []any{gin.H{"id": segment, "score": .9}, gin.H{"id": uuid.NewString(), "score": 1}}}})
	}))
	defer index.Close()
	cfg.QdrantURL = index.URL
	cfg.EmbeddingURL = index.URL
	f.server.AttachSemanticIndex(&semantic.Service{DB: f.db, Client: semantic.Client{Config: cfg}})
	res := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/media-search", pid), `{"query":"石阶"}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 200 || len(data["items"].([]any)) != 1 {
		t.Fatalf("search %+v", data)
	}
	ref := data["items"].([]any)[0].(map[string]any)["reference"].(map[string]any)
	if ref["href"] != fmt.Sprintf("/projects/%d/assets/%d/?startMs=10000&endMs=20000", pid, aid) {
		t.Fatalf("evidence %+v", ref)
	}
	calls := 0
	f.server.networkResolver = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	f.server.aiHTTPClientFactory = func(*url.URL, []netip.Addr) *http.Client {
		return &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return chatResponse(r, []any{chatFunction("search_media", "media", `{"query":"石阶"}`)}), nil
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			found := false
			for _, raw := range body["input"].([]any) {
				row := raw.(map[string]any)
				if row["type"] == "function_call_output" {
					var result map[string]any
					json.Unmarshal([]byte(row["output"].(string)), &result)
					found = len(result["items"].([]any)) == 1
				}
			}
			if !found {
				t.Error("agent did not receive authorized media evidence")
			}
			return chatResponse(r, []any{chatText("找到石阶片段，请打开证据复核。")}), nil
		})}
	}
	res = f.request(t, "POST", path, `{"content":"找一下石阶的视频"}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 201 || calls != 2 {
		t.Fatalf("agent %d %+v", res.StatusCode, data)
	}
	var stored []byte
	if err := f.db.QueryRow(`SELECT tool_calls_json FROM agent_messages WHERE role='assistant' ORDER BY id DESC LIMIT 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var retained []map[string]any
	if json.Unmarshal(stored, &retained) != nil || len(retained) != 1 {
		t.Fatal("agent evidence retention failed")
	}
	refs, ok := retained[0]["evidenceRefs"].([]any)
	if !ok || len(refs) != 1 || refs[0].(map[string]any)["id"] != fmt.Sprint(aid) {
		t.Fatalf("retained evidence %s", stored)
	}
	revoke = true
	_, err := f.server.executeMediaSearch(context.Background(), uid, int32(pid), []byte(`{"query":"石阶"}`))
	if err == nil || err.Error() != "PROJECT_ACCESS_DENIED" {
		t.Fatalf("revoked access %v", err)
	}
}
