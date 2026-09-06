package httpapi

import (
	"aerosight/server/internal/webassets"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestStaticPagesGinFallbackBoundary(t *testing.T) {
	files := fstest.MapFS{}
	for _, name := range []string{"index.html", "login/index.html", "projects/index.html", "404.html"} {
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
		{"/projects/42", 307, "text/html"},
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
