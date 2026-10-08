package httpapi

import (
	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/media"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"io"
)

func (s *Server) readVideoAnnotations(c *gin.Context) {
	pid, err := projectID(c)
	id, parseErr := uuid.Parse(c.Param("runId"))
	if err != nil || parseErr != nil {
		s.failure(c, 404, "VIDEO_ANNOTATIONS_NOT_FOUND")
		return
	}
	if _, err = s.projectAccess(c.Request.Context(), s.queries, currentUser(c).ID, pid, "project:view"); err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	var key, checksum string
	err = s.db.QueryRowContext(c.Request.Context(), `select raw_result_object_key,raw_result_checksum_sha256 from algorithm_runs where id=$1 and project_id=$2 and status='succeeded' and canonical_result_json->>'kind'='video'`, id, pid).Scan(&key, &checksum)
	if err != nil {
		s.failure(c, 404, "VIDEO_ANNOTATIONS_NOT_FOUND")
		return
	}
	storage := s.mediaObjectStorage
	if storage == nil {
		storage, err = media.NewLocalObjectStorage(s.mediaStorageRoot)
	}
	if err != nil {
		s.failure(c, 503, "VIDEO_ANNOTATIONS_UNAVAILABLE")
		return
	}
	object, err := storage.GetObject(c.Request.Context(), key)
	if err != nil || len(object.Body) > 64*1024*1024 {
		s.failure(c, 503, "VIDEO_ANNOTATIONS_UNAVAILABLE")
		return
	}
	sum := sha256.Sum256(object.Body)
	if hex.EncodeToString(sum[:]) != checksum {
		s.failure(c, 409, "VIDEO_ANNOTATIONS_CHECKSUM_CHANGED")
		return
	}
	if c.Query("format") == "jsonl" {
		c.Header("Content-Disposition", `attachment; filename="video-annotations-`+id.String()+`.jsonl"`)
		c.Header("Cache-Control", "private, no-store")
		c.Data(200, "application/x-ndjson", object.Body)
		return
	}
	frames := []algorithm.VideoFrame{}
	decoder := json.NewDecoder(bytes.NewReader(object.Body))
	for {
		var frame algorithm.VideoFrame
		err = decoder.Decode(&frame)
		if err == io.EOF {
			break
		}
		if err != nil || len(frames) >= algorithm.MaxVideoFrames {
			s.failure(c, 503, "VIDEO_ANNOTATIONS_UNAVAILABLE")
			return
		}
		frames = append(frames, frame)
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, gin.H{"frames": frames})
}
