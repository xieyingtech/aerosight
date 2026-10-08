package media

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestS3ConfigurationRejectsPartialValues(t *testing.T) {
	for _, key := range []string{"S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY", "S3_REGION", "S3_SESSION_TOKEN", "S3_FORCE_PATH_STYLE"} {
		t.Setenv(key, "")
	}
	cfg, err := LoadS3Config()
	if err != nil || cfg.Endpoint != "" {
		t.Fatalf("empty config: %v", err)
	}
	t.Setenv("S3_ENDPOINT", "https://objects.example")
	if _, err = LoadS3Config(); err == nil {
		t.Fatal("partial config accepted")
	}
	t.Setenv("S3_BUCKET", "media")
	t.Setenv("S3_ACCESS_KEY_ID", "test")
	t.Setenv("S3_SECRET_ACCESS_KEY", "test-secret")
	cfg, err = LoadS3Config()
	if err != nil || cfg.Region != "us-east-1" || !cfg.ForcePathStyle {
		t.Fatalf("complete config: %+v %v", cfg, err)
	}
	t.Setenv("S3_ENDPOINT", "https://objects.example/bucket")
	if _, err = LoadS3Config(); err == nil {
		t.Fatal("endpoint path accepted")
	}
}

func TestS3SwitchPreservesLocalFilesOnlyForMissingObjects(t *testing.T) {
	root := t.TempDir()
	local, err := NewLocalObjectStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	key := "projects/17/imports/old.jpg"
	if _, err = local.PutObject(context.Background(), key, strings.NewReader("old-file"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	code := "NoSuchKey"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Amz-Error-Code", code)
		if code == "AccessDenied" {
			w.WriteHeader(403)
		} else {
			w.WriteHeader(404)
		}
		fmt.Fprintf(w, "<Error><Code>%s</Code></Error>", code)
	}))
	defer upstream.Close()
	store, err := NewConfiguredObjectStorage(root, S3Config{Endpoint: upstream.URL, Bucket: "media", AccessKeyID: "test", SecretAccessKey: "test-secret", Region: "us-east-1", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	object, err := store.GetObject(context.Background(), key)
	if err != nil || string(object.Body) != "old-file" {
		t.Fatalf("local image fallback: %v", err)
	}
	reader, err := OpenStoredProjectObject(context.Background(), store, root, 17, key)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(body) != "old-file" {
		t.Fatalf("local streaming fallback: %v", err)
	}
	code = "AccessDenied"
	if _, err = store.GetObject(context.Background(), key); err == nil {
		t.Fatal("remote permission error fell back to local")
	}
}

func TestS3SignedRoundTripAndVideoRange(t *testing.T) {
	var mu sync.Mutex
	var body []byte
	var contentType string
	rangeReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("request lacks S3 signature")
		}
		if r.URL.Path != "/media/projects/17/video.mp4" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case "PUT":
			var payload io.Reader = r.Body
			if strings.Contains(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING") {
				payload = httputil.NewChunkedReader(r.Body)
			}
			body, _ = io.ReadAll(payload)
			contentType = r.Header.Get("Content-Type")
			w.Header().Set("ETag", `"12345678901234567890123456789012"`)
			w.WriteHeader(200)
		case "HEAD", "GET":
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("ETag", `"12345678901234567890123456789012"`)
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			if r.Method == "HEAD" {
				return
			}
			if r.Header.Get("Range") != "" {
				rangeReads++
			}
			http.ServeContent(w, r, "video.mp4", time.Now(), bytes.NewReader(body))
		default:
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(405)
		}
	}))
	defer upstream.Close()
	store, err := NewS3ObjectStorage(S3Config{Endpoint: upstream.URL, Bucket: "media", AccessKeyID: "test", SecretAccessKey: "test-secret", Region: "us-east-1", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte("0123456789")
	written, err := store.PutObject(context.Background(), "projects/17/video.mp4", bytes.NewReader(expected), "video/mp4")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetObject(context.Background(), written.Key)
	if err != nil || !bytes.Equal(loaded.Body, expected) || loaded.ChecksumSHA256 != written.ChecksumSHA256 {
		t.Fatalf("round trip: %+v %v", loaded, err)
	}
	reader, err := OpenStoredProjectObject(context.Background(), store, "", 17, written.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err = reader.Seek(2, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 4)
	if _, err = io.ReadFull(reader, got); err != nil || string(got) != "2345" {
		t.Fatalf("seek: %q %v", got, err)
	}
	if _, err = OpenStoredProjectObject(context.Background(), store, "", 18, written.Key); err == nil {
		t.Fatal("cross-project read accepted")
	}
	if _, err = store.GetObject(context.Background(), "projects/17/../secret"); err == nil {
		t.Fatal("traversal accepted")
	}
	mu.Lock()
	defer mu.Unlock()
	if rangeReads == 0 {
		t.Fatal(fmt.Sprintf("no ranged reads: %d", rangeReads))
	}
}
