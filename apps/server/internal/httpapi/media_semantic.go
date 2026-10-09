package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"aerosight/server/internal/database"
	"aerosight/server/internal/semantic"
	"github.com/gin-gonic/gin"
)

func (s *Server) AttachSemanticIndex(index *semantic.Service) { s.semanticIndex = index }
func mediaSearchSchema() map[string]any {
	return agentObject(map[string]any{"query": gin.H{"type": "string", "minLength": 1, "maxLength": 2000}, "limit": gin.H{"type": "integer", "minimum": 1, "maximum": 20}, "start": agentText(), "end": agentText()}, "query")
}
func (s *Server) semanticRoutes() {
	s.router.POST("/api/projects/:id/media-search", s.requireUser, s.chatTimeout, s.searchMedia)
	s.router.GET("/api/projects/:id/assets/:assetId/semantic-index", s.requireUser, s.timeout, s.mediaIndexStatus)
	s.router.POST("/api/projects/:id/assets/:assetId/semantic-index/retry", s.requireUser, s.timeout, s.retryMediaIndex)
}
func parseMediaSearch(raw json.RawMessage) (semantic.SearchInput, error) {
	var out semantic.SearchInput
	var generic any
	if json.Unmarshal(raw, &generic) != nil {
		return out, errors.New("MEDIA_SEARCH_INPUT_INVALID")
	}
	if err := chatScopeArgument(generic); err != nil {
		return out, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil {
		return out, errors.New("MEDIA_SEARCH_INPUT_INVALID")
	}
	if fields, ok := generic.(map[string]any); ok {
		if _, present := fields["limit"]; present && out.Limit == 0 {
			return out, errors.New("MEDIA_SEARCH_INPUT_INVALID")
		}
		for _, name := range []string{"start", "end"} {
			if value, present := fields[name]; present {
				if _, ok := value.(string); !ok {
					return out, errors.New("MEDIA_SEARCH_INPUT_INVALID")
				}
			}
		}
	}
	var rest any
	if decoder.Decode(&rest) != io.EOF {
		return out, errors.New("MEDIA_SEARCH_INPUT_INVALID")
	}
	return out, out.Validate()
}
func (s *Server) executeMediaSearch(ctx context.Context, uid, pid int32, arguments json.RawMessage) (gin.H, error) {
	input, err := parseMediaSearch(arguments)
	if err != nil {
		return nil, err
	}
	if _, err = s.projectAccess(ctx, s.queries, uid, pid, "project:view"); err != nil {
		return nil, errors.New("PROJECT_ACCESS_DENIED")
	}
	if s.semanticIndex == nil {
		return nil, semantic.ErrUnavailable
	}
	authorize := func(ctx context.Context, aid int32) error {
		_, _, err := s.readMediaAsset(ctx, uid, pid, aid, "preview")
		return err
	}
	rows, truncated, err := s.semanticIndex.Search(ctx, pid, input, authorize)
	if err != nil {
		return nil, err
	}
	if _, err = s.projectAccess(ctx, s.queries, uid, pid, "project:view"); err != nil {
		return nil, errors.New("PROJECT_ACCESS_DENIED")
	}
	items := []gin.H{}
	for _, row := range rows {
		items = append(items, gin.H{"segmentId": row.ID, "assetId": row.AssetID, "version": row.Version, "startMs": row.StartMS, "endMs": row.EndMS, "description": row.Description, "timeQuality": row.TimeQuality, "capturedStart": row.CapturedStart, "capturedEnd": row.CapturedEnd, "score": row.Score, "reference": gin.H{"type": "asset", "id": strconv.FormatInt(int64(row.AssetID), 10), "href": fmt.Sprintf("/projects/assets/?projectId=%d&assetId=%d&startMs=%d&endMs=%d", pid, row.AssetID, row.StartMS, row.EndMS)}})
	}
	return gin.H{"items": items, "truncated": truncated, "summary": fmt.Sprintf("找到 %d 个素材片段。描述由模型生成，请打开原素材复核；零匹配不证明目标不存在。", len(items)), "query": input.Query}, nil
}
func (s *Server) searchMedia(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 400, "INVALID_PROJECT")
		return
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		s.failure(c, 400, "MEDIA_SEARCH_INPUT_INVALID")
		return
	}
	result, err := s.executeMediaSearch(c.Request.Context(), currentUser(c).ID, pid, raw)
	if err != nil {
		switch err.Error() {
		case "MEDIA_SEARCH_INPUT_INVALID":
			s.failure(c, 400, err.Error())
		case "PROJECT_ACCESS_DENIED":
			s.failure(c, 403, err.Error())
		default:
			s.failure(c, 503, "MEDIA_INDEX_UNAVAILABLE")
		}
		return
	}
	c.JSON(200, result)
}
func (s *Server) semanticAsset(c *gin.Context, write bool) (int32, int32, bool) {
	pid, err := projectID(c)
	aid, e := strconv.ParseInt(c.Param("assetId"), 10, 32)
	if err != nil || e != nil || aid <= 0 {
		s.failure(c, 400, "INVALID_ASSET")
		return 0, 0, false
	}
	uid := currentUser(c).ID
	if _, _, err = s.readMediaAsset(c.Request.Context(), uid, pid, int32(aid), "preview"); err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return 0, 0, false
	}
	if write {
		access, err := s.projectAccess(c.Request.Context(), s.queries, uid, pid, "project:view")
		if err != nil || (access.Role != "owner" && access.Role != "admin") {
			s.failure(c, 403, "PROJECT_ADMIN_REQUIRED")
			return 0, 0, false
		}
	}
	return pid, int32(aid), true
}
func (s *Server) mediaIndexStatus(c *gin.Context) {
	pid, aid, ok := s.semanticAsset(c, false)
	if !ok {
		return
	}
	if s.semanticIndex == nil {
		c.JSON(200, semantic.Status{State: "disabled"})
		return
	}
	result, err := s.semanticIndex.Status(c.Request.Context(), pid, aid)
	if err != nil {
		s.failure(c, 503, "MEDIA_INDEX_UNAVAILABLE")
		return
	}
	c.JSON(200, result)
}
func (s *Server) retryMediaIndex(c *gin.Context) {
	pid, aid, ok := s.semanticAsset(c, true)
	if !ok {
		return
	}
	if s.semanticIndex == nil {
		s.failure(c, 503, "MEDIA_INDEX_UNAVAILABLE")
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "project:view")
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "asset.semantic_index.retry", ResourceType: "asset", ResourceID: strconv.Itoa(int(aid)), Input: gin.H{"assetId": aid}}
	_, err = database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "project:view", true), func(w *database.WriteTx) (gin.H, error) {
		if _, _, e := s.mediaAsset(ctx, w.Queries, uid, pid, aid, "preview"); e != nil {
			return nil, e
		}
		return gin.H{"state": "queued"}, s.semanticIndex.RetryTx(ctx, w.Tx, pid, aid)
	})
	if err != nil {
		if err.Error() == "MEDIA_INDEX_BUSY_OR_UNSUPPORTED" {
			s.failure(c, 409, err.Error())
			return
		}
		s.failure(c, 503, "MEDIA_INDEX_UNAVAILABLE")
		return
	}
	c.JSON(202, gin.H{"state": "queued"})
}
