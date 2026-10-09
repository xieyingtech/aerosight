package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/media"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
)

func fmtID(id int32) string { return strconv.FormatInt(int64(id), 10) }

func TestConfigurationPinsSpacesAndEnsureDoesNotMaskAuthFailure(t *testing.T) {
	t.Setenv("SEMANTIC_ENABLED", "true")
	t.Setenv("QDRANT_URL", "http://127.0.0.1:6333")
	t.Setenv("EMBEDDING_URL", "http://127.0.0.1:6335")
	t.Setenv("EMBEDDING_MODEL", "e5")
	t.Setenv("EMBEDDING_REVISION", strings.Repeat("a", 40))
	t.Setenv("EMBEDDING_DIMENSION", "384")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	other := cfg
	other.Dimension = 768
	if other.Space() == cfg.Space() {
		t.Fatal("model spaces mixed")
	}
	t.Setenv("EMBEDDING_REVISION", "latest")
	if _, err = LoadConfig(); err == nil {
		t.Fatal("unpinned revision accepted")
	}
	calls := 0
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" {
			t.Error("authentication failure should not attempt collection creation")
		}
		w.WriteHeader(403)
	}))
	defer host.Close()
	cfg.QdrantURL = host.URL
	if err = (Client{Config: cfg}).Ensure(context.Background()); err == nil || calls != 1 {
		t.Fatal("authentication failure masked")
	}
}

func TestEmbeddingRejectsWrongSpaceAndMalformedVectors(t *testing.T) {
	for _, body := range []string{`{"model":"wrong","revision":"rev","data":[{"index":0,"embedding":[1,0]}]}`, `{"model":"e5","revision":"wrong","data":[{"index":0,"embedding":[1,0]}]}`, `{"model":"e5","revision":"rev","data":[{"index":0,"embedding":[0,0]}]}`, `{"model":"e5","revision":"rev","data":[{"index":2,"embedding":[1,0]}]}`} {
		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		c := Client{Config: Config{EmbeddingURL: host.URL, Model: "e5", Revision: "rev", Dimension: 2}}
		if _, err := c.Embed(context.Background(), []string{"树木"}, true); err == nil {
			t.Fatal("invalid embedding accepted")
		}
		host.Close()
	}
	if validVector([]float32{1}, 2) {
		t.Fatal("wrong dimension accepted")
	}
}
func TestTimestampQualityAndSearchInput(t *testing.T) {
	src := source{Captured: sqlTime{Time: time.Now(), Valid: true}, Metadata: []byte(`{"timeQuality":"user-supplied-unverified"}`)}
	_, a, b := segmentTimes(src, 0, 1000)
	if a != nil || b != nil {
		t.Fatal("unverified timestamp promoted")
	}
	src.Metadata = []byte(`{"timeQuality":"verified"}`)
	_, a, b = segmentTimes(src, 10, 1010)
	if a == nil || b.Sub(*a) != time.Second {
		t.Fatal("trusted time lost")
	}
	for _, in := range []SearchInput{{Query: ""}, {Query: "x", Limit: 21}, {Query: "x", Start: a}, {Query: "x", Start: b, End: a}} {
		if in.Validate() == nil {
			t.Fatal("invalid query accepted")
		}
	}
}

