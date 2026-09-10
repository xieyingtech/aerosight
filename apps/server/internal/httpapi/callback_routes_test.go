package httpapi

import (
	"aerosight/server/internal/algorithm"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

type callbackTestStore struct {
	reads, writes int
	body          []byte
}

func (s *callbackTestStore) PutRawResult(ctx context.Context, key string, body io.Reader, contentType string) (algorithm.RawResultObject, error) {
	raw, err := io.ReadAll(body)
	s.writes++
	hash := sha256.Sum256(raw)
	return algorithm.RawResultObject{Key: key, ChecksumSHA256: hex.EncodeToString(hash[:])}, err
}
func (s *callbackTestStore) ReadAlgorithmAsset(context.Context, string) (algorithm.AlgorithmAsset, error) {
	s.reads++
	if s.body != nil {
		return algorithm.AlgorithmAsset{Body: s.body, ContentType: "image/jpeg"}, nil
	}
	return algorithm.AlgorithmAsset{Body: []byte("asset-bytes"), ContentType: "image/jpeg"}, nil
}

func TestAlgorithmAssetRangeAndTransferDeadline(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	var aid int
	if err := f.db.QueryRow("insert into assets(project_id,team_id,kind,storage_key,logical_key,mime_type) values($1,$2,'image','test.jpg','test.jpg','image/jpeg') returning id", pid, team).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	store := &callbackTestStore{body: []byte(strings.Repeat("algorithm asset ", 20000))}
	signer := algorithm.NewAssetURLSigner(strings.Repeat("s", 32), "https://aerosight.example")
	f.server.AttachRuntime(algorithm.NewAssetAccessHandler(f.db, store, signer))
	f.host.Close()
	f.server.cfg.RequestTimeout = 100 * time.Millisecond
	handler := f.server.Handler()
	f.host = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(&delayedMediaWriter{ResponseWriter: w}, r)
	}))
	t.Cleanup(f.host.Close)
	signed, err := signer.IssueAssetURL(pid, aid, 1, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(signed)
	for _, tc := range []struct {
		method, rng string
		status      int
		body        string
	}{
		{"GET", "", 200, string(store.body)}, {"GET", "bytes=2-5", 206, string(store.body[2:6])}, {"HEAD", "", 200, ""}, {"GET", "bytes=9999999-", 416, ""},
	} {
		r, _ := http.NewRequest(tc.method, f.host.URL+u.RequestURI(), nil)
		r.Header.Set("Range", tc.rng)
		r.Header.Set("Accept-Encoding", "gzip")
		res, err := f.client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != tc.status || tc.status != 416 && string(body) != tc.body || res.Header.Get("Content-Encoding") != "" {
			t.Fatalf("asset %s range=%s status=%d bytes=%d err=%v", tc.method, tc.rng, res.StatusCode, len(body), err)
		}
		if tc.status == 206 && !strings.HasPrefix(res.Header.Get("Content-Range"), "bytes 2-5/") {
			t.Fatal("missing Content-Range")
		}
		if tc.method == "HEAD" && res.Header.Get("Content-Length") != strconv.Itoa(len(store.body)) {
			t.Fatal("HEAD length")
		}
	}
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("lock table assets in access exclusive mode"); err != nil {
		t.Fatal(err)
	}
	res, err := f.client.Get(f.host.URL + u.RequestURI())
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 504 {
		t.Fatalf("unbounded asset lookup %d", res.StatusCode)
	}
}

