//go:build !dev

package webassets

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedProductionPages(t *testing.T) {
	h, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/login/", "/projects/", "/projects/tasks/runs/detail/?projectId=34&runId=56"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<html") || !strings.Contains(w.Body.String(), "/_next/static/") {
			t.Fatalf("embedded %s: status %d", path, w.Code)
		}
	}
}
