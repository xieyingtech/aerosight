package httpapi

import (
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func acceptsGzip(value string) bool {
	wildcard := false
	for _, entry := range strings.Split(value, ",") {
		parts := strings.Split(entry, ";")
		name := strings.TrimSpace(parts[0])
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && key == "q" {
				q, err := strconv.ParseFloat(value, 64)
				if err != nil || q < 0 || q > 1 {
					quality = 0
				} else {
					quality = q
				}
			}
		}
		if name == "gzip" {
			return quality > 0
		}
		if name == "*" {
			wildcard = quality > 0
		}
	}
	return wildcard
}

func (s *Server) staticCompression() gin.HandlerFunc {
	compress := gzip.Gzip(gzip.DefaultCompression, gzip.WithCustomShouldCompressFn(func(c *gin.Context) bool {
		return c.Request.Method == http.MethodGet && c.GetHeader("Range") == "" && c.GetHeader("If-Range") == "" && c.GetHeader("Upgrade") == "" && acceptsGzip(c.GetHeader("Accept-Encoding"))
	}))
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if s.staticPages == nil || p == "/api" || strings.HasPrefix(p, "/api/") || p == "/algorithm-assets" || strings.HasPrefix(p, "/algorithm-assets/") {
			return
		}
		if _, legacy := legacyPageURL(c.Request.URL); legacy {
			return
		}
		text := strings.HasSuffix(p, "/")
		switch path.Ext(p) {
		case ".html", ".js", ".css", ".json", ".txt", ".svg", ".xml":
			text = true
		}
		if !text {
			return
		}
		// Identity responses also vary, so shared caches do not reuse the wrong encoding.
		c.Header("Vary", "Accept-Encoding")
		compress(c)
	}
}
