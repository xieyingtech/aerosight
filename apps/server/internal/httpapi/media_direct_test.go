package httpapi

import (
	"aerosight/server/internal/media"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type directPlaybackStore struct {
	media.ObjectStorage
	calls int
	key   string
}

func (s *directPlaybackStore) PresignRead(ctx context.Context, key string, ttl time.Duration) (*media.Access, error) {
	s.calls++
	s.key = key
	return &media.Access{URL: "https://private.objects.example/video.mp4?signed=test", ExpiresAt: time.Now().Add(ttl).UTC().Format(time.RFC3339)}, nil
}

func TestDirectVideoPlaybackRequiresProjectAuthorization(t *testing.T) {
	f := newAPIFixture(t)
	f.server.AttachDeviceCredentials(strings.Repeat("s", 32))
	team, pid := f.project(t)
	local, e := media.NewLocalObjectStorage(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	store := &directPlaybackStore{ObjectStorage: local}
	f.server.AttachObjectStorage(store)
	var id int
	key := fmt.Sprintf("projects/%d/clip.mp4", pid)
	if e = f.db.QueryRow(`insert into assets(project_id,team_id,kind,storage_key,logical_key,mime_type) values($1,$2,'video',$3,'clip','video/mp4') returning id`, pid, team, key).Scan(&id); e != nil {
		t.Fatal(e)
	}
	response := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/assets/%d/access?action=play", pid, id), "")
	data := decodedResponse(t, response)
	if response.StatusCode != 200 || !strings.HasPrefix(data["url"].(string), "https://private.objects.example/") || store.calls != 1 || store.key != key {
		t.Fatalf("direct access: status %d calls %d data %v", response.StatusCode, store.calls, data)
	}
	if response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatal("signed URL may be cached")
	}
	_, other := f.project(t)
	response = f.request(t, "GET", fmt.Sprintf("/api/projects/%d/assets/%d/access?action=play", other, id), "")
	response.Body.Close()
	if response.StatusCode == 200 || store.calls != 1 {
		t.Fatal("cross-project object signed")
	}
	response = f.request(t, "GET", fmt.Sprintf("/api/projects/%d/assets/%d/access?action=download", pid, id), "")
	data = decodedResponse(t, response)
	if response.StatusCode != 200 || !strings.HasPrefix(data["url"].(string), "/api/projects/") || store.calls != 1 {
		t.Fatal("download bypassed gateway")
	}
}
