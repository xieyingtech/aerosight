package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-chi/httprate"
)

func userRateLimiter(limit, fallback int) *httprate.RateLimiter {
	if limit <= 0 {
		limit = fallback
	}
	return httprate.NewRateLimiter(limit, time.Minute, httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED")
	}))
}
func (s *Server) limitUser(c *gin.Context, limiter *httprate.RateLimiter) bool {
	if limiter.RespondOnLimit(c.Writer, c.Request, strconv.Itoa(int(currentUser(c).ID))) {
		c.Abort()
		return true
	}
	return false
}
func (s *Server) limitAuthenticatedRequest(c *gin.Context) bool {
	if c.Request.Method == http.MethodGet {
		switch c.FullPath() {
		case "/api/projects/:id/events", "/api/projects/:id/realtime-channels/:channelId/events":
			return s.limitUser(c, s.streamRate)
		}
		return false
	}
	if c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
		return false
	}
	// These handlers classify a validated command before applying the same write
	// bucket. Emergency stop and return-home continue through normal authorization.
	switch c.FullPath() {
	case "/api/projects/:id/task-runs/:runId/control", "/api/projects/:id/devices/:deviceId/commands":
		return false
	}
	return s.limitUser(c, s.writeRate)
}
