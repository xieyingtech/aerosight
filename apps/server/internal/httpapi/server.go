package httpapi

import (
	"aerosight/server/internal/config"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/device"
	"aerosight/server/internal/observability"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/httprate"
	"github.com/gorilla/csrf"
	sloggin "github.com/samber/slog-gin"
	"github.com/unrolled/secure"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type Server struct {
	staticPages         http.Handler
	aiHTTPClientFactory func(*url.URL, []netip.Addr) *http.Client
	mediaStorageRoot    string
	networkResolver     device.HostResolver
	networkProbe        device.EndpointProbe
	credentialSecret    string
	flightHub           *flightHubService
	ready               atomic.Bool
	router              *gin.Engine
	sessions            *scs.SessionManager
	store               *postgresstore.PostgresStore
	queries             *sqlcgen.Queries
	db                  *sql.DB
	logger              *slog.Logger
	cfg                 config.HTTP
	loginRate           *httprate.RateLimiter
	writeRate           *httprate.RateLimiter
	streamRate          *httprate.RateLimiter
}

func New(db *sql.DB, cfg config.HTTP, logger *slog.Logger) (*Server, error) {
	r := gin.New()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, err
	}
	sessions := scs.New()
	store := postgresstore.New(db)
	sessionTimeout := cfg.RequestTimeout
	if sessionTimeout <= 0 {
		sessionTimeout = 30 * time.Second
	}
	sessions.Store = &sessionStore{PostgresStore: store, queries: sqlcgen.New(db), timeout: sessionTimeout}
	sessions.ErrorFunc = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("session operation failed", "request_id", r.Header.Get("X-Request-ID"))
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(w, 504, "REQUEST_TIMEOUT")
			return
		}
		writeError(w, 500, "SESSION_FAILED")
	}
	sessions.Cookie.Name = "aerosight_session"
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	sessions.Cookie.Secure = strings.HasPrefix(cfg.PublicOrigin, "https://")
	sessions.Lifetime = cfg.SessionLifetime
	sessions.IdleTimeout = cfg.SessionIdle
	s := &Server{router: r, sessions: sessions, store: store, queries: sqlcgen.New(db), db: db, logger: logger, cfg: cfg}
	s.loginRate = httprate.NewRateLimiter(cfg.LoginLimit, time.Minute, httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) { writeError(w, 429, "RATE_LIMITED") }))
	s.writeRate = userRateLimiter(cfg.WriteLimit, 120)
	s.streamRate = userRateLimiter(cfg.SSELimit, 30)
	r.Use(requestid.New())
	r.Use(sloggin.NewWithConfig(slog.New(accessLogHandler{logger.Handler()}), sloggin.Config{WithRequestID: true, ClientErrorLevel: slog.LevelWarn, ServerErrorLevel: slog.LevelError, WithCustomMessage: func(*gin.Context) string { return "HTTP request" }}))
	r.Use(gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
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
	r.NoRoute(s.staticCompression(), s.pageNotFound)
	r.NoMethod(func(c *gin.Context) { s.failure(c, 405, "METHOD_NOT_ALLOWED") })
	r.HandleMethodNotAllowed = true
	s.installMetrics()
	s.authRoutes()
	s.directoryRoutes()
	s.streamRoutes()
	s.projectReadRoutes()
	s.flightHubRoutes()
	s.deviceAdapterRoutes()
	s.missionReadRoutes()
	s.legacyEventRoutes()
	s.issueReadRoutes()
	s.algorithmRunRoutes()
	s.algorithmDefinitionRoutes()
	s.algorithmProviderRoutes()
	s.aiProviderRoutes()
	s.agentSessionRoutes()
	s.mediaAccessRoutes()
	s.router.POST("/api/media-auth", s.timeout, s.mediaAuth)
	s.router.GET("/api/projects/:id/live-streams/:streamId/playback", s.requireUser, s.timeout, s.getLivePlayback)
	s.router.POST("/api/projects/:id/live-streams/:streamId/stop", s.requireUser, s.timeout, s.stopLiveStream)
	s.router.POST("/api/projects/:id/devices/:deviceId/live-streams", s.requireUser, s.timeout, s.startLiveStream)
	s.router.POST("/api/projects/:id/task-runs/:runId/reports", s.requireUser, s.timeout, s.createReportDraft)
	s.router.POST("/api/projects/:id/reports/:reportId/publish", s.requireUser, s.timeout, s.publishReport)
	s.router.GET("/api/projects/:id/reports/:reportId/export", s.requireUser, s.timeout, s.exportReport)
	s.router.POST("/api/projects/:id/task-runs/:runId/control", s.requireUser, s.timeout, s.controlMissionRun)
	s.router.GET("/api/projects/:id/task-runs/:runId/audit-trace", s.requireUser, s.timeout, s.getMissionAuditTrace)
	s.router.POST("/api/projects/:id/task-runs/:runId/emergency-stop-drill", s.requireUser, s.timeout, s.runEmergencyStopDrill)
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
		r = withConnectionController(r, w)
		// Correlate even requests rejected before Gin routing.
		id := observability.CorrelationID(r.Header.Get("X-Request-ID"))
		r.Header.Set("X-Request-ID", id)
		w.Header().Set("X-Request-ID", id)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			// Apply before CSRF, which can parse form bodies before Gin runs.
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/callbacks/algorithms/") {
			if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
				controller := http.NewResponseController(w)
				_ = controller.SetReadDeadline(time.Now().Add(s.cfg.RequestTimeout))
			}
		}
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
	if c.Request.Context().Err() == context.DeadlineExceeded {
		status, code = http.StatusGatewayTimeout, "REQUEST_TIMEOUT"
	}
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
