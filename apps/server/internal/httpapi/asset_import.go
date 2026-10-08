package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"aerosight/server/internal/database"
	"aerosight/server/internal/media"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// importImageAsset imports images or MP4 video without creating a flight/task.
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
	const limit = 512 << 20
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
	temporary, err := os.CreateTemp("", "aerosight-import-*")
	if err != nil {
		s.failure(c, 503, "ASSET_STORAGE_UNAVAILABLE")
		return
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	size, err := io.Copy(temporary, io.LimitReader(f, limit+1))
	if err != nil || size > limit {
		s.failure(c, 413, "ASSET_IMPORT_TOO_LARGE")
		return
	}
	temporary.Seek(0, io.SeekStart)
	var head [512]byte
	n, _ := temporary.Read(head[:])
	temporary.Seek(0, io.SeekStart)
	kind, contentType, format := "image", "", ""
	width, height := 0, 0
	duration := 0.0
	if n >= 12 && string(head[4:8]) == "ftyp" {
		info, probeErr := probeImportedVideo(c.Request.Context(), temporary.Name())
		if probeErr != nil {
			s.failure(c, 422, "ASSET_IMPORT_VIDEO_INVALID")
			return
		}
		kind, contentType, format = "video", "video/mp4", "mp4"
		width, height, duration = info.Width, info.Height, info.Duration
	} else {
		if size > 40<<20 {
			s.failure(c, 413, "ASSET_IMPORT_TOO_LARGE")
			return
		}
		data, readErr := io.ReadAll(temporary)
		im, imageFormat, imageErr := image.DecodeConfig(bytes.NewReader(data))
		if readErr != nil || imageErr != nil || (imageFormat != "jpeg" && imageFormat != "png") || im.Width < 1 || im.Height < 1 || int64(im.Width)*int64(im.Height) > 90000000 {
			s.failure(c, 422, "ASSET_IMPORT_IMAGE_INVALID")
			return
		}
		if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
			s.failure(c, 422, "ASSET_IMPORT_IMAGE_INVALID")
			return
		}
		format = imageFormat
		contentType = "image/" + format
		width, height = im.Width, im.Height
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
	meta := gin.H{"name": name, "width": width, "height": height, "durationSeconds": duration, "source": "manual-upload", "sourceDescription": source, "positionQuality": "unavailable"}
	if c.PostForm("streamId") != "" && c.PostForm("videoAssetId") != "" {
		s.failure(c, 422, "ASSET_IMPORT_SOURCE_AMBIGUOUS")
		return
	}
	var deviceID any
	if raw := c.PostForm("streamId"); raw != "" {
		streamID, parseErr := strconv.ParseInt(raw, 10, 32)
		var did int32
		if parseErr != nil || kind != "image" || s.db.QueryRowContext(c.Request.Context(), `select device_id from live_streams where project_id=$1 and id=$2 and status in ('starting','live','degraded')`, pid, streamID).Scan(&did) != nil {
			s.failure(c, 422, "ASSET_IMPORT_STREAM_INVALID")
			return
		}
		deviceID = did
		meta["source"] = "live-frame"
		meta["streamId"] = streamID
		meta["timeQuality"] = "browser-receive-time"
	}
	if raw := c.PostForm("videoAssetId"); raw != "" {
		parent, parseErr := strconv.ParseInt(raw, 10, 32)
		offset, offsetErr := strconv.ParseFloat(c.PostForm("mediaTimeSeconds"), 64)
		var valid bool
		if parseErr != nil || offsetErr != nil || math.IsNaN(offset) || math.IsInf(offset, 0) || offset < 0 || kind != "image" || s.db.QueryRowContext(c.Request.Context(), `select exists(select 1 from assets where id=$1 and project_id=$2 and kind='video' and status='available' and deleted_at is null and coalesce((metadata_json->>'durationSeconds')::double precision,0)>$3)`, parent, pid, offset).Scan(&valid) != nil || !valid {
			s.failure(c, 422, "ASSET_IMPORT_VIDEO_SOURCE_INVALID")
			return
		}
		var parentTime sql.NullTime
		if s.db.QueryRowContext(c.Request.Context(), `select captured_at from assets where id=$1 and project_id=$2`, parent, pid).Scan(&parentTime) == nil && parentTime.Valid {
			captured = parentTime.Time.Add(time.Duration(offset * float64(time.Second)))
		}
		meta["source"] = "video-frame"
		meta["videoAssetId"] = parent
		meta["mediaTimeSeconds"] = offset
		meta["timeQuality"] = "video-offset"
	}
	metadata, _ := json.Marshal(meta)
	store := s.mediaObjectStorage
	if store == nil {
		store, err = media.NewLocalObjectStorage(s.mediaStorageRoot)
	}
	if err != nil {
		s.failure(c, 503, "ASSET_STORAGE_UNAVAILABLE")
		return
	}
	key := fmt.Sprintf("projects/%d/imports/%s.%s", pid, uuid.NewString(), format)
	temporary.Seek(0, io.SeekStart)
	object, err := store.PutObject(c.Request.Context(), key, temporary, contentType)
	if err != nil {
		s.failure(c, 503, "ASSET_STORAGE_UNAVAILABLE")
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "asset.import", ResourceType: "asset", Input: gin.H{"name": name, "checksumSha256": object.ChecksumSHA256, "sourceDescription": source}}
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "project:view", true), func(w *database.WriteTx) (gin.H, error) {
		var id int
		e := w.Tx.QueryRowContext(c.Request.Context(), `insert into assets(project_id,team_id,kind,mime_type,storage_key,logical_key,size_bytes,checksum_sha256,checksum,captured_at,metadata_json,status,available_at,device_id) values($1,$2,$3,$4,$5,$5,$6,$7,$7,$8,$9,'available',now(),$10) returning id`, pid, access.TeamID, kind, contentType, key, size, object.ChecksumSHA256, captured, metadata, deviceID).Scan(&id)
		return gin.H{"assetId": id, "checksumSha256": object.ChecksumSHA256, "width": width, "height": height, "kind": kind, "durationSeconds": duration}, e
	})
	if err != nil {
		s.failure(c, 500, "ASSET_IMPORT_FAILED")
		return
	}
	c.JSON(201, result)
}

type importedVideo struct {
	Width, Height int
	Duration      float64
}

func probeImportedVideo(ctx context.Context, file string) (importedVideo, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height,codec_name:format=duration,format_name", "-of", "json", file).Output()
	if err != nil {
		return importedVideo{}, err
	}
	var info struct {
		Streams []struct{ Width, Height int }
		Format  struct {
			Duration   string
			FormatName string `json:"format_name"`
		}
	}
	if json.Unmarshal(output, &info) != nil || len(info.Streams) != 1 {
		return importedVideo{}, fmt.Errorf("invalid video")
	}
	duration, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || info.Streams[0].Width <= 0 || info.Streams[0].Height <= 0 {
		return importedVideo{}, fmt.Errorf("invalid video")
	}
	return importedVideo{info.Streams[0].Width, info.Streams[0].Height, duration}, nil
}
