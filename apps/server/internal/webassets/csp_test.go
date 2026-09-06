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
