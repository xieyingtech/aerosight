package httpapi

import (
	"aerosight/server/internal/webassets"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestWorkspaceResourcePoliciesThroughHTTPBoundary(t *testing.T) {
	files := fstest.MapFS{}
	for _, name := range []string{"index.html", "login/index.html", "projects/index.html", "projects/realtime/index.html", "rtc-viewer/index.html", "404.html"} {
		files[name] = &fstest.MapFile{Data: []byte("<html>" + name + "</html>")}
	}
	pages, err := webassets.New(files)
	if err != nil {
		t.Fatal(err)
	}
	s := boundaryServer(t, io.Discard)
	s.AttachStaticPages(pages)
	handler := s.Handler()
	for _, tc := range []struct {
		path, frameOptions, ancestors string
		status                        int
	}{
		{"/rtc-viewer/", "SAMEORIGIN", "'self'", 200},
		{"/rtc-viewer/index.html", "SAMEORIGIN", "'self'", 200},
		{"/projects/", "DENY", "'none'", 200},
		{"/projects/realtime/", "DENY", "'none'", 200},
		{"/projects/missing/", "DENY", "'none'", 404},
		{"/login/", "DENY", "'none'", 200},
		{"/", "DENY", "'none'", 200},
		{"/rtc-viewer/missing/", "DENY", "'none'", 404},
	} {
		for _, method := range []string{"GET", "HEAD"} {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(method, tc.path, nil))
			policy := w.Header().Get("Content-Security-Policy")
			if w.Code != tc.status || w.Header().Get("X-Frame-Options") != tc.frameOptions || !strings.Contains(policy, "frame-ancestors "+tc.ancestors+";") {
				t.Fatalf("%s %s: %d %+v", method, tc.path, w.Code, w.Header())
			}
			projectPage := strings.HasPrefix(tc.path, "/projects/") && tc.status == 200
			workspace := projectPage || tc.frameOptions == "SAMEORIGIN"
			for _, directive := range strings.Split(policy, ";") {
				fields := strings.Fields(directive)
				if len(fields) == 0 {
					continue
				}
				wantHTTPS := workspace && (fields[0] == "connect-src" || fields[0] == "font-src" || fields[0] == "img-src" || fields[0] == "media-src") || projectPage && fields[0] == "frame-src"
				wantWSS := workspace && fields[0] == "connect-src"
				if strings.Contains(" "+strings.Join(fields, " ")+" ", " https: ") != wantHTTPS || strings.Contains(" "+strings.Join(fields, " ")+" ", " wss: ") != wantWSS {
					t.Fatalf("%s: incorrect secure resource scope: %s", tc.path, directive)
				}
			}
			if tc.status == 200 {
				r := httptest.NewRequest(method, tc.path, nil)
				r.Header.Set("If-None-Match", w.Header().Get("ETag"))
				cached := httptest.NewRecorder()
				handler.ServeHTTP(cached, r)
				if cached.Code != 304 || cached.Header().Get("X-Frame-Options") != tc.frameOptions || cached.Header().Get("Content-Security-Policy") != policy {
					t.Fatalf("%s %s: cached response lost frame policy: %d %+v", method, tc.path, cached.Code, cached.Header())
				}
			}
		}
	}
}

func TestStaticPagesGinFallbackBoundary(t *testing.T) {
	files := fstest.MapFS{}
	for _, name := range []string{"index.html", "login/index.html", "projects/index.html", "projects/detail/index.html", "404.html"} {
		files[name] = &fstest.MapFile{Data: []byte("<html>" + name + "</html>")}
	}
	pages, err := webassets.New(files)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	s.AttachStaticPages(pages)
	r := gin.New()
	r.NoRoute(s.pageNotFound)
	for _, tc := range []struct {
		path   string
		status int
		kind   string
	}{
		{"/", 200, "text/html"}, {"/login/", 200, "text/html"}, {"/missing", 404, "text/html"},
		{"/api/missing", 404, "application/json"}, {"/algorithm-assets/missing", 404, "application/json"},
		{"/projects/42", 200, "text/html"},
	} {
		for _, method := range []string{"GET", "HEAD"} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(method, tc.path, nil))
			if w.Code != tc.status || method == "GET" && !strings.HasPrefix(w.Header().Get("Content-Type"), tc.kind) {
				t.Fatalf("%s %s %d %+v", method, tc.path, w.Code, w.Header())
			}
			if method == "HEAD" && !strings.HasPrefix(tc.path, "/api") && !strings.HasPrefix(tc.path, "/algorithm-assets") && w.Body.Len() != 0 {
				t.Fatal("HEAD body")
			}
		}
	}
}
