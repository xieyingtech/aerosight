package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) algorithmRunRoutes() {
	group := s.router.Group("/api/projects/:id/algorithm-runs", s.requireUser, s.timeout)
	group.POST("", s.startAlgorithmRun)
	group.POST("/:runId/retry", s.retryAlgorithmRun)
	group.GET("", func(c *gin.Context) {
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			rows, err := q.ListAlgorithmRuns(c.Request.Context(), a.ProjectID)
			if err != nil {
				return nil, err
			}
			return decodeSnapshotRows(rows)
		})
	})
	group.GET("/:runId", func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("runId"))
		if err != nil {
			s.failure(c, 404, "ALGORITHM_RUN_NOT_FOUND")
			return
		}
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			raw, e := q.ReadAlgorithmRunDetail(c.Request.Context(), sqlcgen.ReadAlgorithmRunDetailParams{ProjectID: a.ProjectID, ID: id})
			if e != nil {
				return nil, e
			}
			rows, e := decodeSnapshotRows([]json.RawMessage{raw})
			if e != nil {
				return nil, e
			}
			run := rows[0]
			attempts, e := q.ReadAlgorithmRunAttempts(c.Request.Context(), sqlcgen.ReadAlgorithmRunAttemptsParams{ProjectID: a.ProjectID, AlgorithmRunID: id})
			if e != nil {
				return nil, e
			}
			decoded, e := decodeSnapshotRows(attempts)
			if e != nil {
				return nil, e
			}
			view := algorithmRunDiagnostics(run, effectivePermissions(a.Role, a.Permissions), time.Now())
			// The old server-rendered page used only the safe derived view. Never send
			// the private worker input snapshot (signed URLs/callback token) to browsers.
			delete(run, "inputSnapshot")
			return gin.H{"run": run, "attempts": decoded, "view": view}, nil
		})
	})
}

func algorithmRunDiagnostics(run gin.H, permissions map[string]bool, now time.Time) gin.H {
	object := func(v any) map[string]any {
		m, ok := v.(map[string]any)
		if !ok {
			return map[string]any{}
		}
		return m
	}
	str := func(v any) any {
		s, ok := v.(string)
		if !ok {
			return nil
		}
		return s
	}
	number := func(v any) any {
		n, ok := v.(float64)
		if !ok {
			return nil
		}
		return n
	}
	snapshot := object(run["inputSnapshot"])
	asset := object(snapshot["inputAsset"])
	definition := object(snapshot["definition"])
	canonical := object(run["canonicalResult"])
	source := object(canonical["source"])
	diagnostics := []string{}
	if values, ok := canonical["mappingDiagnostics"].([]any); ok {
		for _, v := range values {
			if s, ok := v.(string); ok {
				diagnostics = append(diagnostics, s)
			}
		}
	}
	var duration, raw any
	if start, ok := run["startedAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, start); err == nil {
			end := now
			if finish, ok := run["finishedAt"].(string); ok {
				if parsed, e := time.Parse(time.RFC3339Nano, finish); e == nil {
					end = parsed
				}
			}
			duration = max(int64(0), end.UnixMilli()-t.UnixMilli())
		}
	}
	if key, ok := run["rawResultObjectKey"].(string); ok && key != "" {
		raw = gin.H{"objectKey": key, "checksumSha256": run["rawResultChecksumSha256"]}
	}
	version := definition["configurationSnapshotId"]
	if version == nil {
		version = definition["definitionVersionId"]
	}
	return gin.H{"input": gin.H{"assetId": number(asset["assetId"]), "assetVersion": number(asset["version"]), "checksumSha256": str(asset["checksumSha256"]), "mimeType": str(asset["mimeType"]), "parameters": object(snapshot["parameters"]), "context": object(snapshot["context"])}, "provenance": gin.H{"configurationSnapshotId": number(version), "modelOrProcess": str(definition["modelOrProcess"]), "modelRevision": str(source["modelRevision"]), "modelDigest": str(source["modelDigest"]), "mappingVersion": str(definition["mappingVersion"]), "providerType": str(definition["providerType"])}, "durationMs": duration, "diagnostics": diagnostics, "rawResult": raw, "retryAllowed": permissions["algorithm:manage"] && (run["status"] == "failed" || run["status"] == "timed_out")}
}

type algorithmRunInput struct {
	SnapshotID int64
	AssetID    int32
	Parameters map[string]any
}

