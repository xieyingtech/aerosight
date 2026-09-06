package httpapi

import (
	"aerosight/server/internal/config"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/observability"
	"context"
	"database/sql"
	"encoding/json"
	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/httprate"
	"github.com/gorilla/csrf"
	sloggin "github.com/samber/slog-gin"
	"github.com/unrolled/secure"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type Server struct {
	credentialSecret string
	flightHub        *flightHubService
	ready            atomic.Bool
	router           *gin.Engine
	sessions         *scs.SessionManager
	store            *postgresstore.PostgresStore
	queries          *sqlcgen.Queries
	db               *sql.DB
	logger           *slog.Logger
	cfg              config.HTTP
	loginRate        *httprate.RateLimiter
}

func New(db *sql.DB, cfg config.HTTP, logger *slog.Logger) (*Server, error) {
	r := gin.New()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, err
	}
	sessions := scs.New()
	store := postgresstore.New(db)
	sessions.Store = store
	sessions.Cookie.Name = "aerosight_session"
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	sessions.Cookie.Secure = strings.HasPrefix(cfg.PublicOrigin, "https://")
	sessions.Lifetime = cfg.SessionLifetime
	sessions.IdleTimeout = cfg.SessionIdle
	s := &Server{router: r, sessions: sessions, store: store, queries: sqlcgen.New(db), db: db, logger: logger, cfg: cfg}
	s.loginRate = httprate.NewRateLimiter(cfg.LoginLimit, time.Minute, httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) { writeError(w, 429, "RATE_LIMITED") }))
	r.Use(requestid.New())
	r.Use(sloggin.NewWithConfig(logger, sloggin.Config{WithRequestID: true, WithRequestBody: false, WithResponseBody: false, WithRequestHeader: false, WithResponseHeader: false}))
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		logger.Error("request panic", "request_id", requestid.Get(c))
		if !c.Writer.Written() {
			s.failure(c, 500, "INTERNAL_ERROR")
		} else {
			c.Abort()
		}
	}))
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Header("Cache-Control", "no-store")
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		}
		c.Next()
	})
	r.NoRoute(func(c *gin.Context) { s.failure(c, 404, "NOT_FOUND") })
	r.NoMethod(func(c *gin.Context) { s.failure(c, 405, "METHOD_NOT_ALLOWED") })
	r.HandleMethodNotAllowed = true
	s.installMetrics()
	s.authRoutes()
	s.directoryRoutes()
	s.streamRoutes()
	s.projectReadRoutes()
	s.flightHubRoutes()
	s.deviceAdapterRoutes()
	s.router.POST("/api/projects/:id/device-adapters/discoveries/:identityId/bind", s.requireUser, s.timeout, s.bindDiscoveredDevice)
	s.router.POST("/api/projects/:id/devices/:deviceId/commands", s.requireUser, s.timeout, s.submitDeviceCommand)
	s.router.GET("/api/projects/:id/snapshot", s.requireUser, s.timeout, s.projectSnapshot)
	return s, nil
}
func (s *Server) Close() { s.store.StopCleanup() }
func (s *Server) Handler() http.Handler {
	origin, _ := url.Parse(s.cfg.PublicOrigin)
	protect := csrf.Protect(s.cfg.CSRFKey, csrf.TrustedOrigins([]string{origin.Host}), csrf.Secure(strings.HasPrefix(s.cfg.PublicOrigin, "https://")), csrf.Path("/"), csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeError(w, 403, "CSRF_FAILED") })))
	browser := s.sessions.LoadAndSave(protect(s.router))
	dispatch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Correlate even requests rejected before Gin routing.
		id := observability.CorrelationID(r.Header.Get("X-Request-ID"))
		r.Header.Set("X-Request-ID", id)
		w.Header().Set("X-Request-ID", id)
		if (strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/media-auth") || r.URL.Path == "/metrics" {
			w.Header().Set("Cache-Control", "no-store")
			if s.cfg.Development {
				r = csrf.PlaintextHTTPRequest(r)
			}
			// Unsafe requests must originate at the configured browser origin, including the development proxy.
			if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
				origin := r.Header.Get("Origin")
				if origin != "" && origin != s.cfg.PublicOrigin {
					writeError(w, 403, "CSRF_FAILED")
					return
				}
			}
			browser.ServeHTTP(w, r)
			return
		}
		s.router.ServeHTTP(w, r)
	})
	return secure.New(secure.Options{ContentTypeNosniff: true, FrameDeny: true, STSSeconds: 31536000, IsDevelopment: s.cfg.Development}).Handler(dispatch)
}
func (s *Server) failure(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, gin.H{"error": code})
}
func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
func (s *Server) timeout(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), s.cfg.RequestTimeout)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	c.Next()
}
