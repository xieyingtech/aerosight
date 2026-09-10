package webassets

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func exportFixture() fstest.MapFS {
	return fstest.MapFS{
		"index.html":                      {Data: []byte("<html>home</html>")},
		"login/index.html":                {Data: []byte("<html>login</html>")},
		"projects/index.html":             {Data: []byte("<html>projects</html>")},
		"404.html":                        {Data: []byte("<html>missing</html>")},
		"_next/static/chunks/abc123.js":   {Data: []byte("0123456789")},
		"_next/static/chunks/abc123.css":  {Data: []byte("body {}")},
		"_next/static/media/abc123.woff2": {Data: []byte("font")},
		"projects/__next._index.txt":      {Data: []byte("flight payload")},
	}
}
func TestStaticExportHTTP(t *testing.T) {
	h, err := New(exportFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, body, mime string }{
		{"/", "<html>home</html>", "text/html"}, {"/login/", "<html>login</html>", "text/html"},
		{"/_next/static/chunks/abc123.js", "0123456789", "text/javascript"},
		{"/_next/static/chunks/abc123.css", "body {}", "text/css"},
		{"/_next/static/media/abc123.woff2", "font", "font/woff2"},
		{"/projects/__next._index.txt", "flight payload", "text/plain"},
	} {
		for _, method := range []string{"GET", "HEAD"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(method, tc.path, nil))
			if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), tc.mime) {
				t.Fatalf("%s %s %d %+v", method, tc.path, w.Code, w.Header())
			}
			if method == "HEAD" && w.Body.Len() != 0 || method == "GET" && w.Body.String() != tc.body {
				t.Fatalf("body %s %s", method, tc.path)
			}
			cache := "no-cache"
			if strings.HasPrefix(tc.path, "/_next/static/") {
				cache = "public, max-age=31536000, immutable"
			}
			if w.Header().Get("Cache-Control") != cache || w.Header().Get("ETag") == "" || w.Header().Get("Content-Length") == "" {
				t.Fatalf("headers %+v", w.Header())
			}
			r := httptest.NewRequest("GET", tc.path, nil)
			r.Header.Set("If-None-Match", w.Header().Get("ETag"))
			cached := httptest.NewRecorder()
			h.ServeHTTP(cached, r)
			if cached.Code != 304 || cached.Body.Len() != 0 {
				t.Fatal("ETag revalidation")
			}
		}
	}
	r := httptest.NewRequest("GET", "/_next/static/chunks/abc123.js", nil)
	r.Header.Set("Range", "bytes=2-5")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "2345" || w.Header().Get("Content-Encoding") != "" || w.Header().Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("Range %+v %s", w.Header(), w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/projects?search=test", nil))
	if w.Code != 307 || w.Header().Get("Location") != "/projects/?search=test" {
		t.Fatal("canonical page redirect")
	}
	for _, target := range []string{"/unknown", "/_next/static/chunks/missing.js", "/_next/static/", "/../secret", "/%2e%2e/secret", "/a%5cb", "//secret", "/api/missing", "/algorithm-assets/secret"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
		if w.Code != 404 || strings.Contains(w.Body.String(), "home") {
			t.Fatalf("boundary %s %d %s", target, w.Code, w.Body.String())
		}
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/login/", nil))
	if w.Code != 405 {
		t.Fatal("static write accepted")
	}
}
func TestStaticExportMissingAndSymlink(t *testing.T) {
	files := exportFixture()
	delete(files, "login/index.html")
	if _, err := New(files); err == nil {
		t.Fatal("accepted missing page")
	}
	files = exportFixture()
	files["escape"] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("/secret")}
	if _, err := New(files); err == nil {
		t.Fatal("accepted symlink")
	}
}
