package httpapi

import (
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoginRateUsesTrustedClientAddress(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		t.Run(strconv.FormatBool(trusted), func(t *testing.T) {
			s := boundaryServer(t, io.Discard)
			s.loginRate = userRateLimiter(1, 1)
			if trusted {
				if err := s.router.SetTrustedProxies([]string{"127.0.0.1"}); err != nil {
					t.Fatal(err)
				}
			}
			// Invalid input reaches the real login limiter without consulting the DB.
			s.router.POST("/login-rate-test", s.login)
			for i := 0; i < 3; i++ {
				r := httptest.NewRequest("POST", "/login-rate-test", strings.NewReader("{}"))
				r.RemoteAddr = "127.0.0.1:50000"
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("X-Forwarded-For", "192.0.2."+strconv.Itoa(i+1))
				r.Header.Set("X-Real-IP", "198.51.100."+strconv.Itoa(i+1))
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, r)
				want := 400
				if !trusted && i > 0 {
					want = 429
				}
				if w.Code != want {
					t.Fatalf("request %d: %d want %d", i, w.Code, want)
				}
				if want == 429 && (w.Header().Get("Retry-After") == "" || !strings.Contains(w.Body.String(), "RATE_LIMITED")) {
					t.Fatalf("rate response: %v %s", w.Header(), w.Body)
				}
			}
		})
	}
}

func TestAuthenticatedRateBuckets(t *testing.T) {
	s := boundaryServer(t, io.Discard)
	s.writeRate = userRateLimiter(1, 1)
	s.streamRate = userRateLimiter(1, 1)
	var uid int32 = 7
	handler := func(c *gin.Context) {
		c.Set("user", User{ID: uid})
		if s.limitAuthenticatedRequest(c) {
			return
		}
		c.Status(204)
	}
	s.router.POST("/ordinary-write", handler)
	s.router.GET("/ordinary-read", handler)
	// Replace the business handlers only in this test router so the routing
	// classification can be tested separately from real DB authorization tests.
	r := gin.New()
	r.GET("/api/projects/:id/events", handler)
	r.GET("/api/projects/:id/realtime-channels/:channelId/events", handler)
	call := func(router *gin.Engine, method, path string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d", method, path, w.Code, want)
		}
	}
	call(s.router, "POST", "/ordinary-write", 204)
	call(s.router, "POST", "/ordinary-write", 429)
	call(s.router, "GET", "/ordinary-read", 204)
	call(r, "GET", "/api/projects/1/events", 204)
	call(r, "GET", "/api/projects/2/realtime-channels/3/events", 429)
	uid = 8
	call(s.router, "POST", "/ordinary-write", 204)
	call(r, "GET", "/api/projects/1/events", 204)
}
