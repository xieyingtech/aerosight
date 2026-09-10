package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func decodeSnapshotRows(raw []json.RawMessage) ([]gin.H, error) {
	out := make([]gin.H, 0, len(raw))
	for _, r := range raw {
		var row gin.H
		if err := json.Unmarshal(r, &row); err != nil {
			return nil, err
		}
		// node-postgres serialized top-level timestamp columns as UTC millisecond ISO strings.
		for key, value := range row {
			if strings.HasSuffix(key, "At") {
				if text, ok := value.(string); ok {
					if date, err := time.Parse(time.RFC3339Nano, text); err == nil {
						row[key] = timestamp(date)
					} else if date, err := time.Parse("2006-01-02T15:04:05.999999999", text); err == nil {
						row[key] = timestamp(date)
					}
				}
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *Server) projectSnapshot(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	result, err := s.readSnapshot(c.Request.Context(), currentUser(c).ID, pid)
	if errors.Is(err, sql.ErrNoRows) {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	if err != nil {
		s.logger.Error("snapshot failed", "error", err)
		s.failure(c, 500, "SNAPSHOT_FAILED")
		return
	}
	c.JSON(200, result)
}
func (s *Server) readSnapshot(ctx context.Context, uid, pid int32) (gin.H, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	project, err := q.SnapshotProjectScope(ctx, sqlcgen.SnapshotProjectScopeParams{ID: pid, UserID: uid})
	if err != nil {
		return nil, err
	}
	rawGrants, err := q.SnapshotDeviceGrants(ctx, sqlcgen.SnapshotDeviceGrantsParams{ProjectID: pid, TeamID: project.TeamId, UserID: uid})
	if err != nil {
		return nil, err
	}
	grants, err := decodeSnapshotRows(rawGrants)
	if err != nil {
		return nil, err
	}
	result := gin.H{"project": gin.H{"id": project.ID, "name": project.Name, "teamId": project.TeamId, "dependencyHealth": project.DependencyHealth}, "consistency": "repeatable-read", "regions": []gin.H{}}
	// Keep all reads on this transaction's snapshot; never fan out onto pool connections.
	reads := []struct {
		key  string
		read func(context.Context, int32) ([]json.RawMessage, error)
	}{
		{"devices", q.SnapshotDevices}, {"tracks", q.SnapshotTracks}, {"activeTasks", q.SnapshotActiveTasks}, {"taskSteps", q.SnapshotTaskSteps}, {"algorithmRuns", q.SnapshotAlgorithmRuns}, {"liveStreams", q.SnapshotLiveStreams}, {"realtimeChannels", q.SnapshotRealtimeChannels}, {"diagnostics", q.SnapshotDiagnostics}, {"mediaPoints", q.SnapshotMedia}, {"suspectedConstruction", q.SnapshotSuspectedConstruction}, {"openAlerts", q.SnapshotAlerts}, {"openIssues", q.SnapshotIssues},
	}
	for _, read := range reads {
		raw, err := read.read(ctx, pid)
		if err != nil {
			return nil, err
		}
		rows, err := decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		result[read.key] = rows
	}
	for _, device := range result["devices"].([]gin.H) {
		projectCapabilities(device, project.Role, grants)
	}
	now := time.Now()
	var latest time.Time
	for _, key := range []string{"devices", "tracks", "activeTasks", "liveStreams", "realtimeChannels", "mediaPoints", "openIssues"} {
		for _, row := range result[key].([]gin.H) {
			for _, field := range []string{"capturedAt", "updatedAt", "createdAt", "lastSeenAt"} {
				if str, ok := row[field].(string); ok {
					if date, err := time.Parse(time.RFC3339Nano, str); err == nil && date.After(latest) {
						latest = date
					}
				}
			}
		}
	}
	var latestAt any
	if !latest.IsZero() {
		latestAt = timestamp(latest)
	}
	result["generatedAt"] = timestamp(now)
	result["freshness"] = gin.H{"latestCapturedAt": latestAt, "isRealtime": !latest.IsZero() && now.Sub(latest) <= 120*time.Second}
	health, availability := snapshotHealth(project.DependencyHealth)
	result["health"] = health
	result["availability"] = availability
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func snapshotHealth(raw json.RawMessage) (gin.H, gin.H) {
	states := map[string]string{}
	_ = json.Unmarshal(raw, &states)
	names := []string{"database", "object_storage", "algorithm_service", "model_service", "device_adapter"}
	reasons := []string{}
	for _, name := range names {
		if states[name] != "unavailable" && states[name] != "disabled" {
			states[name] = "available"
		}
		if states[name] == "unavailable" {
			reasons = append(reasons, strings.ToUpper(name)+"_UNAVAILABLE")
		}
	}
	available := states["database"] == "available"
	status := "healthy"
	if !available {
		status = "unavailable"
	} else if len(reasons) > 0 {
		status = "degraded"
	}
	capabilities := gin.H{}
	for name, dependency := range map[string]string{"media_ingestion": "object_storage", "algorithm_execution": "algorithm_service", "ai_generation": "model_service", "realtime_device_control": "device_adapter"} {
		state := states[dependency]
		if state == "unavailable" {
			state = "degraded"
		}
		capabilities[name] = state
	}
	capabilities["historical_queries"] = "available"
	if !available {
		capabilities["historical_queries"] = "degraded"
	}
	layers := gin.H{"devices": "available", "tasks": "available", "media": "available", "issues": "available", "alerts": "available", "liveStreams": "available", "suspectedConstruction": "available", "regions": "not-configured"}
	if capabilities["realtime_device_control"] == "degraded" {
		layers["liveStreams"] = "degraded"
	}
	if capabilities["algorithm_execution"] == "degraded" {
		layers["suspectedConstruction"] = "degraded"
	}
	return gin.H{"status": status, "ready": available, "historicalDataAvailable": available, "degradationReasons": reasons, "capabilityAvailability": capabilities}, layers
}

func projectCapabilities(device gin.H, role string, grants []gin.H) {
	capabilities := []gin.H{}
	raw, _ := device["rawCapabilities"].([]any)
	for _, item := range raw {
		cap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		code, _ := cap["code"].(string)
		allowed := role == "owner" || role == "admin"
		denied := false
		for _, grant := range grants {
			pattern, _ := grant["actionPattern"].(string)
			matches := pattern == "*" || pattern == code || (strings.HasSuffix(pattern, ".*") && strings.HasPrefix(code, strings.TrimSuffix(pattern, "*")))
			scope := grant["scopeType"] == "project" || (grant["scopeType"] == "device" && grant["deviceId"] == device["id"]) || (grant["scopeType"] == "device_type" && grant["deviceTypeId"] == device["deviceTypeId"])
			if matches && scope {
				if grant["effect"] == "deny" {
					denied = true
				}
				if grant["effect"] == "allow" {
					allowed = true
				}
			}
		}
		allowed = allowed && !denied
		cap["authorized"] = allowed
		actions := []gin.H{}
		var reason any
		if !allowed {
			reason = "当前账号没有该操作权限"
		} else if cap["availability"] != "available" {
			reason = cap["reason"]
			if reason == nil {
				reason = "设备能力当前不可用"
			}
		} else if device["status"] != "online" {
			reason = "设备不在线，暂时无法执行"
		}
		if allowed {
			for _, action := range capabilityActions(code) {
				action["capabilityCode"] = code
				action["risk"] = cap["risk"]
				action["enabled"] = reason == nil
				action["unavailableReason"] = reason
				actions = append(actions, action)
			}
		}
		cap["actions"] = actions
		capabilities = append(capabilities, cap)
	}
	device["capabilities"] = capabilities
	channels := device["rawChannels"]
	if channels == nil {
		channels = []any{}
	}
	device["channels"] = channels
	delete(device, "rawCapabilities")
	delete(device, "rawChannels")
}
