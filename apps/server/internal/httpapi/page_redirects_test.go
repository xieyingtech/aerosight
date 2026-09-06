package httpapi

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProjectPageURLScopeAndEncoding(t *testing.T) {
	parameters := url.Values{"projectId": {"99"}, "selected": {"a&b 中文"}, "layer": {"one", "two"}}
	href := projectPageURL(42, "/projects/assets/", parameters)
	u, err := url.Parse(href)
	if err != nil || u.Path != "/projects/assets/" || u.Query().Get("projectId") != "42" || u.Query().Get("selected") != "a&b 中文" || len(u.Query()["layer"]) != 2 {
		t.Fatal(href)
	}
	if parameters.Get("projectId") != "99" {
		t.Fatal("mutated caller parameters")
	}
}

func TestLegacyPageRedirects(t *testing.T) {
	id := "01234567-89ab-cdef-0123-456789abcdef"
	cases := map[string]string{
		"/teams/42":                          "/teams/detail/?teamId=42",
		"/projects/42":                       "/projects/detail/?projectId=42",
		"/projects/42/tasks/runs/56":         "/projects/tasks/runs/detail/?projectId=42&runId=56",
		"/projects/42/algorithms/runs/" + id: "/projects/algorithms/runs/detail/?projectId=42&runId=" + id,
		"/projects/42/issues/7":              "/projects/issues/detail/?issueId=7&projectId=42",
		"/projects/42/events/" + id:          "/projects/events/detail/?eventId=" + id + "&projectId=42",
	}
	for _, section := range []string{"tasks", "settings", "realtime", "assets", "issues", "events", "devices", "algorithms", "connectors", "agents"} {
		cases["/projects/42/"+section] = "/projects/" + section + "/?projectId=42"
	}
	router := gin.New()
	s := &Server{}
	router.NoRoute(s.pageNotFound)
	for old, target := range cases {
		for _, method := range []string{"GET", "HEAD"} {
			for _, suffix := range []string{"", "/"} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(method, old+suffix, nil))
				if w.Code != 307 || w.Header().Get("Location") != target || w.Header().Get("Cache-Control") != "no-cache" {
					t.Fatalf("%s %s: %d %+v", method, old+suffix, w.Code, w.Header())
				}
				if method == "HEAD" && w.Body.Len() != 0 {
					t.Fatal("HEAD has body")
				}
			}
		}
	}
	u, _ := url.Parse("/projects/42/tasks/runs/56?projectId=99&projectId=100&runId=70&layer=a&layer=b&label=%E4%B8%AD%E6%96%87%26x")
	target, ok := legacyPageURL(u)
	if !ok {
		t.Fatal("not mapped")
	}
	parsed, _ := url.Parse(target)
	q := parsed.Query()
	if q.Get("projectId") != "42" || len(q["projectId"]) != 1 || q.Get("runId") != "56" || strings.Join(q["layer"], ",") != "a,b" || q.Get("label") != "中文&x" {
		t.Fatal(target)
	}
	for _, path := range []string{"/projects/new", "/projects/detail/", "/projects/0", "/projects/-1", "/projects/01", "/projects/+1", "/projects/2147483648", "/projects/1e2", "/projects/42/unknown", "/projects/42/tasks/runs/0", "/projects/42/issues/2147483648", "/projects/42/events/not-uuid", "/projects/42/algorithms/runs/7", "/teams/42/extra", "//projects/42", "/api/projects/42", "/projects/42?bad=%ZZ"} {
		u, err := url.Parse(path)
		if err != nil {
			t.Fatal(err)
		}
		if target, ok := legacyPageURL(u); ok {
			t.Fatalf("invalid %s -> %s", path, target)
		}
	}
	for _, method := range []string{"POST", "PUT", "DELETE", "OPTIONS"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, "/projects/42", nil))
		if w.Code != 404 || w.Header().Get("Location") != "" {
			t.Fatalf("mutation redirect %s %d", method, w.Code)
		}
	}
	for _, value := range []string{"1", "2147483647"} {
		if !validPageID(value) {
			t.Fatal(value)
		}
	}
}
