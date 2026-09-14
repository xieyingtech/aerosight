package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"aerosight/server/internal/database"
	"aerosight/server/internal/media"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// importImageAsset freezes uploaded image bytes without creating any flight or Run.
func (s *Server) importImageAsset(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	uid := currentUser(c).ID
	access, err := s.projectAccess(c.Request.Context(), s.queries, uid, pid, "project:view")
	if err != nil || (access.Role != "owner" && access.Role != "admin") {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	const limit = 40 << 20
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit+(1<<20))
	if err = c.Request.ParseMultipartForm(1 << 20); err != nil {
		s.failure(c, 400, "ASSET_IMPORT_INVALID_MULTIPART")
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	f, header, err := c.Request.FormFile("file")
	if err != nil {
		s.failure(c, 400, "ASSET_IMPORT_FILE_REQUIRED")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || len(data) > limit {
		s.failure(c, 413, "ASSET_IMPORT_TOO_LARGE")
		return
	}
	im, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || im.Width < 1 || im.Height < 1 || int64(im.Width)*int64(im.Height) > 90000000 {
		s.failure(c, 422, "ASSET_IMPORT_IMAGE_INVALID")
		return
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		s.failure(c, 422, "ASSET_IMPORT_IMAGE_INVALID")
		return
	}
	var captured any
	if raw := c.PostForm("capturedAt"); raw != "" {
		t, e := time.Parse(time.RFC3339, raw)
		if e != nil {
			s.failure(c, 400, "ASSET_IMPORT_TIME_INVALID")
			return
		}
		captured = t.UTC()
	}
	source := c.PostForm("sourceDescription")
	if len(source) > 2000 {
		s.failure(c, 400, "ASSET_IMPORT_SOURCE_INVALID")
		return
	}
	name := filepath.Base(header.Filename)
	metadata, _ := json.Marshal(gin.H{"name": name, "width": im.Width, "height": im.Height, "source": "manual-upload", "sourceDescription": source, "positionQuality": "unavailable"})
	store, err := media.NewLocalObjectStorage(s.mediaStorageRoot)
	if err != nil {
		s.failure(c, 503, "ASSET_STORAGE_UNAVAILABLE")
		return
	}
	key := fmt.Sprintf("projects/%d/imports/%s.%s", pid, uuid.NewString(), format)
	object, err := store.PutObject(c.Request.Context(), key, bytes.NewReader(data), "image/"+format)
	if err != nil {
		s.failure(c, 503, "ASSET_STORAGE_UNAVAILABLE")
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "asset.import", ResourceType: "asset", Input: gin.H{"name": name, "checksumSha256": object.ChecksumSHA256, "sourceDescription": source}}
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "project:view", true), func(w *database.WriteTx) (gin.H, error) {
		var id int
		e := w.Tx.QueryRowContext(c.Request.Context(), `insert into assets(project_id,team_id,kind,mime_type,storage_key,logical_key,size_bytes,checksum_sha256,checksum,captured_at,metadata_json,status,available_at) values($1,$2,'image',$3,$4,$4,$5,$6,$6,$7,$8,'available',now()) returning id`, pid, access.TeamID, "image/"+format, key, len(data), object.ChecksumSHA256, captured, metadata).Scan(&id)
		return gin.H{"assetId": id, "checksumSha256": object.ChecksumSHA256, "width": im.Width, "height": im.Height}, e
	})
	if err != nil {
		s.failure(c, 500, "ASSET_IMPORT_FAILED")
		return
	}
	c.JSON(201, result)
}
