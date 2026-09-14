package flighthub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"aerosight/server/internal/inspection"
)

func TestInspectionMediaCountersDistinguishAbsentFromZero(t *testing.T) {
	for _, tc := range []struct {
		folder             string
		listed, accessible int
		truncated          bool
		want               inspection.Completeness
	}{
		{`{}`, 0, 0, false, inspection.Partial},
		{`null`, 0, 0, false, inspection.Partial},
		{`{"expected_file_count":0}`, 0, 0, false, inspection.Partial},
		{`{"expected_file_count":null,"uploaded_file_count":0}`, 0, 0, false, inspection.Partial},
		{`{"expected_file_count":0,"uploaded_file_count":0}`, 0, 0, false, inspection.Complete},
		{`{"expected_file_count":2,"uploaded_file_count":1}`, 1, 1, false, inspection.Partial},
		{`{"expected_file_count":2,"uploaded_file_count":2}`, 1, 1, false, inspection.Partial},
		{`{"expected_file_count":2,"uploaded_file_count":2}`, 2, 1, false, inspection.Partial},
		{`{"expected_file_count":2,"uploaded_file_count":2}`, 2, 2, true, inspection.Partial},
		{`{"expected_file_count":2,"uploaded_file_count":2}`, 2, 2, false, inspection.Complete},
	} {
		t.Run(fmt.Sprintf("%s/%d/%d/%v", tc.folder, tc.listed, tc.accessible, tc.truncated), func(t *testing.T) {
			var task FlightTask
			if err := json.Unmarshal([]byte(`{"status":"success","folder_info":`+tc.folder+`}`), &task); err != nil {
				t.Fatal(err)
			}
			if got := InspectionMediaStatus(task, tc.listed, tc.accessible, tc.truncated).Completeness(); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
			task.Status = "running"
			if got := InspectionMediaStatus(task, tc.listed, tc.accessible, tc.truncated).Completeness(); got == inspection.Complete {
				t.Fatal("running flight treated as complete")
			}
		})
	}
	for _, value := range []string{`{"expected_file_count":-1}`, `{"uploaded_file_count":-1}`, `{"expected_file_count":"0"}`} {
		var folder FlightTaskFolderInfo
		if json.Unmarshal([]byte(value), &folder) == nil {
			t.Fatal("invalid counters accepted")
		}
	}
	// Decoder reuse must not retain known counters from the previous response.
	var folder FlightTaskFolderInfo
	if err := json.Unmarshal([]byte(`{"expected_file_count":2,"uploaded_file_count":2}`), &folder); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{}`), &folder); err != nil {
		t.Fatal(err)
	}
	if folder.CountsKnown {
		t.Fatal("stale counter presence")
	}
}

func TestInspectionMediaHashUsesValidatedBoundedUnauthenticatedDownload(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Project-Uuid") != "" {
			t.Error("API credentials forwarded to media host")
		}
		switch r.URL.Path {
		case "/ok":
			fmt.Fprint(w, "image-bytes")
		case "/chunked":
			w.(http.Flusher).Flush()
			fmt.Fprint(w, strings.Repeat("x", 20))
		case "/empty":
		case "/expired":
			w.WriteHeader(http.StatusForbidden)
		case "/redirect":
			http.Redirect(w, r, "/ok", http.StatusFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	host, _ := url.Parse(server.URL)
	client, err := NewChinaClient(Config{HTTPClient: server.Client(), AllowedLinkHosts: []string{host.Hostname()}, RequestID: func() string { return "test" }})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path  string
		limit int64
		code  string
	}{
		{"/ok", 20, ""}, {"/ok", 3, "media_size_limit_exceeded"}, {"/chunked", 3, "media_size_limit_exceeded"}, {"/empty", 20, "media_empty"}, {"/expired", 20, "temporary_link_expired"}, {"/redirect", 20, "media_read_failed"}, {"/failed", 20, "media_read_failed"},
	} {
		t.Run(tc.path+tc.code, func(t *testing.T) {
			body, readErr := client.ReadInspectionMedia(context.Background(), TemporaryDownload{URL: server.URL + tc.path, ExpiresAt: time.Now().Add(time.Minute)}, tc.limit)
			if tc.code != "" {
				if !IsSafeCode(readErr, tc.code) || body != nil {
					t.Fatalf("bounded read: %v", readErr)
				}
			} else if readErr != nil || string(body) != "image-bytes" {
				t.Fatalf("read: %q %v", body, readErr)
			}
			digest, err := client.HashInspectionMedia(context.Background(), TemporaryDownload{URL: server.URL + tc.path, ExpiresAt: time.Now().Add(time.Minute)}, tc.limit)
			if tc.code != "" {
				if !IsSafeCode(err, tc.code) {
					t.Fatalf("got %v want %s", err, tc.code)
				}
				return
			}
			sum := sha256.Sum256([]byte("image-bytes"))
			if err != nil || digest != hex.EncodeToString(sum[:]) {
				t.Fatalf("bad digest %s %v", digest, err)
			}
		})
	}
	before := calls
	for _, download := range []TemporaryDownload{{URL: "https://forbidden.example/image", ExpiresAt: time.Now().Add(time.Minute)}, {URL: server.URL + "/ok", ExpiresAt: time.Now().Add(-time.Minute)}} {
		if _, err := client.HashInspectionMedia(context.Background(), download, 20); err == nil {
			t.Fatal("untrusted or expired URL accepted")
		}
	}
	if calls != before {
		t.Fatal("invalid link caused request")
	}
}

func TestInspectionMediaDirectoryCapDoesNotReturnPartialList(t *testing.T) {
	items := make([]FlightTaskMedia, maxFlightTaskMedia)
	for n := range items {
		items[n] = FlightTaskMedia{UUID: fmt.Sprintf("media-%d", n), Name: "fixture", FileType: "image", Suffix: "jpg", SizeBytes: 1, OriginalURL: "https://objects.vendor.example/image", CreatedAt: "2026-09-01T10:00:00Z", UpdatedAt: "2026-09-01T10:00:00Z"}
	}
	body, err := json.Marshal(map[string]any{"code": 0, "message": "", "data": map[string]any{"list": items}})
	if err != nil {
		t.Fatal(err)
	}
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return response(http.StatusOK, body, nil), nil }), func(c *Config) {
		c.MaxResponseBytes = 16 << 20
		c.AllowedLinkHosts = []string{"objects.vendor.example"}
	})
	result, err := client.ListFlightTaskMedia(context.Background(), "TOKEN_REDACTED", "11111111-1111-4111-8111-111111111111", "fixture-flight")
	if !IsSafeCode(err, "directory_limit_reached") || result != nil {
		t.Fatalf("capped list returned usable evidence: len=%d err=%v", len(result), err)
	}
}
