package algorithm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
)

type pinnedTestStore func(context.Context, string) (AlgorithmAsset, error)

func (f pinnedTestStore) ReadAlgorithmAsset(ctx context.Context, key string) (AlgorithmAsset, error) {
	return f(ctx, key)
}

func TestPinnedAssetAccessReadsOnlyMatchingContent(t *testing.T) {
	db := testdb.New(t)
	if _, err := migrations.Embedded(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var team, project, assetID int
	if err := db.QueryRow("insert into teams(name) values('pinned') returning id").Scan(&team); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("insert into projects(team_id,name) values($1,'pinned') returning id", team).Scan(&project); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("insert into assets(project_id,team_id,kind,mime_type,storage_key,logical_key) values($1,$2,'image','image/jpeg','projects/pinned.jpg','pinned') returning id", project, team).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	body := []byte("original image fixture")
	reads := 0
	store := pinnedTestStore(func(context.Context, string) (AlgorithmAsset, error) {
		reads++
		return AlgorithmAsset{Body: body, ContentType: "image/jpeg"}, nil
	})
	signer := NewAssetURLSigner(strings.Repeat("s", 32), "https://worker.example")
	sum := sha256.Sum256(body)
	checksum := hex.EncodeToString(sum[:])
	pinned, err := signer.IssuePinnedAssetURL(project, assetID, 1, checksum, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewAssetAccessHandler(db, store, signer)
	server := httptest.NewServer(handler)
	defer server.Close()
	serve := func(raw string, want int) {
		t.Helper()
		res, err := server.Client().Get(strings.Replace(raw, "https://worker.example", server.URL, 1))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		received, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("got %d want %d: %s", res.StatusCode, want, received)
		}
		if want == http.StatusOK && string(received) != string(body) {
			t.Fatal("image bytes were not delivered")
		}
		if want != http.StatusOK && strings.Contains(string(received), "replacement") {
			t.Fatal("mismatched bytes disclosed")
		}
	}
	serve(pinned, http.StatusOK)
	body = []byte("replacement image fixture")
	serve(pinned, http.StatusConflict)
	before := reads
	link, _ := url.Parse(pinned)
	values := link.Query()
	values.Del("checksum")
	link.RawQuery = values.Encode()
	serve(link.String(), http.StatusForbidden)
	values.Set("checksum", strings.Repeat("b", 64))
	link.RawQuery = values.Encode()
	serve(link.String(), http.StatusForbidden)
	if reads != before {
		t.Fatal("invalid signature accessed content")
	}
	legacy, err := signer.IssueAssetURL(project, assetID, 1, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	serve(legacy, http.StatusOK)
}
