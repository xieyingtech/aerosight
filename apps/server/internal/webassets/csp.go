package webassets

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"sort"
	"strings"

	"github.com/unrolled/secure"
	"golang.org/x/net/html"
)

type cspEntry struct {
	Digest string   `json:"digest"`
	Hashes []string `json:"hashes"`
}

// ConfigureCSP must run before serving requests. Sources come from validated
// application configuration; they never come from request headers or URLs.
func (h *Handler) ConfigureCSP(mapOrigins, mediaOrigins []string) {
	for name, file := range h.files {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		scripts := "'self'"
		for _, hash := range scriptHashes(file.data) {
			scripts += " '" + hash + "'"
		}
		maps := strings.Join(mapOrigins, " ")
		media := strings.Join(mediaOrigins, " ")
		policy := "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; " +
			"script-src " + scripts + "; script-src-attr 'none'; " +
			"style-src 'self' 'unsafe-inline'; font-src 'self' " + maps + "; " +
			"img-src 'self' data: blob: " + maps + " " + media + "; " +
			"connect-src 'self' " + maps + " " + media + "; " +
			"worker-src 'self' blob:; media-src 'self' blob: " + media + "; frame-src 'self' " + media + ";"
		file.policy = secure.New(secure.Options{ContentSecurityPolicy: policy, ReferrerPolicy: "same-origin"})
		h.files[name] = file
	}
}

// Read the actual script text independently of the build-time extractor.
// Hashes cover raw script text, not HTML-escaped attribute values.
func scriptHashes(data []byte) []string {
	tokens := html.NewTokenizer(strings.NewReader(string(data)))
	inside := false
	hashes := map[string]bool{}
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			result := make([]string, 0, len(hashes))
			for hash := range hashes {
				result = append(result, hash)
			}
			sort.Strings(result)
			return result
		case html.StartTagToken:
			name, _ := tokens.TagName()
			inside = string(name) == "script"
		case html.EndTagToken:
			inside = false
		case html.TextToken:
			if inside {
				body := tokens.Text()
				if len(body) > 0 {
					sum := sha256.Sum256(body)
					hashes["sha256-"+base64.StdEncoding.EncodeToString(sum[:])] = true
				}
			}
		}
	}
}

func (h *Handler) validateCSP(source fs.FS) error {
	file, err := source.Open("csp-manifest.json")
	if err != nil {
		return fmt.Errorf("missing CSP build manifest: %w", err)
	}
	defer file.Close()
	var manifest struct {
		Version int                 `json:"version"`
		Pages   map[string]cspEntry `json:"pages"`
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("invalid CSP manifest: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing CSP manifest data")
	}
	if manifest.Version != 1 {
		return fmt.Errorf("unsupported CSP manifest version")
	}
	count := 0
	for name, asset := range h.files {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		count++
		entry, ok := manifest.Pages[name]
		if !ok || entry.Digest != fmt.Sprintf("%x", sha256.Sum256(asset.data)) || !reflect.DeepEqual(entry.Hashes, scriptHashes(asset.data)) {
			return fmt.Errorf("CSP manifest does not match HTML: %s", name)
		}
	}
	if len(manifest.Pages) != count {
		return fmt.Errorf("CSP manifest contains unknown pages")
	}
	return nil
}