func parseAlgorithmRunInput(raw map[string]any) (algorithmRunInput, error) {
	bad := errors.New("ALGORITHM_RUN_INPUT_INVALID")
	out := algorithmRunInput{Parameters: map[string]any{}}
	coerce := func(v any, limit float64) (int64, bool) {
		var n float64
		switch value := v.(type) {
		case float64:
			n = value
		case string:
			var e error
			n, e = strconv.ParseFloat(strings.TrimSpace(value), 64)
			if e != nil {
				return 0, false
			}
		case bool:
			if value {
				n = 1
			}
		default:
			return 0, false
		}
		return int64(n), !math.IsNaN(n) && n > 0 && n <= limit && math.Trunc(n) == n
	}
	for key := range raw {
		if key != "configurationSnapshotId" && key != "definitionVersionId" && key != "assetId" && key != "parameters" {
			return out, bad
		}
	}
	var legacy, current int64
	for _, key := range []string{"configurationSnapshotId", "definitionVersionId"} {
		if v, yes := raw[key]; yes {
			id, ok := coerce(v, 9007199254740991)
			if !ok {
				return out, bad
			}
			if key == "configurationSnapshotId" {
				current = id
			} else {
				legacy = id
			}
		}
	}
	out.SnapshotID = current
	if current == 0 {
		out.SnapshotID = legacy
	}
	if out.SnapshotID == 0 {
		return out, bad
	}
	asset, ok := coerce(raw["assetId"], math.MaxInt32)
	if !ok {
		return out, bad
	}
	out.AssetID = int32(asset)
	if value, present := raw["parameters"]; present {
		out.Parameters, ok = value.(map[string]any)
		if !ok || out.Parameters == nil {
			return out, bad
		}
	}
	return out, nil
}

func (s *Server) algorithmRunFailure(c *gin.Context, err error, retry bool) {
	code, status := "ALGORITHM_RUN_CREATE_FAILED", 400
	if retry {
		code, status = "ALGORITHM_RUN_RETRY_FAILED", 409
	}
	switch err.Error() {
	case "PROJECT_ACCESS_DENIED":
		code, status = err.Error(), 403
	case "ALGORITHM_RUN_NOT_FOUND", "ALGORITHM_RUN_NOT_RETRYABLE", "ALGORITHM_RUN_SOURCE_NOT_AVAILABLE", "ALGORITHM_INPUT_ASSET_CHECKSUM_REQUIRED", "ALGORITHM_RUN_INPUT_INVALID":
		code = err.Error()
	}
	s.failure(c, status, code)
}

func (s *Server) startAlgorithmRun(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 400, "ALGORITHM_RUN_INPUT_INVALID")
		return
	}
	var raw map[string]any
	if strictJSON(c, &raw) != nil {
		s.failure(c, 400, "ALGORITHM_RUN_INPUT_INVALID")
		return
	}
	input, err := parseAlgorithmRunInput(raw)
	if err != nil {
		s.algorithmRunFailure(c, err, false)
		return
	}
	uid := currentUser(c).ID
	ctx := c.Request.Context()
	a, err := s.projectAccess(ctx, s.queries, uid, pid, "algorithm:manage")
	if err != nil {
		s.algorithmRunFailure(c, errors.New("PROJECT_ACCESS_DENIED"), false)
		return
	}
	id := uuid.New()
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "algorithm_run.create", ResourceType: "algorithm_configuration_snapshot", ResourceID: strconv.FormatInt(input.SnapshotID, 10), Input: gin.H{"configurationSnapshotId": input.SnapshotID, "assetId": input.AssetID, "parameters": input.Parameters}, PolicyResult: map[string]any{"permission": "algorithm:manage", "configurationSnapshotRequired": true}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "algorithm:manage", false), func(w *database.WriteTx) (gin.H, error) {
		source, e := w.Queries.ReadAlgorithmRunSource(ctx, sqlcgen.ReadAlgorithmRunSourceParams{ProjectID: pid, SnapshotID: input.SnapshotID, AssetID: input.AssetID})
		if errors.Is(e, sql.ErrNoRows) {
			return nil, errors.New("ALGORITHM_RUN_SOURCE_NOT_AVAILABLE")
		}
		if e != nil {
			return nil, e
		}
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(source.ChecksumSha256) {
			return nil, errors.New("ALGORITHM_INPUT_ASSET_CHECKSUM_REQUIRED")
		}
		snapshot, e := json.Marshal(gin.H{"schemaVersion": "aerosight.algorithm.input/v1", "runId": id.String(), "projectId": pid, "definition": gin.H{"configurationSnapshotId": source.ConfigurationSnapshotID, "providerType": source.ProviderType, "modelOrProcess": source.ModelOrProcess, "executionMode": source.ExecutionMode, "mappingVersion": source.MappingVersion}, "inputAsset": gin.H{"assetId": source.AssetID, "version": source.AssetVersion, "checksumSha256": source.ChecksumSha256, "mimeType": source.MimeType, "accessUrl": "", "accessExpiresAt": "1970-01-01T00:00:00.000Z"}, "context": gin.H{"requestedByUserId": uid, "requestedAt": time.Now().UTC().Format("2006-01-02T15:04:05.000Z")}, "parameters": input.Parameters})
		if e != nil {
			return nil, e
		}
		parameters, e := json.Marshal(input.Parameters)
		if e != nil {
			return nil, e
		}
		e = w.Queries.InsertCatalogAlgorithmRun(ctx, sqlcgen.InsertCatalogAlgorithmRunParams{ID: id, ProjectID: pid, TeamID: source.TeamID, AlgorithmDefinitionVersionID: source.ConfigurationSnapshotID, InputAssetID: source.AssetID, IdempotencyKey: fmt.Sprintf("catalog:%d:asset:%d:%s", source.ConfigurationSnapshotID, source.AssetID, id), Parameters: parameters, Snapshot: snapshot})
		if e != nil {
			return nil, e
		}
		_, e = w.Publish(ctx, database.ProjectEvent{ProjectID: pid, TeamID: source.TeamID, EventID: "algorithm-run-requested:" + id.String(), EventType: "algorithm.run.requested", Payload: gin.H{"runId": id.String()}})
		return gin.H{"runId": id.String()}, e
	})
	if err != nil {
		s.algorithmRunFailure(c, err, false)
		return
	}
	c.JSON(202, result)
}

