package webassets

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type asset struct {
	data              []byte
	contentType, etag string
}
type Handler struct{ files map[string]asset }

func plainError(w http.ResponseWriter, r *http.Request, message string, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(message)+1))
	w.WriteHeader(status)
	if r.Method != "HEAD" {
		_, _ = w.Write([]byte(message + "\n"))
	}
}

// New loads only build-time files. Requests never access the host filesystem.
func New(source fs.FS) (*Handler, error) {
	h := &Handler{files: map[string]asset{}}
	err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular web asset: %s", name)
		}
		data, err := fs.ReadFile(source, name)
		if err != nil {
			return err
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		switch path.Ext(name) {
		case ".js":
			contentType = "text/javascript; charset=utf-8"
		case ".css":
			contentType = "text/css; charset=utf-8"
		case ".html":
			contentType = "text/html; charset=utf-8"
		case ".woff2":
			contentType = "font/woff2"
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		h.files[name] = asset{data, contentType, fmt.Sprintf(`"%x"`, sha256.Sum256(data))}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, required := range []string{"index.html", "login/index.html", "projects/index.html", "404.html"} {
		if len(h.files[required].data) == 0 {
			return nil, fmt.Errorf("missing static export: %s", required)
		}
	}
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		plainError(w, r, "Method not allowed", 405)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if strings.ContainsAny(name, "\\\x00") || strings.HasPrefix(name, "/") {
		plainError(w, r, "Not found", 404)
		return
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "." || segment == ".." {
			plainError(w, r, "Not found", 404)
			return
		}
	}
	if name == "api" || strings.HasPrefix(name, "api/") || name == "algorithm-assets" || strings.HasPrefix(name, "algorithm-assets/") {
		plainError(w, r, "Not found", 404)
		return
	}
	file, ok := h.files[name]
	if !ok {
		directory := strings.TrimSuffix(name, "/")
		index := directory + "/index.html"
		if directory == "" {
			index = "index.html"
		}
		file, ok = h.files[index]
		if ok && !strings.HasSuffix(r.URL.Path, "/") {
			destination := (&url.URL{Path: r.URL.Path + "/", RawQuery: r.URL.RawQuery}).String()
			http.Redirect(w, r, destination, http.StatusTemporaryRedirect)
			return
		}
	}
	if !ok {
		if path.Ext(name) == "" || path.Ext(name) == ".html" {
			file = h.files["404.html"]
			w.Header().Set("Content-Type", file.contentType)
			w.Header().Set("Content-Length", strconv.Itoa(len(file.data)))
			w.WriteHeader(404)
			if r.Method != "HEAD" {
				_, _ = w.Write(file.data)
			}
		} else {
			plainError(w, r, "Not found", 404)
		}
		return
	}
	if strings.HasPrefix(name, "_next/static/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Header().Set("Content-Type", file.contentType)
	w.Header().Set("ETag", file.etag)
	http.ServeContent(w, r, path.Base(name), time.Time{}, bytes.NewReader(file.data))
}
