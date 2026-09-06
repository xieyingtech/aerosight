package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strconv"
	"strings"
)

type User struct {
	ID    int32   `json:"id"`
	Name  string  `json:"name"`
	Email *string `json:"email"`
	Phone *string `json:"phone"`
	Role  string  `json:"role"`
}

func nullable(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}
func userDTO(r sqlcgen.GetUserRow) User {
	return User{r.ID, r.Name, nullable(r.Email), nullable(r.Phone), r.Role}
}
func (s *Server) authRoutes() {
	s.router.GET("/api/auth/csrf", func(c *gin.Context) { c.JSON(200, gin.H{"csrfToken": csrf.Token(c.Request)}) })
	s.router.POST("/api/auth/login", s.login)
	s.router.POST("/api/auth/logout", func(c *gin.Context) {
		if err := s.sessions.Destroy(c.Request.Context()); err != nil {
			s.failure(c, 500, "SESSION_FAILED")
			return
		}
		c.Status(204)
	})
	s.router.GET("/api/auth/session", s.requireUser, func(c *gin.Context) { c.JSON(200, gin.H{"user": currentUser(c)}) })
}
func (s *Server) login(c *gin.Context) {
	if s.loginRate.RespondOnLimit(c.Writer, c.Request, c.ClientIP()) {
		c.Abort()
		return
	}
	var in struct {
		Username string `json:"username" binding:"required,max=320"`
		Password string `json:"password" binding:"required,max=1024"`
	}
	if c.ShouldBindJSON(&in) != nil {
		s.failure(c, 400, "LOGIN_INPUT_INVALID")
		return
	}
	user, err := s.queries.FindLoginUser(c.Request.Context(), strings.TrimSpace(in.Username))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.failure(c, 500, "LOGIN_FAILED")
		return
	}
	if err != nil || !user.Password.Valid || bcrypt.CompareHashAndPassword([]byte(user.Password.String), []byte(in.Password)) != nil {
		s.failure(c, 401, "INVALID_CREDENTIALS")
		return
	}
	if err = s.sessions.RenewToken(c.Request.Context()); err != nil {
		s.failure(c, 500, "SESSION_FAILED")
		return
	}
	s.sessions.Put(c.Request.Context(), "userId", int(user.ID))
	for _, name := range []string{"authjs.session-token", "__Secure-authjs.session-token", "next-auth.session-token", "__Secure-next-auth.session-token"} {
		http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: strings.HasPrefix(name, "__Secure-"), SameSite: http.SameSiteLaxMode})
	}
	c.JSON(200, gin.H{"user": User{user.ID, user.Name, nullable(user.Email), nullable(user.Phone), user.Role}})
}
func (s *Server) requireUser(c *gin.Context) {
	id := s.sessions.GetInt(c.Request.Context(), "userId")
	if id <= 0 || id > 2147483647 {
		s.failure(c, 401, "UNAUTHENTICATED")
		return
	}
	user, err := s.queries.GetUser(c.Request.Context(), int32(id))
	if errors.Is(err, sql.ErrNoRows) {
		s.failure(c, 401, "UNAUTHENTICATED")
		return
	}
	if err != nil {
		s.failure(c, 500, "USER_LOOKUP_FAILED")
		return
	}
	c.Set("user", userDTO(user))
	c.Next()
}
func currentUser(c *gin.Context) User { return c.MustGet("user").(User) }
func (s *Server) requireAdmin(c *gin.Context) {
	if currentUser(c).Role != "admin" {
		s.failure(c, 403, "FORBIDDEN")
		return
	}
	c.Next()
}

var permissions = []string{"project:view", "event:handle", "issue:handle", "issue:assign", "mission:operate", "mission:approve", "device:configure", "safety:manage", "algorithm:manage", "agent:use", "report:export"}

func effectivePermissions(role string, explicit []string) map[string]bool {
	result := map[string]bool{"project:view": true}
	for _, p := range permissions {
		if role == "owner" || role == "admin" {
			result[p] = true
		}
		for _, grant := range explicit {
			if p == grant {
				result[p] = true
			}
		}
	}
	if result["event:handle"] {
		result["issue:handle"] = true
	}
	return result
}
func projectID(c *gin.Context) (int32, error) {
	n, err := strconv.ParseInt(c.Param("id"), 10, 32)
	if err != nil || n <= 0 {
		return 0, errors.New("PROJECT_NOT_FOUND")
	}
	return int32(n), nil
}
func (s *Server) projectAccess(ctx context.Context, q *sqlcgen.Queries, uid, pid int32, permission string) (sqlcgen.GetProjectAccessRow, error) {
	access, err := q.GetProjectAccess(ctx, sqlcgen.GetProjectAccessParams{UserID: uid, ProjectID: pid})
	if err != nil {
		return access, err
	}
	if !effectivePermissions(access.Role, access.Permissions)[permission] {
		return access, errors.New("PROJECT_ACCESS_DENIED")
	}
	return access, nil
}
