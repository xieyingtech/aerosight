package httpapi

import (
	"aerosight/server/internal/config"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func boundaryServer(t *testing.T, logs io.Writer) *Server {
	t.Helper()
	// These routes exercise HTTP middleware without querying a database.
	db, err := sql.Open("pgx", "postgresql://localhost/unused")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.HTTP{PublicOrigin: "http://frontend.test", Development: true, CSRFKey: bytes.Repeat([]byte{1}, 32), SessionLifetime: time.Hour, SessionIdle: time.Hour, LoginLimit: 10, RequestTimeout: time.Second}
	s, err := New(db, cfg, slog.New(slog.NewJSONHandler(logs, nil)))
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(); db.Close() })
	return s
}

func TestHTTPAccessLogsExcludeSensitiveInput(t *testing.T) {
	var logs bytes.Buffer
	s := boundaryServer(t, &logs)
	s.router.POST("/log-test/:value", func(c *gin.Context) {
		if c.Param("value") != "path-secret" || c.Query("signature") != "query-secret" || c.GetHeader("Authorization") != "Bearer header-secret" {
			t.Error("logging changed request")
		}
		body, _ := io.ReadAll(c.Request.Body)
		if string(body) != "body-secret" {
			t.Error("logging consumed request body")
		}
		_ = c.Error(errors.New("upstream error-secret"))
		c.Header("Set-Cookie", "response-secret")
		c.String(500, "response-body-secret")
	})
	r := httptest.NewRequest("POST", "/log-test/path-secret?signature=query-secret", strings.NewReader("body-secret"))
	r.Header.Set("X-Request-ID", "test-correlation-123")
	r.Header.Set("Authorization", "Bearer header-secret")
	r.Header.Set("Cookie", "session=cookie-secret")
	r.Header.Set("Referer", "https://example.test/referer-secret")
	r.Header.Set("User-Agent", "agent-secret")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 500 {
		t.Fatalf("status %d", w.Code)
	}
	for _, secret := range []string{"path-secret", "query-secret", "header-secret", "body-secret", "cookie-secret", "referer-secret", "agent-secret", "error-secret", "response-secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("leaked %s: %s", secret, logs.String())
		}
	}
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &record); err != nil {
		t.Fatal(err)
	}
	request := record["request"].(map[string]any)
	response := record["response"].(map[string]any)
	if request["route"] != "/log-test/:value" || request["method"] != "POST" || response["status"] != float64(500) || response["latency"] == nil || record["id"] != "test-correlation-123" || record["level"] != "ERROR" {
		t.Fatalf("missing metadata: %s", logs.String())
	}
}

func TestHTTPPanicBoundaries(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "stream"}[stream], func(t *testing.T) {
			var logs bytes.Buffer
			s := boundaryServer(t, &logs)
			s.router.GET("/panic-test", func(c *gin.Context) {
				if stream {
					c.Header("Content-Type", "text/event-stream")
					c.String(200, "data: first\n\n")
					c.Writer.Flush()
				}
				panic("panic-secret")
			})
			ts := httptest.NewServer(s.Handler())
			defer ts.Close()
			r, err := http.NewRequest("GET", ts.URL+"/panic-test", nil)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("X-Request-ID", "panic-correlation")
			response, err := ts.Client().Do(r)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			ts.Close() // Wait until request logging has finished before reading the buffer.
			if stream {
				if response.StatusCode != 200 || string(body) != "data: first\n\n" {
					t.Fatalf("corrupted stream: %d %q", response.StatusCode, body)
				}
			} else if response.StatusCode != 500 || !strings.Contains(string(body), "INTERNAL_ERROR") {
				t.Fatalf("ordinary panic: %d %q", response.StatusCode, body)
			}
			if response.Header.Get("X-Request-ID") != "panic-correlation" || !strings.Contains(logs.String(), `"request_id":"panic-correlation"`) || strings.Contains(logs.String(), "panic-secret") || strings.Contains(string(body), "panic-secret") {
				t.Fatalf("panic correlation/redaction: %s %s", body, logs.String())
			}
		})
	}
}

func TestHTTPInvalidCorrelationIDIsReplaced(t *testing.T) {
	var logs bytes.Buffer
	s := boundaryServer(t, &logs)
	for _, id := range []string{"", "invalid id", strings.Repeat("a", 129)} {
		r := httptest.NewRequest("GET", "/unknown", nil)
		r.Header.Set("X-Request-ID", id)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if got := w.Header().Get("X-Request-ID"); got == id || len(got) != 32 {
			t.Fatalf("invalid ID not replaced: %q", got)
		}
	}
}
