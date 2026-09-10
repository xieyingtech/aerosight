package webassets

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCSPManifestRejectsDrift(t *testing.T) {
	for _, change := range []string{"valid", "missing", "body", "hash", "digest", "extra", "version"} {
		t.Run(change, func(t *testing.T) {
			files := exportFixture()
			files["index.html"] = &fstest.MapFile{Data: []byte(`<html><script>window.ready = true;</script></html>`)}
			pages := map[string]cspEntry{}
			for name, file := range files {
				if strings.HasSuffix(name, ".html") {
					pages[name] = cspEntry{fmt.Sprintf("%x", sha256.Sum256(file.Data)), scriptHashes(file.Data)}
				}
			}
			version := 1
			switch change {
			case "body":
				files["index.html"].Data = []byte(`<script>window.changed = true;</script>`)
			case "hash":
				entry := pages["index.html"]
				entry.Hashes = []string{}
				pages["index.html"] = entry
			case "digest":
				entry := pages["index.html"]
				entry.Digest = "invalid"
				pages["index.html"] = entry
			case "extra":
				pages["unknown.html"] = pages["index.html"]
			case "version":
				version = 2
			}
			data, err := json.Marshal(map[string]any{"version": version, "pages": pages})
			if err != nil {
				t.Fatal(err)
			}
			if change != "missing" {
				files["csp-manifest.json"] = &fstest.MapFile{Data: data}
			}
			h, err := New(files)
			if err != nil {
				t.Fatal(err)
			}
			err = h.validateCSP(files)
			if (err == nil) != (change == "valid") {
				t.Fatalf("validation: %v", err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/csp-manifest.json", nil))
			if w.Code != 404 {
				t.Fatalf("build manifest served: %d", w.Code)
			}
		})
	}
}

func TestPageCSPHeadersAndCaching(t *testing.T) {
	files := exportFixture()
	files["index.html"] = &fstest.MapFile{Data: []byte(`<script>window.home = true;</script>`)}
	files["login/index.html"] = &fstest.MapFile{Data: []byte(`<script>window.login = true;</script>`)}
	files["404.html"] = &fstest.MapFile{Data: []byte(`<script>window.missing = true;</script>`)}
	h, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	h.ConfigureCSP([]string{"https://maps.example"}, []string{"https://media.example:8889"})
	for _, tc := range []struct {
		path, file string
		status     int
	}{
		{"/", "index.html", 200}, {"/login/", "login/index.html", 200}, {"/missing", "404.html", 404},
	} {
		for _, method := range []string{"GET", "HEAD"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(method, tc.path, nil))
			policy := w.Header().Get("Content-Security-Policy")
			if w.Code != tc.status {
				t.Fatalf("%s: %d", tc.path, w.Code)
			}
			for _, hash := range scriptHashes(files[tc.file].Data) {
				if !strings.Contains(policy, "'"+hash+"'") {
					t.Fatalf("missing page script hash: %s", policy)
				}
			}
			for _, required := range []string{"script-src-attr 'none'", "worker-src 'self' blob:", "frame-src 'self' https://media.example:8889", "frame-ancestors 'none'", "connect-src 'self' https://maps.example https://media.example:8889"} {
				if !strings.Contains(policy, required) {
					t.Fatalf("missing %s: %s", required, policy)
				}
			}
			for _, directive := range strings.Split(policy, ";") {
				if strings.HasPrefix(strings.TrimSpace(directive), "script-src ") && (strings.Contains(directive, "unsafe-") || strings.Contains(directive, "https://")) {
					t.Fatalf("script source expanded: %s", directive)
				}
			}
			if tc.file != "index.html" && strings.Contains(policy, scriptHashes(files["index.html"].Data)[0]) {
				t.Fatal("another page's hash leaked into policy")
			}
			if tc.status == 200 {
				req := httptest.NewRequest(method, tc.path, nil)
				req.Header.Set("If-None-Match", w.Header().Get("ETag"))
				cached := httptest.NewRecorder()
				h.ServeHTTP(cached, req)
				if cached.Code != 304 || cached.Header().Get("Content-Security-Policy") != policy {
					t.Fatal("304 lost policy")
				}
			}
		}
	}
}