func TestDurableIndexRebuildAndStaleCandidates(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.Bootstrap(ctx, db); err != nil {
		t.Fatal(err)
	}
	var team, pid, aid int32
	if err := db.QueryRow(`INSERT INTO teams(name) VALUES('semantic') RETURNING id`).Scan(&team); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO projects(name,team_id) VALUES('semantic',$1) RETURNING id`, team).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, _ := media.NewLocalObjectStorage(root)
	var jpegData bytes.Buffer
	jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 20, 20)), nil)
	obj, err := store.PutObject(ctx, "projects/"+fmtID(pid)+"/test.jpg", bytes.NewReader(jpegData.Bytes()), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO assets(project_id,team_id,kind,mime_type,storage_key,logical_key,checksum_sha256,metadata_json) VALUES($1,$2,'image','image/jpeg',$3,$3,$4,'{"timeQuality":"unknown"}') RETURNING id`, pid, team, obj.Key, obj.ChecksumSHA256).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	points := map[string]Point{}
	visionCalls := 0
	failVisionAt := 0
	failUpsert := false
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/embeddings":
			var input struct {
				Input []string `json:"input"`
			}
			json.NewDecoder(r.Body).Decode(&input)
			rows := []any{}
			for i := range input.Input {
				rows = append(rows, map[string]any{"index": i, "embedding": []float32{1, 0}})
			}
			json.NewEncoder(w).Encode(map[string]any{"model": "e5", "revision": "rev", "data": rows})
		case r.URL.Path == "/chat/completions":
			visionCalls++
			if failVisionAt > 0 && visionCalls == failVisionAt {
				w.WriteHeader(503)
				return
			}
			w.Write([]byte(`{"choices":[{"message":{"content":"林间石阶"}}]}`))
		case strings.HasSuffix(r.URL.Path, "/points/query"):
			hits := []Hit{}
			for _, p := range points {
				hits = append(hits, Hit{ID: p.ID, Score: .95})
			}
			hits = append(hits, Hit{ID: "not-a-uuid", Score: 1})
			json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"points": hits}})
		case strings.HasSuffix(r.URL.Path, "/points/delete"):
			var body struct {
				Points []string `json:"points"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			for _, id := range body.Points {
				delete(points, id)
			}
			w.Write([]byte(`{"result":true}`))
		case strings.HasSuffix(r.URL.Path, "/points"):
			if failUpsert {
				w.WriteHeader(503)
				return
			}
			var body struct {
				Points []Point `json:"points"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			for _, p := range body.Points {
				points[p.ID] = p
			}
			w.Write([]byte(`{"result":true}`))
		default:
			w.Write([]byte(`{"result":{"config":{"params":{"vectors":{"size":2,"distance":"Cosine"}}}}}`))
		}
	}))
	defer host.Close()
	var providerID int64
	err = db.QueryRow(`INSERT INTO ai_providers(name,provider_type,base_url,model_id,enabled,is_default,credential_envelope_json,created_by_user_id,updated_by_user_id) SELECT 'vision','openai',$1,'vision',true,true,'{}',id,id FROM users LIMIT 1 RETURNING ai_providers.id`, host.URL).Scan(&providerID)
	if err != nil {
		t.Fatal(err)
	}
	envelope, _ := credentials.EncryptJSON(map[string]string{"apiKey": "test"}, "secret", credentials.AAD("ai-provider", providerID, nil))
	raw, _ := json.Marshal(envelope)
	if _, err = db.Exec(`UPDATE ai_providers SET credential_envelope_json=$2 WHERE id=$1`, providerID, raw); err != nil {
		t.Fatal(err)
	}
	svc := &Service{DB: db, Client: Client{Config: Config{QdrantURL: host.URL, EmbeddingURL: host.URL, Model: "e5", Revision: "rev", Dimension: 2}}, Storage: store, Root: root, Secret: "secret"}
	failUpsert = true
	if svc.Tick(ctx) == nil {
		t.Fatal("Qdrant failure should fail processing")
	}
	status, err := svc.Status(ctx, pid, aid)
	if err != nil || status.State != "failed" {
		t.Fatalf("failure %+v %v", status, err)
	}
	failUpsert = false
	if err = svc.Retry(ctx, pid, aid); err != nil {
		t.Fatal(err)
	}
	if err = svc.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if visionCalls != 1 {
		t.Fatal("retry re-ran vision instead of persisted artifact")
	}
	status, err = svc.Status(ctx, pid, aid)
	if err != nil || status.State != "indexed" || status.Segments != 1 {
		t.Fatalf("indexed %+v %v", status, err)
	}
	allow := func(context.Context, int32) error { return nil }
	matches, _, err := svc.Search(ctx, pid, SearchInput{Query: "石阶"}, allow)
	if err != nil || len(matches) != 1 {
		t.Fatalf("search %v %v", matches, err)
	}
	matches, _, err = svc.Search(ctx, pid+1, SearchInput{Query: "石阶"}, allow)
	if err != nil || len(matches) != 0 {
		t.Fatal("cross-project candidates leaked")
	}
	matches, _, err = svc.Search(ctx, pid, SearchInput{Query: "石阶"}, func(context.Context, int32) error { return errors.New("denied") })
	if err != nil || len(matches) != 0 {
		t.Fatal("denied candidates leaked")
	}
	now := time.Now()
	later := now.Add(time.Hour)
	matches, _, err = svc.Search(ctx, pid, SearchInput{Query: "石阶", Start: &now, End: &later}, allow)
	if err != nil || len(matches) != 0 {
		t.Fatal("unknown capture time matched")
	}
	mu.Lock()
	clear(points)
	mu.Unlock()
	if err = svc.Retry(ctx, pid, aid); err != nil {
		t.Fatal(err)
	}
	if err = svc.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if visionCalls != 1 || len(points) != 1 {
		t.Fatal("artifact rebuild failed")
	}
	var resumedID int64
	if err = db.QueryRow(`UPDATE media_index_jobs SET state='running',attempts=10,next_attempt_at=now() WHERE asset_id=$1 RETURNING id`, aid).Scan(&resumedID); err != nil {
		t.Fatal(err)
	}
	owner, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.ExecContext(ctx, `SELECT pg_advisory_lock(7391,$1::integer)`, resumedID); err != nil {
		t.Fatal(err)
	}
	if processed, e := svc.Process(ctx, resumedID); e != nil || processed {
		t.Fatal("duplicate processing while another worker owns job")
	}
	if _, err = owner.ExecContext(ctx, `SELECT pg_advisory_unlock(7391,$1::integer)`, resumedID); err != nil {
		t.Fatal(err)
	}
	owner.Close()
	if processed, e := svc.Process(ctx, resumedID); e != nil || !processed {
		t.Fatalf("running task recovery %v", e)
	}
	if visionCalls != 1 {
		t.Fatal("recovery repeated vision analysis")
	}
	if _, err = db.Exec(`UPDATE assets SET version=2 WHERE id=$1`, aid); err != nil {
		t.Fatal(err)
	}
	matches, _, err = svc.Search(ctx, pid, SearchInput{Query: "石阶"}, allow)
	if err != nil || len(matches) != 0 {
		t.Fatal("obsolete version leaked")
	}
	if err = svc.Reconcile(ctx); err != nil || len(points) != 0 {
		t.Fatalf("stale cleanup %v", err)
	}
	if err = svc.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DELETE FROM assets WHERE id=$1`, aid); err != nil {
		t.Fatal(err)
	}
	if err = svc.Reconcile(ctx); err != nil || len(points) != 0 {
		t.Fatalf("hard delete cleanup %v", err)
	}
	if _, e := exec.LookPath("ffmpeg"); e != nil {
		t.Log("video checkpoint extension not run: ffmpeg unavailable")
		return
	}
	if _, e := exec.LookPath("ffprobe"); e != nil {
		t.Log("video checkpoint extension not run: ffprobe unavailable")
		return
	}
	videoPath := filepath.Join(t.TempDir(), "checkpoint.mp4")
	if err = exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "color=c=black:s=32x32:r=10", "-t", "11", "-pix_fmt", "yuv420p", "-c:v", "libx264", videoPath).Run(); err != nil {
		t.Fatal(err)
	}
	videoBytes, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	videoObj, err := store.PutObject(ctx, "projects/"+fmtID(pid)+"/checkpoint.mp4", bytes.NewReader(videoBytes), "video/mp4")
	if err != nil {
		t.Fatal(err)
	}
	var videoID int32
	if err = db.QueryRow(`INSERT INTO assets(project_id,team_id,kind,mime_type,storage_key,logical_key,checksum_sha256) VALUES($1,$2,'video','video/mp4',$3,$3,$4) RETURNING id`, pid, team, videoObj.Key, videoObj.ChecksumSHA256).Scan(&videoID); err != nil {
		t.Fatal(err)
	}
	baseline := visionCalls
	failVisionAt = baseline + 2
	if err = svc.Tick(ctx); err == nil {
		t.Fatal("second window failure should remain failed")
	}
	var key string
	if err = db.QueryRow(`SELECT artifact_key FROM media_index_jobs WHERE asset_id=$1`, videoID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	partialObj, err := store.GetObject(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	var partial Artifact
	if json.Unmarshal(partialObj.Body, &partial) != nil || partial.Complete || len(partial.Segments) != 1 {
		t.Fatal("first-window checkpoint was not preserved")
	}
	failVisionAt = 0
	if err = svc.Retry(ctx, pid, videoID); err != nil {
		t.Fatal(err)
	}
	if err = svc.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if visionCalls != baseline+3 {
		t.Fatal("resume repeated the first window")
	}
	status, err = svc.Status(ctx, pid, videoID)
	if err != nil || status.State != "indexed" || status.Segments != 2 {
		t.Fatalf("video resume %+v %v", status, err)
	}
}
