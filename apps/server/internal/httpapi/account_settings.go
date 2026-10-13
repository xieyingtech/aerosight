package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func settingsInput(c *gin.Context, value any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("INPUT_INVALID")
	}
	return nil
}

func validSettingsName(name string) bool {
	return utf8.ValidString(name) && utf8.RuneCountInString(name) >= 1 && utf8.RuneCountInString(name) <= 100
}

func (s *Server) accountAuthorization(uid int32, version int) database.Reauthorize {
	return func(ctx context.Context, w *database.WriteTx) error {
		account, err := w.Queries.LockAccount(ctx, uid)
		if err != nil {
			return err
		}
		if int(account.CredentialVersion) != version {
			return errors.New("UNAUTHENTICATED")
		}
		return nil
	}
}

func (s *Server) renameProfile(c *gin.Context) {
	var in struct {
		Name string `json:"name"`
	}
	if settingsInput(c, &in) != nil {
		s.failure(c, 400, "PROFILE_INPUT_INVALID")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validSettingsName(in.Name) {
		s.failure(c, 400, "PROFILE_INPUT_INVALID")
		return
	}
	user := currentUser(c)
	ctx := c.Request.Context()
	result, err := database.AuditedPlatformWrite(ctx, s.db, database.AuditContext{ActorUserID: user.ID, RequestID: c.GetHeader("X-Request-ID"), Action: "account.name.update", ResourceType: "user", ResourceID: strconv.Itoa(int(user.ID)), Input: in}, s.accountAuthorization(user.ID, s.sessions.GetInt(ctx, "credentialVersion")), func(w *database.WriteTx) (User, error) {
		err := w.Queries.UpdateProfileName(ctx, sqlcgen.UpdateProfileNameParams{ID: user.ID, Name: in.Name})
		user.Name = in.Name
		return user, err
	})
	if err != nil {
		s.settingsFailure(c, err, "PROFILE_UPDATE_FAILED")
		return
	}
	c.JSON(200, gin.H{"user": result})
}

func (s *Server) changePassword(c *gin.Context) {
	uid := currentUser(c).ID
	if s.loginRate.RespondOnLimit(c.Writer, c.Request, "password:"+strconv.Itoa(int(uid))) {
		c.Abort()
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if settingsInput(c, &in) != nil || len(in.CurrentPassword) == 0 || len(in.CurrentPassword) > 1024 || !utf8.ValidString(in.NewPassword) || utf8.RuneCountInString(in.NewPassword) < 8 || len(in.NewPassword) > 72 || in.NewPassword == in.CurrentPassword {
		s.failure(c, 400, "PASSWORD_INPUT_INVALID")
		return
	}
	ctx := c.Request.Context()
	account, err := s.queries.ReadAccountPassword(ctx, uid)
	if err != nil {
		s.failure(c, 500, "PASSWORD_UPDATE_FAILED")
		return
	}
	if !account.Password.Valid || bcrypt.CompareHashAndPassword([]byte(account.Password.String), []byte(in.CurrentPassword)) != nil {
		s.failure(c, 400, "CURRENT_PASSWORD_INVALID")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), 12)
	if err != nil {
		s.failure(c, 500, "PASSWORD_UPDATE_FAILED")
		return
	}
	_, err = database.AuditedPlatformWrite(ctx, s.db, database.AuditContext{ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "account.password.update", ResourceType: "user", ResourceID: strconv.Itoa(int(uid)), Input: gin.H{"changed": true}}, s.accountAuthorization(uid, s.sessions.GetInt(ctx, "credentialVersion")), func(w *database.WriteTx) (bool, error) {
		count, err := w.Queries.UpdateAccountPassword(ctx, sqlcgen.UpdateAccountPasswordParams{UserID: uid, NewPassword: sql.NullString{String: string(hash), Valid: true}, OldPassword: account.Password, CredentialVersion: account.CredentialVersion})
		if err == nil && count != 1 {
			err = errors.New("PASSWORD_CHANGE_CONFLICT")
		}
		return true, err
	})
	if err != nil {
		s.settingsFailure(c, err, "PASSWORD_UPDATE_FAILED")
		return
	}
	// Version invalidation is committed before this response, including this session.
	c.Status(204)
}

func (s *Server) renameProject(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "project:view")
	if err != nil || (access.Role != "owner" && access.Role != "admin") {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if settingsInput(c, &in) != nil {
		s.failure(c, 400, "PROJECT_INPUT_INVALID")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validSettingsName(in.Name) {
		s.failure(c, 400, "PROJECT_INPUT_INVALID")
		return
	}
	result, err := database.AuditedWrite(ctx, s.db, database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "project.name.update", ResourceType: "project", ResourceID: strconv.Itoa(int(pid)), Input: in}, s.authorizeWrite(uid, pid, access.TeamID, "project:view", true), func(w *database.WriteTx) (sqlcgen.UpdateProjectNameRow, error) {
		return w.Queries.UpdateProjectName(ctx, sqlcgen.UpdateProjectNameParams{ID: pid, TeamID: access.TeamID, Name: in.Name})
	})
	if err != nil {
		s.settingsFailure(c, err, "PROJECT_UPDATE_FAILED")
		return
	}
	c.JSON(200, gin.H{"id": result.ID, "name": result.Name})
}

func (s *Server) settingsFailure(c *gin.Context, err error, fallback string) {
	switch err.Error() {
	case "UNAUTHENTICATED":
		s.failure(c, 401, err.Error())
	case "PROJECT_ACCESS_DENIED":
		s.failure(c, 403, err.Error())
	case "PASSWORD_CHANGE_CONFLICT":
		s.failure(c, 409, err.Error())
	default:
		s.failure(c, 500, fallback)
	}
}
