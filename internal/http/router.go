package http

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"aerosight/internal/auth"
	"aerosight/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Server struct {
	store    *store.Store
	sessions *auth.Manager
}

func NewRouter(store *store.Store, sessions *auth.Manager) http.Handler {
	server := &Server{store: store, sessions: sessions}
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), cors())

	api := router.Group("/api")
	api.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	api.POST("/auth/login", server.login)
	api.POST("/auth/logout", server.logout)
	api.GET("/auth/session", server.session)

	authenticated := api.Group("")
	authenticated.Use(server.requireUser())
	authenticated.GET("/profile", server.profile)
	authenticated.PATCH("/profile", server.updateProfile)
	authenticated.GET("/profile/teams", server.profileTeams)
	authenticated.GET("/teams", server.teams)
	authenticated.POST("/teams", server.createTeam)
	authenticated.GET("/teams/managed", server.managedTeams)
	authenticated.GET("/teams/:id", server.team)
	authenticated.GET("/projects", server.projects)
	authenticated.POST("/projects", server.createProjectForManagedTeam)
	authenticated.GET("/projects/:id", server.project)
	authenticated.GET("/projects/:id/devices", server.projectDevices)
	authenticated.GET("/projects/:id/agents", server.projectAgents)
	authenticated.GET("/projects/:id/tasks", server.projectTasks)
	authenticated.GET("/projects/:id/issues", server.projectIssues)
	authenticated.GET("/projects/:id/assets", server.projectAssets)

	admin := authenticated.Group("/admin")
	admin.Use(server.requireAdmin())
	admin.GET("/overview", server.adminOverview)
	admin.GET("/users", server.adminUsers)
	admin.POST("/users", server.adminCreateUser)
	admin.PATCH("/users/:id", server.adminUpdateUser)
	admin.DELETE("/users/:id", server.adminDeleteUser)
	admin.GET("/teams", server.adminTeams)
	admin.POST("/teams", server.adminCreateTeam)
	admin.PATCH("/teams/:id", server.adminUpdateTeam)
	admin.DELETE("/teams/:id", server.adminDeleteTeam)
	admin.GET("/projects", server.adminProjects)
	admin.POST("/projects", server.adminCreateProject)
	admin.PATCH("/projects/:id", server.adminUpdateProject)
	admin.DELETE("/projects/:id", server.adminDeleteProject)

	return router
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := os.Getenv("WEB_ORIGIN")
		if allowed == "" {
			allowed = "http://localhost:3000"
		}
		if origin == allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type")
			c.Header("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func (s *Server) requireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := s.sessions.Read(c.Request)
		if err != nil {
			abort(c, http.StatusUnauthorized, "errors.auth.forbidden")
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

func (s *Server) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := currentUser(c)
		if user.Role != "admin" {
			abort(c, http.StatusForbidden, "errors.auth.forbidden")
			return
		}
		c.Next()
	}
}

func (s *Server) requireProject(c *gin.Context) (auth.User, store.Project, bool) {
	user := currentUser(c)
	id, ok := idParam(c)
	if !ok {
		return user, store.Project{}, false
	}
	project, err := s.store.GetProjectForUser(c.Request.Context(), user.ID, id)
	if err != nil {
		status := http.StatusInternalServerError
		if err == pgx.ErrNoRows {
			status = http.StatusNotFound
		}
		abort(c, status, "errors.generic")
		return user, store.Project{}, false
	}
	return user, project, true
}

func currentUser(c *gin.Context) auth.User {
	value, _ := c.Get("user")
	user, _ := value.(auth.User)
	return user
}

func idParam(c *gin.Context) (int32, bool) {
	raw := c.Param("id")
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value <= 0 {
		abort(c, http.StatusBadRequest, "errors.validation.id.required")
		return 0, false
	}
	return int32(value), true
}

func abort(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"message": message})
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		abort(c, http.StatusBadRequest, "errors.generic")
		return false
	}
	return true
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
