package algorithm

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aerosight/server/internal/outbox"
)

type inspectionHTTPRawStore struct{}

func (inspectionHTTPRawStore) PutRawResult(_ context.Context, key string, body io.Reader, _ string) (RawResultObject, error) {
	raw, err := io.ReadAll(body)
	sum := sha256.Sum256(raw)
	return RawResultObject{Key: key, ChecksumSHA256: hex.EncodeToString(sum[:])}, err
}

// Uses the parent test's real database setup; each scenario rolls back only its
// completion state. Both the Provider and media gateway use real TLS sockets.
func checkInspectionHTTPDispatch(t *testing.T, db *sql.DB, project, team, provider, parentStep int64, child string) {
	t.Helper()
	for _, tc := range []struct {
		name    string
		result  string
		changed bool
		want    string
	}{
		{"zero", `{"modelRevision":"fixture-v1","results":[]}`, false, "succeeded"},
		{"detection", `{"modelRevision":"fixture-v1","results":[{"id":"one","class":"candidate","score":0.9,"bbox":{"type":"bbox","x":1,"y":2,"width":3,"height":4}}]}`, false, "succeeded"},
		{"changed image", `{"results":[]}`, true, "failed"},
	} {
		t.Run("http "+tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			original := []byte("image bytes protocol fixture")
			sum := sha256.Sum256(original)
			body := original
			if tc.changed {
				body = []byte("changed image")
			}
			signer := NewAssetURLSigner(strings.Repeat("s", 32), "")
			gateway := httptest.NewTLSServer(NewAssetAccessHandler(db, pinnedTestStore(func(context.Context, string) (AlgorithmAsset, error) {
				return AlgorithmAsset{Body: body, ContentType: "image/jpeg"}, nil
			}), signer))
			defer gateway.Close()
			signer.baseURL = gateway.URL
			calls, fetches := 0, 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var input Input
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					http.Error(w, "bad input", 400)
					return
				}
				if input.Context["inspection"] != true || !strings.Contains(input.InputAsset.AccessURL, "checksum=") {
					t.Error("inspection binding missing")
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.InputAsset.AccessURL, nil)
				if err != nil {
					t.Error(err)
					http.Error(w, "bad media request", 400)
					return
				}
				res, err := gateway.Client().Do(req)
				if err != nil {
					t.Error(err)
					http.Error(w, "media unavailable", 400)
					return
				}
				defer res.Body.Close()
				data, err := io.ReadAll(res.Body)
				fetches++
				if err != nil || res.StatusCode != http.StatusOK {
					http.Error(w, "media unavailable", 400)
					return
				}
				if string(data) != string(original) {
					t.Error("provider received wrong bytes")
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, tc.result)
			}))
			defer server.Close()
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err = tx.ExecContext(ctx, "update algorithm_providers set base_url=$2 where id=$1", provider, server.URL); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.ExecContext(ctx, `update algorithm_runs set input_snapshot_json=jsonb_set(input_snapshot_json,'{inputAsset,checksumSha256}',to_jsonb($2::text)) where id=$1`, child, hex.EncodeToString(sum[:])); err != nil {
				t.Fatal(err)
			}
			processor := NewProcessor(server.Client(), nil, inspectionHTTPRawStore{}, "", signer, nil, "")
			payload, _ := json.Marshal(map[string]string{"runId": child})
			event := outbox.Event{ProjectID: int(project), TeamID: int(team), Payload: payload}
			if err = processor.Handler(ctx, tx, event); err != nil {
				t.Fatal(err)
			}
			var state, parent string
			if err = tx.QueryRowContext(ctx, "select status from algorithm_runs where id=$1", child).Scan(&state); err != nil || state != tc.want {
				t.Fatalf("child=%s want=%s err=%v", state, tc.want, err)
			}
			if err = tx.QueryRowContext(ctx, "select status from task_run_steps where id=$1", parentStep).Scan(&parent); err != nil || parent != "running" {
				t.Fatal("child advanced parent", parent, err)
			}
			var completions int
			if err = tx.QueryRowContext(ctx, `select count(*) from outbox_events where project_id=$1 and event_type='inspection.algorithm.completed'`, project).Scan(&completions); err != nil || completions != 1 {
				t.Fatal("missing completion event", completions, err)
			}
			if tc.want == "succeeded" {
				var canonical []byte
				if err = tx.QueryRowContext(ctx, "select canonical_result_json from algorithm_runs where id=$1", child).Scan(&canonical); err != nil {
					t.Fatal(err)
				}
				var saved struct {
					Result CanonicalResult `json:"result"`
					Source struct {
						ModelRevision string `json:"modelRevision"`
					} `json:"source"`
				}
				if err = json.Unmarshal(canonical, &saved); err != nil {
					t.Fatal(err)
				}
				expected := 0
				if tc.name == "detection" {
					expected = 1
				}
				if saved.Result.Kind != ResultDetection || len(saved.Result.Detections) != expected || saved.Source.ModelRevision != "fixture-v1" {
					t.Fatalf("incorrect saved result: %s", canonical)
				}
			}
			if err = processor.Handler(ctx, tx, event); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || fetches != 1 {
				t.Fatalf("replay called provider: calls=%d fetches=%d", calls, fetches)
			}
		})
	}
}