func (s *Server) retryAlgorithmRun(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 409, "ALGORITHM_RUN_NOT_FOUND")
		return
	}
	sourceID, err := uuid.Parse(c.Param("runId"))
	if err != nil {
		s.failure(c, 409, "ALGORITHM_RUN_NOT_FOUND")
		return
	}
	uid := currentUser(c).ID
	ctx := c.Request.Context()
	a, err := s.projectAccess(ctx, s.queries, uid, pid, "algorithm:manage")
	if err != nil {
		s.algorithmRunFailure(c, errors.New("PROJECT_ACCESS_DENIED"), true)
		return
	}
	id := uuid.New()
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "algorithm_run.retry", ResourceType: "algorithm_run", ResourceID: sourceID.String(), Input: gin.H{"sourceRunId": sourceID.String()}, PolicyResult: map[string]any{"permission": "algorithm:manage", "terminalFailureOnly": true}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "algorithm:manage", false), func(w *database.WriteTx) (gin.H, error) {
		source, e := w.Queries.LockAlgorithmRetrySource(ctx, sqlcgen.LockAlgorithmRetrySourceParams{ProjectID: pid, ID: sourceID})
		if errors.Is(e, sql.ErrNoRows) {
			return nil, errors.New("ALGORITHM_RUN_NOT_FOUND")
		}
		if e != nil {
			return nil, e
		}
		if source.Status != "failed" && source.Status != "timed_out" {
			return nil, errors.New("ALGORITHM_RUN_NOT_RETRYABLE")
		}
		var snapshot map[string]any
		if e = json.Unmarshal(source.InputSnapshotJson, &snapshot); e != nil {
			return nil, e
		}
		if snapshot == nil {
			return nil, errors.New("invalid snapshot")
		}
		delete(snapshot, "callback")
		snapshot["runId"] = id.String()
		context, _ := snapshot["context"].(map[string]any)
		if context == nil {
			context = map[string]any{}
		}
		context["requestedByUserId"] = uid
		context["requestedAt"] = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		snapshot["context"] = context
		if asset, ok := snapshot["inputAsset"].(map[string]any); ok {
			asset["accessUrl"] = ""
			asset["accessExpiresAt"] = "1970-01-01T00:00:00.000Z"
		}
		encoded, e := json.Marshal(snapshot)
		if e != nil {
			return nil, e
		}
		e = w.Queries.InsertCatalogAlgorithmRun(ctx, sqlcgen.InsertCatalogAlgorithmRunParams{ID: id, ProjectID: pid, TeamID: source.TeamID, AlgorithmDefinitionVersionID: source.AlgorithmDefinitionVersionID, InputAssetID: source.InputAssetID, TaskRunID: source.TaskRunID, DeviceID: source.DeviceID, IdempotencyKey: sourceID.String() + ":retry:" + id.String(), Parameters: source.ParametersJson, Snapshot: encoded})
		if e != nil {
			return nil, e
		}
		_, e = w.Publish(ctx, database.ProjectEvent{ProjectID: pid, TeamID: source.TeamID, EventID: "algorithm-run-requested:" + id.String(), EventType: "algorithm.run.requested", Payload: gin.H{"runId": id.String()}})
		return gin.H{"runId": id.String()}, e
	})
	if err != nil {
		s.algorithmRunFailure(c, err, true)
		return
	}
	c.JSON(200, result)
}
