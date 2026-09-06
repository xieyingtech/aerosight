package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type chatToolInput struct {
	DeviceIDs     []int32
	FilterDevices bool
	Limit         int
}

func chatScopeArgument(value any) error {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if err := chatScopeArgument(item); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, item := range v {
			switch key {
			case "userId", "teamId", "projectId", "sessionId", "user_id", "team_id", "project_id", "session_id":
				return errors.New("AGENT_TOOL_SCOPE_ARGUMENT_FORBIDDEN:" + key)
			}
			if err := chatScopeArgument(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseChatToolInput(name string, raw json.RawMessage) (chatToolInput, error) {
	out := chatToolInput{Limit: 100}
	bad := errors.New("AGENT_TOOL_INPUT_INVALID")
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil || input == nil {
		return out, bad
	}
	if err := chatScopeArgument(input); err != nil {
		return out, err
	}
	allowed := ""
	switch name {
	case "query_devices":
		allowed = "deviceIds"
	case "query_tasks", "query_issues":
		allowed = "limit"
		out.Limit = 20
	case "query_assets", "query_tracks", "query_map_context":
	default:
		return out, errors.New("AGENT_TOOL_NOT_READ_ONLY")
	}
	for k := range input {
		if k != allowed {
			return out, bad
		}
	}
	if value, ok := input["limit"]; ok {
		n, yes := value.(float64)
		if !yes || n < 1 || n > 100 || math.Trunc(n) != n {
			return out, bad
		}
		out.Limit = int(n)
	}
	if value, ok := input["deviceIds"]; ok {
		ids, yes := value.([]any)
		if !yes || len(ids) > 100 {
			return out, bad
		}
		out.FilterDevices = true
		out.DeviceIDs = []int32{}
		for _, v := range ids {
			n, yes := v.(float64)
			if !yes || n <= 0 || n > math.MaxInt32 || math.Trunc(n) != n {
				return out, bad
			}
			out.DeviceIDs = append(out.DeviceIDs, int32(n))
		}
	}
	return out, nil
}

func chatEvidenceReference(pid int32, name, id string) gin.H {
	query := url.Values{"projectId": {strconv.Itoa(int(pid))}}
	path, kind := "/projects/detail/", "map-context"
	switch name {
	case "query_devices":
		path, kind = "/projects/devices/", "device"
		query.Set("selected", id)
	case "query_tasks":
		path, kind = "/projects/tasks/runs/detail/", "task-run"
		query.Set("runId", id)
	case "query_issues":
		path, kind = "/projects/issues/detail/", "issue"
		query.Set("issueId", id)
	case "query_assets":
		path, kind = "/projects/assets/", "asset"
		query.Set("selected", id)
	case "query_tracks":
		kind = "track"
		query.Set("selected", id)
	default:
		query.Set("selected", id)
	}
	return gin.H{"type": kind, "id": id, "href": path + "?" + query.Encode()}
}

func formatChatToolResult(pid int32, name string, rows []gin.H, limit int, now time.Time) (gin.H, error) {
	if limit > 100 {
		limit = 100
	}
	items := []gin.H{}
	truncated := len(rows) > limit
	var newest time.Time
	for index, row := range rows {
		if index >= limit {
			break
		}
		item := gin.H{}
		for k, v := range row {
			item[k] = v
		}
		id := row["id"]
		if id == nil {
			id = row["deviceId"]
		}
		if id == nil {
			id = "current"
		}
		idText := fmt.Sprint(id)
		if number, ok := id.(float64); ok {
			idText = strconv.FormatFloat(number, 'f', -1, 64)
		}
		item["reference"] = chatEvidenceReference(pid, name, idText)
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		encoder.SetEscapeHTML(false)
		candidate := append(items, item)
		if err := encoder.Encode(candidate); err != nil {
			return nil, err
		}
		if buffer.Len()-1 > 64*1024 {
			truncated = true
			break
		}
		items = candidate
		for _, key := range []string{"observedAt", "capturedAt", "updatedAt", "createdAt", "lastSeenAt"} {
			if value, ok := item[key].(string); ok {
				if t, err := time.Parse(time.RFC3339Nano, value); err == nil && t.After(newest) {
					newest = t
				}
			}
		}
	}
	var freshness any
	if !newest.IsZero() {
		seconds := int64(now.Sub(newest) / time.Second)
		if seconds < 0 {
			seconds = 0
		}
		freshness = seconds
	}
	return gin.H{"projectId": pid, "observedAt": timestamp(now), "quality": "authoritative-project-query", "freshnessSeconds": freshness, "truncated": truncated, "items": items}, nil
}

func (s *Server) executeChatReadTool(ctx context.Context, uid, pid int32, name string, arguments json.RawMessage) (gin.H, error) {
	input, err := parseChatToolInput(name, arguments)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if _, err = s.projectAccess(ctx, q, uid, pid, "project:view"); err != nil {
		return nil, errors.New("PROJECT_ACCESS_DENIED")
	}
	var raw []json.RawMessage
	switch name {
	case "query_devices":
		raw, err = q.ChatQueryDevices(ctx, sqlcgen.ChatQueryDevicesParams{ProjectID: pid, FilterDevices: input.FilterDevices, DeviceIds: input.DeviceIDs})
	case "query_tasks":
		raw, err = q.ChatQueryTasks(ctx, sqlcgen.ChatQueryTasksParams{ProjectID: pid, ResultLimit: int32(input.Limit + 1)})
	case "query_issues":
		raw, err = q.ChatQueryIssues(ctx, sqlcgen.ChatQueryIssuesParams{ProjectID: pid, ResultLimit: int32(input.Limit + 1)})
	case "query_assets":
		raw, err = q.ChatQueryAssets(ctx, pid)
	case "query_tracks":
		raw, err = q.ChatQueryTracks(ctx, pid)
	case "query_map_context":
		raw, err = q.ChatQueryMapContext(ctx, pid)
	}
	if err != nil {
		return nil, err
	}
	rows, err := decodeSnapshotRows(raw)
	if err != nil {
		return nil, err
	}
	result, err := formatChatToolResult(pid, name, rows, input.Limit, time.Now())
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
