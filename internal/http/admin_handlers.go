package http

import (
	"net/http"

	"aerosight/internal/auth"
	"github.com/gin-gonic/gin"
)

func (s *Server) adminOverview(c *gin.Context) {
	item, err := s.store.Overview(c.Request.Context())
	respond(c, item, err)
}

func (s *Server) adminUsers(c *gin.Context) {
	items, err := s.store.ListAdminUsers(c.Request.Context())
	respond(c, items, err)
}

func (s *Server) adminCreateUser(c *gin.Context) {
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if !bindJSON(c, &body) {
		return
	}
	if optionalString(body.Name) == nil {
		abort(c, http.StatusBadRequest, "errors.validation.userName.required")
		return
	}
	if body.Role == "" {
		body.Role = "user"
	}
	if body.Password == "" {
		abort(c, http.StatusBadRequest, "errors.validation.password.required")
		return
	}
	password, err := auth.HashPassword(body.Password)
	if err != nil {
		abort(c, http.StatusInternalServerError, "errors.generic")
		return
	}
	items, err := s.store.CreateUser(c.Request.Context(), body.Name, optionalString(body.Email), optionalString(body.Phone), body.Role, password)
	respond(c, items, err)
}

func (s *Server) adminUpdateUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var body struct {
		Name     *string `json:"name"`
		Email    *string `json:"email"`
		Phone    *string `json:"phone"`
		Role     *string `json:"role"`
		Password *string `json:"password"`
	}
	if !bindJSON(c, &body) {
		return
	}
	var password *string
	if body.Password != nil {
		hash, err := auth.HashPassword(*body.Password)
		if err != nil {
			abort(c, http.StatusInternalServerError, "errors.generic")
			return
		}
		password = &hash
	}
	items, err := s.store.UpdateUser(c.Request.Context(), id, body.Name, body.Email, body.Phone, body.Role, password)
	respond(c, items, err)
}

func (s *Server) adminDeleteUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	items, err := s.store.DeleteUser(c.Request.Context(), id)
	respond(c, items, err)
}

func (s *Server) adminTeams(c *gin.Context) {
	items, err := s.store.ListAdminTeams(c.Request.Context())
	respond(c, items, err)
}

func (s *Server) adminCreateTeam(c *gin.Context) {
	user := currentUser(c)
	var body struct {
		Name        string `json:"name"`
		OwnerUserID int32  `json:"ownerUserId"`
	}
	if !bindJSON(c, &body) {
		return
	}
	if optionalString(body.Name) == nil {
		abort(c, http.StatusBadRequest, "errors.validation.teamName.required")
		return
	}
	if body.OwnerUserID == 0 {
		body.OwnerUserID = user.ID
	}
	exists, err := s.store.UserExists(c.Request.Context(), body.OwnerUserID)
	if err != nil || !exists {
		abort(c, http.StatusBadRequest, "errors.validation.ownerUserId.required")
		return
	}
	items, err := s.store.CreateTeam(c.Request.Context(), body.Name, body.OwnerUserID)
	respond(c, items, err)
}

func (s *Server) adminUpdateTeam(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var body struct {
		Name        *string `json:"name"`
		OwnerUserID *int32  `json:"ownerUserId"`
	}
	if !bindJSON(c, &body) {
		return
	}
	if body.OwnerUserID != nil {
		exists, err := s.store.UserExists(c.Request.Context(), *body.OwnerUserID)
		if err != nil || !exists {
			abort(c, http.StatusBadRequest, "errors.validation.ownerUserId.required")
			return
		}
	}
	items, err := s.store.UpdateTeam(c.Request.Context(), id, body.Name, body.OwnerUserID)
	respond(c, items, err)
}

func (s *Server) adminDeleteTeam(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	items, err := s.store.DeleteTeam(c.Request.Context(), id)
	respond(c, items, err)
}

func (s *Server) adminProjects(c *gin.Context) {
	items, err := s.store.ListAdminProjects(c.Request.Context())
	respond(c, items, err)
}

func (s *Server) adminCreateProject(c *gin.Context) {
	user := currentUser(c)
	var body struct {
		TeamID      int32  `json:"teamId"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !bindJSON(c, &body) {
		return
	}
	items, err := s.store.CreateProject(c.Request.Context(), body.TeamID, user.ID, body.Name, optionalString(body.Description))
	respond(c, items, err)
}

func (s *Server) adminUpdateProject(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var body struct {
		TeamID      *int32  `json:"teamId"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if !bindJSON(c, &body) {
		return
	}
	items, err := s.store.UpdateProject(c.Request.Context(), id, body.TeamID, body.Name, body.Description)
	respond(c, items, err)
}

func (s *Server) adminDeleteProject(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	items, err := s.store.DeleteProject(c.Request.Context(), id)
	respond(c, items, err)
}
