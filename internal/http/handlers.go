package http

import (
	"net/http"

	"aerosight/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type loginRequest struct {
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

func (s *Server) login(c *gin.Context) {
	var body loginRequest
	if !bindJSON(c, &body) {
		return
	}
	email := optionalString(body.Email)
	phone := optionalString(body.Phone)
	if email == nil && phone == nil {
		abort(c, http.StatusBadRequest, "errors.validation.username.required")
		return
	}
	if body.Password == "" {
		abort(c, http.StatusBadRequest, "errors.validation.password.required")
		return
	}
	user, err := s.store.FindUserForLogin(c.Request.Context(), email, phone)
	if err != nil || user.Password == nil || !auth.VerifyPassword(*user.Password, body.Password) {
		abort(c, http.StatusUnauthorized, "errors.auth.invalidCredentials")
		return
	}
	sessionUser := auth.User{ID: user.ID, Name: user.Name, Role: user.Role}
	if err := s.sessions.Set(c.Writer, sessionUser); err != nil {
		abort(c, http.StatusInternalServerError, "errors.generic")
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": sessionUser})
}

func (s *Server) logout(c *gin.Context) {
	s.sessions.Clear(c.Writer)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) session(c *gin.Context) {
	user, err := s.sessions.Read(c.Request)
	if err != nil {
		abort(c, http.StatusUnauthorized, "errors.auth.forbidden")
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (s *Server) profile(c *gin.Context) {
	items, err := s.store.GetProfile(c.Request.Context(), currentUser(c).ID)
	respond(c, items, err)
}

func (s *Server) updateProfile(c *gin.Context) {
	var body struct {
		Name  *string `json:"name"`
		Email *string `json:"email"`
		Phone *string `json:"phone"`
	}
	if !bindJSON(c, &body) {
		return
	}
	items, err := s.store.UpdateProfile(c.Request.Context(), currentUser(c).ID, body.Name, body.Email, body.Phone)
	respond(c, items, err)
}

func (s *Server) profileTeams(c *gin.Context) {
	items, err := s.store.ListProfileTeams(c.Request.Context(), currentUser(c).ID)
	respond(c, items, err)
}

func (s *Server) managedTeams(c *gin.Context) {
	items, err := s.store.ListManagedTeams(c.Request.Context(), currentUser(c).ID)
	respond(c, items, err)
}

func (s *Server) projects(c *gin.Context) {
	items, err := s.store.ListProjects(c.Request.Context(), currentUser(c).ID, c.Query("scope"), c.Query("search"))
	respond(c, items, err)
}

func (s *Server) createProjectForManagedTeam(c *gin.Context) {
	user := currentUser(c)
	var body struct {
		TeamID int32  `json:"teamId"`
		Name   string `json:"name"`
	}
	if !bindJSON(c, &body) {
		return
	}
	if body.TeamID <= 0 {
		abort(c, http.StatusBadRequest, "errors.validation.teamId.required")
		return
	}
	if optionalString(body.Name) == nil {
		abort(c, http.StatusBadRequest, "errors.validation.projectName.required")
		return
	}
	ok, err := s.store.UserCanManageTeam(c.Request.Context(), user.ID, body.TeamID)
	if err != nil {
		abort(c, http.StatusInternalServerError, "errors.generic")
		return
	}
	if !ok {
		abort(c, http.StatusForbidden, "errors.auth.forbidden")
		return
	}
	items, err := s.store.CreateProject(c.Request.Context(), body.TeamID, user.ID, body.Name, nil)
	respond(c, items, err)
}

func (s *Server) project(c *gin.Context) {
	_, project, ok := s.requireProject(c)
	if ok {
		c.JSON(http.StatusOK, project)
	}
}

func (s *Server) projectDevices(c *gin.Context) {
	_, project, ok := s.requireProject(c)
	if !ok {
		return
	}
	items, err := s.store.ListDevices(c.Request.Context(), project.ID)
	respond(c, items, err)
}

func (s *Server) projectAgents(c *gin.Context) {
	_, project, ok := s.requireProject(c)
	if !ok {
		return
	}
	items, err := s.store.ListAgents(c.Request.Context(), project.ID)
	respond(c, items, err)
}

func (s *Server) projectTasks(c *gin.Context) {
	_, project, ok := s.requireProject(c)
	if !ok {
		return
	}
	items, err := s.store.ListTasks(c.Request.Context(), project.ID)
	respond(c, items, err)
}

func (s *Server) projectIssues(c *gin.Context) {
	_, project, ok := s.requireProject(c)
	if !ok {
		return
	}
	items, err := s.store.ListIssues(c.Request.Context(), project.ID)
	respond(c, items, err)
}

func (s *Server) projectAssets(c *gin.Context) {
	_, project, ok := s.requireProject(c)
	if !ok {
		return
	}
	items, err := s.store.ListAssets(c.Request.Context(), project.ID)
	respond(c, items, err)
}

func respond(c *gin.Context, data any, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		if err == pgx.ErrNoRows {
			status = http.StatusNotFound
		}
		abort(c, status, "errors.generic")
		return
	}
	c.JSON(http.StatusOK, data)
}
