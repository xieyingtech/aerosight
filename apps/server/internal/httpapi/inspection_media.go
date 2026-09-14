package httpapi

import (
	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/inspection"
	"aerosight/server/internal/media"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) AttachInspectionMedia(reader algorithm.RemoteAlgorithmAssetReader) {
	s.inspectionMedia = reader
}

// Preview always uses the sealed observation as its version authority. Never
// redirect to the current asset URL or expose a provider download credential.
func (s *Server) readInspectionImage(c *gin.Context) {
	pid, err := projectID(c)
	observationID, idErr := uuid.Parse(c.Param("observationId"))
	aid, assetErr := strconv.ParseInt(c.Param("assetId"), 10, 32)
	if err != nil || idErr != nil || assetErr != nil || aid <= 0 {
		s.failure(c, 404, "INSPECTION_IMAGE_NOT_FOUND")
		return
	}
	asset, access, err := s.readMediaAsset(c.Request.Context(), currentUser(c).ID, pid, int32(aid), "play")
	if err != nil {
		s.failure(c, 403, "INSPECTION_IMAGE_ACCESS_DENIED")
		return
	}
	var raw []byte
	err = s.db.QueryRowContext(c.Request.Context(), `select manifest_json from inspection_observations where project_id=$1 and team_id=$2 and id=$3 and sealed_at is not null`, pid, access.TeamID, observationID).Scan(&raw)
	if err != nil {
		s.failure(c, 404, "INSPECTION_IMAGE_NOT_FOUND")
		return
	}
	var observation inspection.Observation
	if json.Unmarshal(raw, &observation) != nil {
		s.failure(c, 409, "INSPECTION_IMAGE_SNAPSHOT_INVALID")
		return
	}
	var ref *inspection.AssetRef
	for n := range observation.Assets {
		candidate := &observation.Assets[n]
		if candidate.AssetID == aid && candidate.ProjectID == int(pid) && candidate.TeamID == int(access.TeamID) {
			ref = candidate
			break
		}
	}
	if ref == nil {
		s.failure(c, 404, "INSPECTION_IMAGE_NOT_FOUND")
		return
	}
	if len(ref.ChecksumSHA256) != 64 {
		s.failure(c, 409, "INSPECTION_IMAGE_VERSION_UNVERIFIABLE")
		return
	}
	var current bool
	err = s.db.QueryRowContext(c.Request.Context(), `select exists(select 1 from assets where id=$1 and project_id=$2 and team_id=$3 and version=$4 and coalesce(object_version,'')=$5 and task_run_id is not distinct from $6::integer and status='available' and deleted_at is null and kind='image')`, aid, pid, access.TeamID, ref.Version, ref.ObjectVersion, ref.SourceRunID).Scan(&current)
	if err != nil || !current {
		s.failure(c, 409, "INSPECTION_IMAGE_VERSION_CHANGED")
		return
	}
	var body []byte
	handled := false
	if s.inspectionMedia != nil {
		var remote algorithm.AlgorithmAsset
		remote, handled, err = s.inspectionMedia(c.Request.Context(), int(pid), int(aid), ref.Version)
		body = remote.Body
	}
	if err == nil && !handled {
		// A missing remote reader must never turn a remote reference into a local read.
		var remote bool
		err = s.db.QueryRowContext(c.Request.Context(), `select exists(select 1 from connector_asset_access_refs where project_id=$1 and id=$2)`, pid, aid).Scan(&remote)
		if err != nil || remote {
			s.failure(c, 503, "INSPECTION_IMAGE_READER_UNAVAILABLE")
			return
		}
		file, openErr := media.OpenProjectObject(s.mediaStorageRoot, pid, asset.StorageKey)
		if openErr != nil {
			s.failure(c, 404, "INSPECTION_IMAGE_UNAVAILABLE")
			return
		}
		defer file.Close()
		body, err = io.ReadAll(io.LimitReader(file, (64<<20)+1))
	}
	if err != nil {
		s.failure(c, 404, "INSPECTION_IMAGE_UNAVAILABLE")
		return
	}
	if len(body) > 64<<20 {
		s.failure(c, 413, "INSPECTION_IMAGE_TOO_LARGE")
		return
	}
	checksum := sha256.Sum256(body)
	if hex.EncodeToString(checksum[:]) != ref.ChecksumSHA256 {
		s.failure(c, 409, "INSPECTION_IMAGE_VERSION_CHANGED")
		return
	}
	contentType := http.DetectContentType(body)
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		s.failure(c, 415, "INSPECTION_IMAGE_FORMAT_UNSUPPORTED")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, contentType, body)
}
