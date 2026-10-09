package httpapi

import (
	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/media"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type videoTestStore struct{ media.ObjectStorage }

func (s videoTestStore) PutRawResult(ctx context.Context, key string, r io.Reader, mime string) (algorithm.RawResultObject, error) {
	o, e := s.PutObject(ctx, key, r, mime)
	return algorithm.RawResultObject{Key: o.Key, ChecksumSHA256: o.ChecksumSHA256}, e
}
func (s videoTestStore) ReadAlgorithmAsset(ctx context.Context, key string) (algorithm.AlgorithmAsset, error) {
	o, e := s.GetObject(ctx, key)
	return algorithm.AlgorithmAsset{Body: o.Body, ContentType: o.ContentType}, e
}

func TestVideoAnalysisBackgroundAndPlaybackAnnotations(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Fatal("ffmpeg is required for video integration acceptance")
	}
	f := newAPIFixture(t)
	team, pid := f.project(t)
	root := t.TempDir()
	storage, err := media.NewLocalObjectStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	store := videoTestStore{storage}
	f.server.AttachObjectStorage(storage)
	video := filepath.Join(root, "fixture.mp4")
	if output, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=10", "-t", "2.1", "-c:v", "libx264", "-pix_fmt", "yuv420p", video).CombinedOutput(); err != nil {
		t.Fatalf("video generation: %v %s", err, output)
	}
	data, err := os.ReadFile(video)
	if err != nil {
		t.Fatal(err)
	}
	object, err := storage.PutObject(context.Background(), fmt.Sprintf("projects/%d/source.mp4", pid), bytes.NewReader(data), "video/mp4")
	if err != nil {
		t.Fatal(err)
	}
	signer := algorithm.NewAssetURLSigner(strings.Repeat("v", 32), "")
	// Recreate the signer with the TLS media gateway's URL after it is listening.
	var gateway *httptest.Server
	gateway = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		algorithm.NewAssetAccessHandler(f.db, store, signer).ServeHTTP(w, r)
	}))
	defer gateway.Close()
	signer = algorithm.NewAssetURLSigner(strings.Repeat("v", 32), gateway.URL)
	calls := 0
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var input algorithm.Input
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			t.Error("bad frame input")
			w.WriteHeader(400)
			return
		}
		if input.InputAsset.MIMEType != "image/jpeg" || input.Context["videoAssetId"] == nil || input.Context["videoAnalysis"] != nil {
			t.Error("frame source metadata missing")
		}
		res, err := gateway.Client().Get(input.InputAsset.AccessURL)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		defer res.Body.Close()
		if _, err = jpeg.Decode(res.Body); res.StatusCode != 200 || err != nil {
			t.Error("provider did not receive a valid frozen JPEG")
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":[{"id":"p1","class":"person","score":0.9,"bbox":{"type":"bbox","x":10,"y":20,"width":30,"height":40}}]}`)
	}))
	defer provider.Close()
	var providerID, definition, version int64
	var aid int
	if err = f.db.QueryRow(`insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status) values($1,$2,'Video fixture','http-json',$3,'active') returning id`, pid, team, provider.URL).Scan(&providerID); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`insert into algorithm_definitions(project_id,team_id,provider_id,name,capability_code) values($1,$2,$3,'Video detection','detection') returning id`, pid, team, providerID).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`insert into algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process,output_mapping_json) values($1,$2,$3,1,'published','synchronous','fixture','{"detectionsPath":"results","keyPath":"id","labelPath":"class","confidencePath":"score","geometryPath":"bbox"}') returning id`, pid, team, definition).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`update algorithm_definitions set current_published_version_id=$2 where id=$1`, definition, version); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`insert into assets(project_id,team_id,kind,mime_type,storage_key,logical_key,checksum_sha256,metadata_json) values($1,$2,'video','video/mp4',$3,$3,$4,'{"name":"fixture.mp4","durationSeconds":2.1}') returning id`, pid, team, object.Key, object.ChecksumSHA256).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	endpoint := fmt.Sprintf("/api/projects/%d/algorithm-runs", pid)
	response := f.request(t, "POST", endpoint, fmt.Sprintf(`{"configurationSnapshotId":%d,"assetId":%d,"videoFps":1}`, version, aid))
	created := decodedResponse(t, response)
	if response.StatusCode != 202 {
		t.Fatalf("start: %d %+v", response.StatusCode, created)
	}
	id := created["runId"].(string)
	worker := algorithm.NewProcessor(provider.Client(), nil, store, "", signer, nil, "").WithVideoWorker(f.db, storage)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if worked, err := worker.ProcessVideoNext(ctx); err != nil || !worked {
		t.Fatalf("worker: %v %v", worked, err)
	}
	var state string
	if err = f.db.QueryRow(`select status from algorithm_runs where id=$1`, id).Scan(&state); err != nil || state != "succeeded" {
		var message string
		f.db.QueryRow(`select coalesce(error_message,'') from algorithm_runs where id=$1`, id).Scan(&message)
		rows, _ := f.db.Query(`select coalesce(error_category,''),coalesce(response_status,0) from algorithm_run_attempts where algorithm_run_id=$1`, id)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var category string
				var code int
				rows.Scan(&category, &code)
				t.Logf("attempt %s HTTP %d", category, code)
			}
		}
		t.Fatalf("state %s %v: %s; calls %d", state, err, message, calls)
	}
	response = f.request(t, "GET", endpoint+"/"+id+"/annotations", "")
	annotations := decodedResponse(t, response)
	frames, ok := annotations["frames"].([]any)
	if response.StatusCode != 200 || !ok || len(frames) < 2 {
		t.Fatalf("annotations %d %+v", response.StatusCode, annotations)
	}
	for index, value := range frames {
		frame := value.(map[string]any)
		if frame["timeMs"] != float64(index*1000) || frame["width"] != float64(320) || len(frame["result"].(map[string]any)["detections"].([]any)) != 1 {
			t.Fatalf("frame %+v", frame)
		}
	}
	initialCalls := calls
	var assetCount, attemptCount int
	if err = f.db.QueryRow(`select count(*) from assets where project_id=$1`, pid).Scan(&assetCount); err != nil || assetCount != 1 {
		t.Fatal("ordinary frames created material rows", assetCount, err)
	}
	if err = f.db.QueryRow(`select count(*) from algorithm_run_attempts where algorithm_run_id=$1`, id).Scan(&attemptCount); err != nil || attemptCount != 0 {
		t.Fatal("frame diagnostics stored in business database", attemptCount, err)
	}
	for index := range frames {
		key := fmt.Sprintf("projects/%d/algorithm-runs/%s/frames/%06d.attempts.json", pid, id, index)
		if object, err := storage.GetObject(ctx, key); err != nil || !json.Valid(object.Body) {
			t.Fatal("frame diagnostics missing from object storage", index, err)
		}
	}
	// Simulate a restart after frame artifacts were saved but completion was lost.
	if _, err = f.db.Exec(`update algorithm_runs set status='running' where id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = worker.ProcessVideoNext(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != initialCalls {
		t.Fatal("resume repeated completed frame inference")
	}
	response = f.request(t, "GET", endpoint+"/"+id+"/annotations?format=jsonl", "")
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || len(bytes.Split(bytes.TrimSpace(body), []byte("\n"))) != len(frames) {
		t.Fatal("JSONL export incomplete")
	}
	rowsResponse := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/assets", pid), "")
	var assets []map[string]any
	json.NewDecoder(rowsResponse.Body).Decode(&assets)
	rowsResponse.Body.Close()
	if len(assets) != 1 {
		t.Fatal("generated analysis frames leaked into library")
	}
	_, other := f.project(t)
	response = f.request(t, "GET", fmt.Sprintf("/api/projects/%d/algorithm-runs/%s/annotations", other, id), "")
	response.Body.Close()
	if response.StatusCode == 200 {
		t.Fatal("cross-project annotations accessible")
	}
}
