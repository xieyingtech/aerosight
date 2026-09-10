package httpapi

import (
	"aerosight/server/internal/webassets"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestStaticCompressionHTTPBoundary(t *testing.T) {
	s := boundaryServer(t, io.Discard)
	s.router.GET("/sse-test", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(200, "data: first\n\n")
		c.Writer.Flush()
	})
	text := strings.Repeat("static content ", 100)
	files := fstest.MapFS{}
	for _, name := range []string{"index.html", "login/index.html", "projects/index.html", "404.html", "_next/static/test.js"} {
		files[name] = &fstest.MapFile{Data: []byte(text)}
	}
	files["font.woff2"] = &fstest.MapFile{Data: []byte("binary")}
	pages, err := webassets.New(files)
	if err != nil {
		t.Fatal(err)
	}
	s.AttachStaticPages(pages)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	defer client.CloseIdleConnections()
	var etag string
	for _, tc := range []struct {
		method, path, encoding, rangeHeader string
		status                              int
		compressed                          bool
	}{
		{"GET", "/", "gzip", "", 200, true},
		{"GET", "/_next/static/test.js", "gzip", "", 200, true},
		{"GET", "/", "gzip;q=0, *;q=1", "", 200, false},
		{"GET", "/", "br", "", 200, false},
		{"HEAD", "/", "gzip", "", 200, false},
		{"GET", "/_next/static/test.js", "gzip", "bytes=0-5", 206, false},
		{"GET", "/font.woff2", "gzip", "", 200, false},
		{"GET", "/sse-test", "gzip", "", 200, false},
		{"GET", "/missing/", "gzip", "", 404, false},
		{"GET", "/api/unknown", "gzip", "", 404, false},
		{"GET", "/algorithm-assets/missing", "gzip", "", 404, false},
	} {
		r, _ := http.NewRequest(tc.method, ts.URL+tc.path, nil)
		r.Header.Set("Accept-Encoding", tc.encoding)
		if tc.rangeHeader != "" {
			r.Header.Set("Range", tc.rangeHeader)
		}
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		var reader io.Reader = res.Body
		if res.StatusCode != tc.status || (res.Header.Get("Content-Encoding") == "gzip") != tc.compressed {
			t.Fatalf("%+v: %d %v", tc, res.StatusCode, res.Header)
		}
		if tc.compressed {
			gz, err := gzip.NewReader(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			defer gz.Close()
			reader = gz
			etag = res.Header.Get("ETag")
			if !strings.HasPrefix(etag, "W/") {
				t.Fatalf("strong compressed etag %s", etag)
			}
		}
		body, err := io.ReadAll(reader)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if tc.compressed && string(body) != text {
			t.Fatal("gzip changed contents")
		}
		if tc.method == "HEAD" && len(body) != 0 {
			t.Fatal("HEAD body")
		}
		if tc.rangeHeader != "" && (string(body) != text[:6] || !strings.HasPrefix(res.Header.Get("Content-Range"), "bytes 0-5/")) {
			t.Fatal("range changed")
		}
	}
	r, _ := http.NewRequest("GET", ts.URL+"/_next/static/test.js", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	r.Header.Set("If-None-Match", etag)
	res, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || res.StatusCode != 304 || len(body) != 0 {
		t.Fatalf("conditional gzip %d %q %v", res.StatusCode, body, err)
	}
}
