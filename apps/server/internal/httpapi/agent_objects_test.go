package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObjectQueryFilterAndInput(t *testing.T) {
	all := []queryDetection{{DetectionKey: "0", Label: "bus", Confidence: .9}, {DetectionKey: "1", Label: "person", Confidence: .7}}
	selected, err := filterQueryDetections(all, objectQueryInput{Labels: []string{"BUS"}})
	if err != nil || len(selected) != 1 || selected[0].DetectionKey != "0" {
		t.Fatal(selected, err)
	}
	empty := []string{}
	selected, err = filterQueryDetections(all, objectQueryInput{SelectedDetectionKeys: &empty})
	if err != nil || len(selected) != 0 {
		t.Fatal("empty selection", selected, err)
	}
	invalid := []string{"missing"}
	if _, err = filterQueryDetections(all, objectQueryInput{SelectedDetectionKeys: &invalid}); err == nil {
		t.Fatal("forged key accepted")
	}
	person := []string{"1"}
	if _, err = filterQueryDetections(all, objectQueryInput{Labels: []string{"bus"}, SelectedDetectionKeys: &person}); err == nil {
		t.Fatal("filtered-out key accepted")
	}
	for _, raw := range []string{`{"projectId":3}`, `{"labels":["bus"]}`, `{"algorithmRunId":"bad"}`, `{"minConfidence":2}`, `{"selectedDetectionKeys":null}`, `{"algorithmRunId":"11111111-1111-4111-8111-111111111111","selectedDetectionKeys":[]}`} {
		if _, err := parseObjectQuery([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if _, err := parseObjectQuery([]byte(`{"algorithmRunId":"11111111-1111-4111-8111-111111111111","selectedDetectionKeys":[],"selectionReason":"零匹配"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAgentSkill([]byte(`{"skillName":"../secret"}`)); err == nil {
		t.Fatal("arbitrary skill allowed")
	}
	skill, err := loadAgentSkill([]byte(`{"skillName":"inspection-object-query"}`))
	if err != nil || !strings.Contains(fmt.Sprint(skill), "单期图像") {
		t.Fatal(skill, err)
	}
	href := objectResultHref(1, "run", objectQueryInput{SelectedDetectionKeys: &empty, SelectionReason: "零匹配"}, nil)
	parsed, _ := url.Parse(href)
	if parsed.Query().Get("filtered") != "1" || len(parsed.Query()["object"]) != 0 {
		t.Fatal(href)
	}
}

func TestObjectQueryHTTPAndChatImages(t *testing.T) {
	f, team, pid, uid, sid, path := newChatFixture(t)
	var buf bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 24, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 24; x++ {
			im.Set(x, y, color.RGBA{R: 240, A: 255})
		}
	}
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	body := buf.Bytes()
	digest := sha256.Sum256(body)
	root := t.TempDir()
	key := fmt.Sprintf("projects/%d/objects.png", pid)
	file := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, body, 0600); err != nil {
		t.Fatal(err)
	}
	f.server.AttachMediaStorage(root)
	var provider, definition, version int
	var asset int32
	if err := f.db.QueryRow(`insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status) values($1,$2,'Local detector','http-json','https://detector.example','active') returning id`, pid, team).Scan(&provider); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`insert into algorithm_definitions(project_id,team_id,provider_id,name,capability_code) values($1,$2,$3,'Detect','detection') returning id`, pid, team, provider).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`insert into algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process) values($1,$2,$3,1,'published','synchronous','fixture') returning id`, pid, team, definition).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`insert into assets(project_id,team_id,kind,storage_key,logical_key,checksum_sha256,mime_type) values($1,$2,'image',$3,$3,$4,'image/png') returning id`, pid, team, key, hex.EncodeToString(digest[:])).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	snapshot, _ := json.Marshal(gin.H{"inputAsset": gin.H{"assetId": asset, "version": 1, "checksumSha256": hex.EncodeToString(digest[:]), "accessUrl": "https://private.example?token=secret"}})
	canonical := `{"kind":"detection","result":{"detections":[{"detectionKey":"0","label":"bus","confidence":0.9,"pixelGeometry":{"type":"bbox","x":0,"y":0,"width":12,"height":12}},{"detectionKey":"1","label":"person","confidence":0.8,"pixelGeometry":{"type":"bbox","x":12,"y":0,"width":12,"height":12}}]}}`
	if _, err := f.db.Exec(`insert into algorithm_runs(id,project_id,team_id,algorithm_definition_version_id,input_asset_id,idempotency_key,status,input_snapshot_json,canonical_result_json) values($1,$2,$3,$4,$5,$8,'succeeded',$6,$7)`, id, pid, team, version, asset, string(snapshot), canonical, id); err != nil {
		t.Fatal(err)
	}
	args := []byte(fmt.Sprintf(`{"algorithmRunId":%q}`, id))
	result, err := f.server.executeObjectQuery(context.Background(), uid, int32(pid), args)
	if err != nil || result["visualStatus"] != "available" || len(result["_images"].([]agentPhoto)) != 3 {
		t.Fatal(result, err)
	}
	_, other := f.project(t)
	if _, err = f.server.executeObjectQuery(context.Background(), uid, int32(other), args); err == nil {
		t.Fatal("cross-project run leaked")
	}
	imagePath := fmt.Sprintf("/api/projects/%d/algorithm-runs/%s/image", pid, id)
	res := f.request(t, "GET", imagePath, "")
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("image %d", res.StatusCode)
	}
	calls := 0
	f.server.aiHTTPClientFactory = func(*url.URL, []netip.Addr) *http.Client {
		return &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			var params map[string]any
			if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
				t.Fatal(err)
			}
			switch calls {
			case 1:
				return chatResponse(r, []any{chatFunction("load_skill", "skill", `{"skillName":"inspection-object-query"}`)}), nil
			case 2:
				encoded, _ := json.Marshal(params["input"])
				if !strings.Contains(string(encoded), "单期图像") {
					t.Fatal("skill absent from model context")
				}
				return chatResponse(r, []any{chatFunction("query_objects", "objects", string(args))}), nil
			case 3:
				encoded, _ := json.Marshal(params["input"])
				text := string(encoded)
				if !strings.Contains(text, "input_image") || !strings.Contains(text, "detectionKey=0") || strings.Contains(text, "token=secret") {
					t.Fatal("image context or secret boundary")
				}
				return chatResponse(r, []any{chatFunction("query_objects", "selection", fmt.Sprintf(`{"algorithmRunId":%q,"selectedDetectionKeys":["0"],"selectionReason":"保留车辆","includeImage":false}`, id))}), nil
			default:
				return chatResponse(r, []any{chatText("保留 1 个车辆候选，需要人工核查。")}), nil
			}
		})}
	}
	res = f.request(t, "POST", path, `{"content":"只保留车辆"}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 201 || calls != 4 {
		t.Fatal(res.StatusCode, data, calls)
	}
	var stored []byte
	if err := f.db.QueryRow(`select tool_calls_json from agent_messages where session_id=$1 and role='assistant' order by id desc limit 1`, sid).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	encoded := stored
	if strings.Contains(string(encoded), "data:image") || !strings.Contains(string(encoded), "filtered=1") {
		t.Fatal("history must retain selection, not pixels", string(encoded))
	}
	if err = os.WriteFile(file, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = f.server.executeObjectQuery(context.Background(), uid, int32(pid), args)
	if err != nil || result["visualStatus"] != "unavailable" || result["_images"] != nil {
		t.Fatal("changed image sent", result, err)
	}
	res = f.request(t, "GET", imagePath, "")
	res.Body.Close()
	if res.StatusCode != 409 {
		t.Fatal(res.StatusCode)
	}
	if _, err = f.db.Exec(`update algorithm_runs set status='queued' where id=$1`, id); err != nil {
		t.Fatal(err)
	}
	result, err = f.server.executeObjectQuery(context.Background(), uid, int32(pid), args)
	if err != nil || strings.Contains(fmt.Sprint(result), "detections") || result["_images"] != nil {
		t.Fatal("pending run presented as detections", result, err)
	}
}