func TestUnifiedAlgorithmCallbacksAndAssets(t *testing.T) {
	f := newAPIFixture(t)
	store := &callbackTestStore{}
	signer := algorithm.NewAssetURLSigner(strings.Repeat("s", 32), "https://aerosight.example")
	mux := http.NewServeMux()
	mux.Handle("/callbacks/algorithms/", algorithm.NewCallbackHandler(f.db, store))
	mux.Handle("/algorithm-assets/", algorithm.NewAssetAccessHandler(f.db, store, signer))
	f.server.AttachRuntime(mux)
	const token = "callback-token-with-at-least-thirty-two-characters"
	hash := sha256.Sum256([]byte(token))
	type scope struct {
		pid, asset int
		provider   int64
		run        string
	}
	scopes := []scope{}
	for i := 0; i < 2; i++ {
		team, pid := f.project(t)
		var provider, definition, version int64
		var asset int
		var run string
		if err := f.db.QueryRow("insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status) values($1,$2,'callback','http-json','https://algorithm.example','active') returning id", pid, team).Scan(&provider); err != nil {
			t.Fatal(err)
		}
		if err := f.db.QueryRow("insert into algorithm_definitions(project_id,team_id,provider_id,name,capability_code) values($1,$2,$3,'callback','detection') returning id", pid, team, provider).Scan(&definition); err != nil {
			t.Fatal(err)
		}
		if err := f.db.QueryRow("insert into algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process,output_mapping_json) values($1,$2,$3,1,'published','callback','test','{\"detectionsPath\":\"results\"}') returning id", pid, team, definition).Scan(&version); err != nil {
			t.Fatal(err)
		}
		if err := f.db.QueryRow("insert into assets(project_id,team_id,kind,storage_key,logical_key,status,mime_type) values($1,$2,'image','callback.jpg','callback.jpg','available','image/jpeg') returning id", pid, team).Scan(&asset); err != nil {
			t.Fatal(err)
		}
		if err := f.db.QueryRow("insert into algorithm_runs(id,project_id,team_id,algorithm_definition_version_id,input_asset_id,idempotency_key,status,external_job_id,callback_token_hash) values(gen_random_uuid(),$1,$2,$3,$4,'callback-test','running','job-1',$5) returning id", pid, team, version, asset, hex.EncodeToString(hash[:])).Scan(&run); err != nil {
			t.Fatal(err)
		}
		scopes = append(scopes, scope{pid, asset, provider, run})
	}
	own, other := scopes[0], scopes[1]
	call := func(run string, provider int64, callbackID, body string, at time.Time, expected int) string {
		t.Helper()
		req, err := http.NewRequest("POST", f.host.URL+"/callbacks/algorithms/"+run, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", "https://external-provider.example")
		req.Header.Set("X-Aerosight-Provider-Id", strconv.FormatInt(provider, 10))
		req.Header.Set("X-Aerosight-Callback-Id", callbackID)
		req.Header.Set("X-Aerosight-Timestamp", strconv.FormatInt(at.Unix(), 10))
		req.Header.Set("X-Aerosight-Callback-Token", token)
		req.Header.Set("X-Aerosight-Signature", algorithm.SignCallback(token, callbackID, at, []byte(body)))
		// Use a client without the browser session cookie or CSRF token.
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil || res.StatusCode != expected {
			t.Fatalf("callback %d %s %v", res.StatusCode, raw, err)
		}
		return string(raw)
	}
	now := time.Now()
	body := fmt.Sprintf(`{"providerId":%d,"externalJobId":"job-1","status":"processing"}`, own.provider)
	call(own.run, own.provider, "expired", body, now.Add(-10*time.Minute), 401)
	call(other.run, own.provider, "cross-project", body, now, 401)
	call(own.run, own.provider, "processing", body, now, 200)
	if raw := call(own.run, own.provider, "processing", body, now, 200); !strings.Contains(raw, `"duplicate":true`) {
		t.Fatalf("retry %s", raw)
	}
	call(own.run, own.provider, "processing", body+" ", now, 409)
	completed := fmt.Sprintf(`{"providerId":%d,"externalJobId":"job-1","status":"completed","result":{"results":[]}}`, own.provider)
	call(own.run, own.provider, "completed", completed, now, 200)
	call(own.run, own.provider, "completed", completed, now, 200)
	if store.writes != 1 {
		t.Fatalf("duplicate result storage %d", store.writes)
	}
	var status, otherStatus string
	var receipts int
	if err := f.db.QueryRow("select status from algorithm_runs where id=$1", own.run).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("select status from algorithm_runs where id=$1", other.run).Scan(&otherStatus); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("select count(*) from algorithm_callback_receipts").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || otherStatus != "running" || receipts != 2 {
		t.Fatalf("state %s %s %d", status, otherStatus, receipts)
	}
	call(own.run, own.provider, "large", strings.Repeat("x", (16<<20)+1), now, 413)
	assetCall := func(signed string, expected int) {
		t.Helper()
		u, err := url.Parse(signed)
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.Get(f.host.URL + u.RequestURI())
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode != expected {
			t.Fatalf("asset %d %s", res.StatusCode, raw)
		}
		if expected == 200 && (string(raw) != "asset-bytes" || res.Header.Get("Cache-Control") != "private, no-store") {
			t.Fatalf("asset body/header %s", raw)
		}
	}
	signed, err := signer.IssueAssetURL(own.pid, own.asset, 1, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	assetCall(signed, 200)
	u, _ := url.Parse(signed)
	values := u.Query()
	values.Set("projectId", strconv.Itoa(other.pid))
	u.RawQuery = values.Encode()
	assetCall(u.String(), 403)
	cross, _ := signer.IssueAssetURL(other.pid, own.asset, 1, now.Add(time.Minute))
	assetCall(cross, 404)
	expired, _ := signer.IssueAssetURL(own.pid, own.asset, 1, now.Add(-time.Minute))
	assetCall(expired, 403)
	version, _ := signer.IssueAssetURL(own.pid, own.asset, 2, now.Add(time.Minute))
	assetCall(version, 404)
	if _, err := f.db.Exec("update assets set deleted_at=now() where id=$1", own.asset); err != nil {
		t.Fatal(err)
	}
	assetCall(signed, 404)
	if store.reads != 1 {
		t.Fatalf("unauthorized storage reads %d", store.reads)
	}
}
