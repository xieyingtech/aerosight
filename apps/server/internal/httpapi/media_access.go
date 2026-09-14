package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/httptransport"
	"aerosight/server/internal/media"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) AttachMediaStorage(root string) { s.mediaStorageRoot = root }
func (s *Server) mediaAccessRoutes() {
	s.router.POST("/api/projects/:id/assets/import", s.requireUser, s.importImageAsset)
	group := s.router.Group("/api/projects/:id/assets/:assetId", s.requireUser)
	group.GET("/access", s.timeout, s.issueMediaAccess)
	group.GET("/content", s.readMediaContent)
	group.HEAD("/content", s.readMediaContent)
}
func mediaAccessScope(c *gin.Context) (int32, int32, string, error) {
	pid, err := projectID(c)
	if err != nil {
		return 0, 0, "", err
	}
	id, err := strconv.ParseInt(c.Param("assetId"), 10, 32)
	action := c.Query("action")
	if err != nil || id <= 0 || !media.ValidAccessAction(action) {
		return 0, 0, "", errors.New("INVALID_MEDIA_ACCESS_INPUT")
	}
	return pid, int32(id), action, nil
}
func (s *Server) mediaAsset(ctx context.Context, q *sqlcgen.Queries, uid, pid, aid int32, action string) (sqlcgen.ReadMediaAccessAssetRow, sqlcgen.GetProjectAccessRow, error) {
	access, err := s.projectAccess(ctx, q, uid, pid, "project:view")
	if err != nil {
		return sqlcgen.ReadMediaAccessAssetRow{}, access, err
	}
	asset, err := q.ReadMediaAccessAsset(ctx, sqlcgen.ReadMediaAccessAssetParams{ProjectID: pid, ID: aid})
	if err != nil {
		return asset, access, err
	}
	if !media.AccessAllowed(action, access.Role, effectivePermissions(access.Role, access.Permissions), asset.Sensitive) {
		return asset, access, errors.New("SENSITIVE_MEDIA_DOWNLOAD_DENIED")
	}
	return asset, access, nil
}
func (s *Server) readMediaAsset(ctx context.Context, uid, pid, aid int32, action string) (sqlcgen.ReadMediaAccessAssetRow, sqlcgen.GetProjectAccessRow, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return sqlcgen.ReadMediaAccessAssetRow{}, sqlcgen.GetProjectAccessRow{}, err
	}
	defer tx.Rollback()
	asset, access, err := s.mediaAsset(ctx, s.queries.WithTx(tx), uid, pid, aid, action)
	if err != nil {
		return asset, access, err
	}
	err = tx.Commit()
	return asset, access, err
}
func (s *Server) issueMediaAccess(c *gin.Context) {
	fail := func() { s.failure(c, 403, "Unable to access media") }
	pid, aid, action, err := mediaAccessScope(c)
	if err != nil {
		fail()
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	asset, access, err := s.readMediaAsset(ctx, uid, pid, aid, action)
	if err != nil {
		fail()
		return
	}
	issue := func() (media.Access, error) {
		return media.IssueAccess(s.credentialSecret, pid, aid, action, time.Now(), 120)
	}
	var result media.Access
	if action == "download" && asset.Sensitive {
		audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "media.sensitive_download", ResourceType: "asset", ResourceID: strconv.FormatInt(int64(aid), 10), Input: gin.H{"action": action, "assetId": aid}, PolicyResult: map[string]any{"permission": "project:view", "sensitive": true}}
		result, err = database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "project:view", false), func(w *database.WriteTx) (media.Access, error) {
			if _, _, e := s.mediaAsset(ctx, w.Queries, uid, pid, aid, action); e != nil {
				return media.Access{}, e
			}
			return issue()
		})
	} else {
		result, err = issue()
	}
	if err != nil {
		fail()
		return
	}
	c.JSON(200, result)
}
func (s *Server) readMediaContent(c *gin.Context) {
	fail := func() { s.failure(c, 403, "Media unavailable") }
	pid, aid, action, err := mediaAccessScope(c)
	if err != nil {
		fail()
		return
	}
	lookup, cancel := context.WithTimeout(c.Request.Context(), s.cfg.RequestTimeout)
	asset, _, err := s.readMediaAsset(lookup, currentUser(c).ID, pid, aid, action)
	lookupError := lookup.Err()
	cancel()
	if lookupError == context.DeadlineExceeded {
		s.failure(c, 504, "REQUEST_TIMEOUT")
		return
	}
	if err != nil {
		fail()
		return
	}
	if !media.VerifyAccess(s.credentialSecret, pid, aid, action, c.Query("expires"), c.Query("signature"), time.Now()) {
		fail()
		return
	}
	file, err := media.OpenProjectObject(s.mediaStorageRoot, pid, asset.StorageKey)
	if err != nil {
		fail()
		return
	}
	defer file.Close()
	contentType := "application/octet-stream"
	if asset.MimeType.Valid {
		contentType = asset.MimeType.String
	}
	disposition := "inline"
	if action == "download" {
		var name *string
		if asset.FileName.Valid {
			name = &asset.FileName.String
		}
		disposition = fmt.Sprintf(`attachment; filename="%s"`, media.SafeDownloadName(name, fmt.Sprintf("asset-%d", aid)))
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", disposition)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	// No Last-Modified validators: each access is reauthorized and must not turn
	// into a shared/public cache hit. ServeContent implements byte ranges/HEAD.
	httptransport.ServeContent(c.Writer, c.Request, file)
}
