package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"math"
	"strconv"
	"strings"
	"time"
)

func timestamp(v time.Time) string { return v.UTC().Format("2006-01-02T15:04:05.000Z") }
func nullInt(v sql.NullInt32) any {
	if !v.Valid {
		return nil
	}
	return v.Int32
}
func stringList(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
func mapRows[T any](rows []T, convert func(T) gin.H) []gin.H {
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, convert(r))
	}
	return out
}
func teamDTO(r sqlcgen.ListTeamsRow) gin.H {
	return gin.H{"id": r.ID, "name": r.Name, "role": r.Role, "memberCount": r.MemberCount, "createdAt": timestamp(r.CreatedAt), "updatedAt": timestamp(r.UpdatedAt)}
}
func projectDTO(r sqlcgen.ListProjectsRow) gin.H {
	return gin.H{"id": r.ID, "teamId": r.TeamID, "name": r.Name, "description": nullable(r.Description), "teamName": r.TeamName, "role": r.Role, "permissions": stringList(r.Permissions), "updatedAt": timestamp(r.UpdatedAt)}
}
func (s *Server) directoryRoutes() {
	api := s.router.Group("/api", s.requireUser, s.timeout)
	api.GET("/teams", func(c *gin.Context) {
		rows, err := s.queries.ListTeams(c.Request.Context(), sqlcgen.ListTeamsParams{UserID: currentUser(c).ID, Search: strings.TrimSpace(c.Query("search")), Scope: c.Query("scope")})
		if err != nil {
			s.failure(c, 500, "TEAMS_FAILED")
			return
		}
		c.JSON(200, mapRows(rows, teamDTO))
	})
	api.GET("/teams/:id", func(c *gin.Context) {
		id, err := projectID(c)
		if err != nil {
			s.failure(c, 404, "TEAM_NOT_FOUND")
			return
		}
		team, err := s.queries.GetTeam(c.Request.Context(), sqlcgen.GetTeamParams{UserID: currentUser(c).ID, TeamID: id})
		if errors.Is(err, sql.ErrNoRows) {
			s.failure(c, 404, "TEAM_NOT_FOUND")
			return
		}
		if err != nil {
			s.failure(c, 500, "TEAM_FAILED")
			return
		}
		projects, err := s.queries.ListTeamProjects(c.Request.Context(), id)
		if err != nil {
			s.failure(c, 500, "TEAM_FAILED")
			return
		}
		c.JSON(200, gin.H{"team": teamDTO(sqlcgen.ListTeamsRow(team)), "projects": mapRows(projects, func(r sqlcgen.ListTeamProjectsRow) gin.H {
			return gin.H{"id": r.ID, "name": r.Name, "description": nullable(r.Description), "updatedAt": timestamp(r.UpdatedAt)}
		})})
	})
	api.GET("/projects", func(c *gin.Context) {
		rows, err := s.queries.ListProjects(c.Request.Context(), sqlcgen.ListProjectsParams{UserID: currentUser(c).ID, Scope: c.Query("scope"), Search: strings.TrimSpace(c.Query("search"))})
		if err != nil {
			s.failure(c, 500, "PROJECTS_FAILED")
			return
		}
		c.JSON(200, mapRows(rows, projectDTO))
	})
	api.GET("/projects/:id", func(c *gin.Context) {
		id, err := projectID(c)
		if err != nil {
			s.failure(c, 404, "PROJECT_NOT_FOUND")
			return
		}
		row, err := s.queries.GetProject(c.Request.Context(), sqlcgen.GetProjectParams{UserID: currentUser(c).ID, ProjectID: id})
		if errors.Is(err, sql.ErrNoRows) {
			s.failure(c, 404, "PROJECT_NOT_FOUND")
			return
		}
		if err != nil {
			s.failure(c, 500, "PROJECT_FAILED")
			return
		}
		c.JSON(200, projectDTO(sqlcgen.ListProjectsRow(row)))
	})
	api.GET("/profile", func(c *gin.Context) {
		rows, err := s.queries.ListProfileTeams(c.Request.Context(), currentUser(c).ID)
		if err != nil {
			s.failure(c, 500, "PROFILE_FAILED")
			return
		}
		c.JSON(200, gin.H{"profile": []User{currentUser(c)}, "teams": mapRows(rows, func(r sqlcgen.ListProfileTeamsRow) gin.H {
			return gin.H{"id": r.ID, "name": r.Name, "role": r.Role, "joinedAt": timestamp(r.JoinedAt)}
		})})
	})
	api.POST("/teams", s.createTeam)
	api.POST("/projects", s.createProject)
	admin := api.Group("/admin", s.requireAdmin)
	admin.GET("/overview", func(c *gin.Context) {
		r, err := s.queries.AdminOverview(c.Request.Context())
		if err != nil {
			s.failure(c, 500, "ADMIN_FAILED")
			return
		}
		c.JSON(200, gin.H{"users": r.Users, "teams": r.Teams, "projects": r.Projects})
	})
	admin.GET("/users", func(c *gin.Context) {
		rows, err := s.queries.ListAdminUsers(c.Request.Context())
		if err != nil {
			s.failure(c, 500, "ADMIN_FAILED")
			return
		}
		c.JSON(200, mapRows(rows, func(r sqlcgen.ListAdminUsersRow) gin.H {
			return gin.H{"id": r.ID, "name": r.Name, "email": nullable(r.Email), "phone": nullable(r.Phone), "role": r.Role, "createdAt": timestamp(r.CreatedAt)}
		}))
	})
	admin.GET("/teams", func(c *gin.Context) {
		rows, err := s.queries.ListAdminTeams(c.Request.Context())
		if err != nil {
			s.failure(c, 500, "ADMIN_FAILED")
			return
		}
		c.JSON(200, mapRows(rows, func(r sqlcgen.ListAdminTeamsRow) gin.H {
			return gin.H{"id": r.ID, "name": r.Name, "memberCount": r.MemberCount, "ownerUserId": nullInt(r.OwnerUserID), "ownerName": nullable(r.OwnerName)}
		}))
	})
	admin.GET("/projects", func(c *gin.Context) {
		rows, err := s.queries.ListAdminProjects(c.Request.Context())
		if err != nil {
			s.failure(c, 500, "ADMIN_FAILED")
			return
		}
		c.JSON(200, mapRows(rows, func(r sqlcgen.ListAdminProjectsRow) gin.H {
			return gin.H{"id": r.ID, "name": r.Name, "description": nullable(r.Description), "teamName": r.TeamName, "createdByName": nullable(r.CreatedByName), "createdAt": timestamp(r.CreatedAt)}
		}))
	})
}
func (s *Server) createTeam(c *gin.Context) {
	var in struct {
		Name string `json:"name"`
	}
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Name) == "" || utf16Length(strings.TrimSpace(in.Name)) > 100 {
		s.failure(c, 400, "TEAM_INPUT_INVALID")
		return
	}
	var id int32
	err := database.InTx(c.Request.Context(), s.db, func(_ *sql.Tx, q *sqlcgen.Queries) error {
		var err error
		id, err = q.CreateTeam(c.Request.Context(), strings.TrimSpace(in.Name))
		if err != nil {
			return err
		}
		return q.CreateTeamOwner(c.Request.Context(), sqlcgen.CreateTeamOwnerParams{TeamID: id, UserID: currentUser(c).ID})
	})
	if err != nil {
		s.failure(c, 500, "TEAM_CREATE_FAILED")
		return
	}
	c.JSON(201, gin.H{"id": id})
}
func (s *Server) createProject(c *gin.Context) {
	var in struct {
		TeamID any    `json:"teamId"`
		Name   string `json:"name"`
	}
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Name) == "" || utf16Length(strings.TrimSpace(in.Name)) > 100 {
		s.failure(c, 400, "PROJECT_INPUT_INVALID")
		return
	}
	var number float64
	switch value := in.TeamID.(type) {
	case float64:
		number = value
	case string:
		number, _ = strconv.ParseFloat(strings.TrimSpace(value), 64)
	}
	if math.IsNaN(number) || number <= 0 || number > math.MaxInt32 || math.Trunc(number) != number {
		s.failure(c, 400, "PROJECT_INPUT_INVALID")
		return
	}
	teamID := int32(number)
	var id int32
	err := database.InTx(c.Request.Context(), s.db, func(_ *sql.Tx, q *sqlcgen.Queries) error {
		if _, err := q.GetTeamManager(c.Request.Context(), sqlcgen.GetTeamManagerParams{TeamID: teamID, UserID: currentUser(c).ID}); err != nil {
			return err
		}
		var err error
		id, err = q.CreateProject(c.Request.Context(), sqlcgen.CreateProjectParams{TeamID: teamID, Name: strings.TrimSpace(in.Name), CreatedByUserID: sql.NullInt32{Int32: currentUser(c).ID, Valid: true}})
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		s.failure(c, 403, "FORBIDDEN")
		return
	}
	if err != nil {
		s.failure(c, 500, "PROJECT_CREATE_FAILED")
		return
	}
	c.JSON(201, gin.H{"id": id})
}
