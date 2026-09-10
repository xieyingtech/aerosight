package httpapi

import (
	"aerosight/server/internal/observability"
	"crypto/subtle"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) installMetrics() {
	registry := prometheus.NewRegistry()
	count := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aerosight_http_requests_total", Help: "HTTP requests by declared route."}, []string{"method", "route", "status"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "aerosight_http_request_duration_seconds", Help: "HTTP request duration.", Buckets: prometheus.DefBuckets}, []string{"method", "route"})
	registry.MustRegister(count, duration)
	s.router.Use(func(c *gin.Context) {
		started := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		method := c.Request.Method
		switch method {
		case "GET", "POST", "PATCH", "PUT", "DELETE", "HEAD", "OPTIONS":
		default:
			method = "OTHER"
		}
		count.WithLabelValues(method, route, strconv.Itoa(c.Writer.Status())).Inc()
		duration.WithLabelValues(method, route).Observe(time.Since(started).Seconds())
	})
	handler := promhttp.HandlerFor(prometheus.Gatherers{registry, observability.DefaultMetrics.Gatherer()}, promhttp.HandlerOpts{})
	s.router.GET("/metrics", func(c *gin.Context) {
		token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		tokenOK := strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") && s.cfg.MetricsToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.MetricsToken)) == 1
		if !tokenOK {
			s.requireUser(c)
			if c.IsAborted() {
				return
			}
			if currentUser(c).Role != "admin" {
				s.failure(c, http.StatusForbidden, "FORBIDDEN")
				return
			}
		}
		handler.ServeHTTP(c.Writer, c.Request)
	})
}
