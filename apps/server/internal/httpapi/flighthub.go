package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/flighthub"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/httprate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

type flightHubProjectClient interface {
	ListProjects(context.Context, string) ([]flighthub.Project, error)
}
type flightHubService struct {
	client        flightHubProjectClient
	enabled       bool
	secret        string
	discoveryRate *httprate.RateLimiter
}

func (s *Server) AttachFlightHub(client flightHubProjectClient, enabled bool, secret string) {
	s.flightHub = &flightHubService{client: client, enabled: enabled, secret: secret, discoveryRate: httprate.NewRateLimiter(5, time.Minute)}
}
func (s *Server) flightHubRoutes() {
	group := s.router.Group("/api/projects/:id/connectors/dji-flighthub", s.requireUser, s.timeout, func(c *gin.Context) { c.Header("Cache-Control", "no-store, max-age=0") })
	s.router.POST("/api/projects/:id/devices/:deviceId/flighthub-control-sessions", s.requireUser, s.timeout, s.fhControlSession)
	s.router.PATCH("/api/projects/:id/devices/:deviceId/flighthub-control-sessions/:sessionId", s.requireUser, s.timeout, s.fhControlSession)
	group.GET("", s.listFlightHub)
	group.GET("/activity", s.listFlightHubActivity)
	group.GET("/live-media", s.readFlightHubLiveMedia)
	group.GET("/:connectorId/controlled-operations", s.fhControlledOperations)
	group.POST("/:connectorId/diagnostics", s.fhLifecycleMain)
	group.PUT("/:connectorId", s.fhLifecycleMain)
	group.GET("/:connectorId/diagnostics", s.fhWorkspace("Diagnostics"))
	group.POST("/:connectorId/management", s.fhJoinCode)
	group.GET("/:connectorId/management", s.fhWorkspace("Management"))
	s.router.GET("/api/projects/:id/flight-operations", s.requireUser, s.timeout, s.fhWorkspace("FlightOps"))
	s.router.GET("/api/projects/:id/geospatial", s.requireUser, s.timeout, s.fhWorkspace("Geo"))
	s.router.GET("/api/projects/:id/models", s.requireUser, s.timeout, s.fhWorkspace("Models"))
	for _, kind := range []string{"live", "geospatial"} {
		group.GET("/:connectorId/"+kind+"-actions", s.fhResourceAction(kind))
		group.POST("/:connectorId/"+kind+"-actions", s.fhResourceAction(kind))
	}
	for _, kind := range []string{"flight", "device-admin"} {
		group.GET("/:connectorId/"+kind+"-actions", s.fhGovernedAction(kind))
		group.POST("/:connectorId/"+kind+"-actions", s.fhGovernedAction(kind))
	}
	group.GET("/:connectorId/management-actions", s.fhMemberAction)
	group.PUT("/:connectorId/management-actions", s.fhMemberAction)
	group.POST("/:connectorId/management-actions", s.fhMemberAction)
	group.GET("/:connectorId/model-actions", s.fhModelAction)
	group.POST("/:connectorId/model-actions", s.fhModelAction)
	group.POST("/projects", s.discoverFlightHub)
	group.POST("", s.createFlightHub)
	group.PUT("/:connectorId/token", s.updateFlightHubToken)
	group.POST("/:connectorId/sync", s.syncFlightHub)
	group.DELETE("/:connectorId", s.disconnectFlightHub)
	s.router.GET("/api/projects/:id/features", s.requireUser, s.timeout, func(c *gin.Context) {
		s.scopedRead(c, func(_ *sqlcgen.Queries, _ sqlcgen.GetProjectAccessRow) (any, error) {
			return gin.H{"flightHubEnabled": s.flightHub != nil && s.flightHub.enabled}, nil
		})
	})
}

type flightHubError struct {
	code       string
	retryAfter time.Duration
}

func (e *flightHubError) Error() string { return e.code }
func (s *Server) flightHubFailure(c *gin.Context, err error, discovery bool) {
	code := "configuration_unavailable"
	retry := time.Duration(0)
	var safe *flightHubError
	var upstream *flighthub.APIError
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &safe):
		code = safe.code
		retry = safe.retryAfter
	case errors.As(err, &upstream):
		code = upstream.SafeCode
		retry = upstream.RetryAfter
	case errors.As(err, &pg) && pg.Code == "23505":
		code = "duplicate_connection"
	case errors.Is(err, sql.ErrNoRows):
		code = "connector_not_found"
	case errors.Is(err, context.DeadlineExceeded):
		code = "request_timeout"
	default:
		if err != nil && err.Error() == "PROJECT_ACCESS_DENIED" {
			code = "access_denied"
		}
	}
	status := 503
	if discovery {
		status = 502
	}
	switch code {
	case "invalid_request":
		status = 400
	case "access_denied", "scope_forbidden":
		status = 403
	case "credential_invalid":
		status = 401
	case "connector_not_found":
		status = 404
	case "scope_not_found":
		status = 409
		if strings.HasSuffix(c.Request.URL.Path, "/token") {
			status = 503
		}
		if discovery {
			status = 404
		}
	case "project_access_changed", "duplicate_connection", "connector_disabled", "connector_not_disabled":
		status = 409
	case "rate_limited":
		status = 429
	case "request_timeout":
		status = 504
	}
	if retry > 0 {
		c.Header("Retry-After", strconv.Itoa(int((retry+time.Second-1)/time.Second)))
	}
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code}})
}
func (s *Server) flightHubManager(c *gin.Context) (sqlcgen.GetProjectAccessRow, bool) {
	pid, err := projectID(c)
	var access sqlcgen.GetProjectAccessRow
	if err == nil {
		access, err = s.projectAccess(c.Request.Context(), s.queries, currentUser(c).ID, pid, "device:configure")
	}
	if err != nil || (access.Role != "owner" && access.Role != "admin") {
		s.flightHubFailure(c, &flightHubError{code: "access_denied"}, false)
		return access, false
	}
	return access, true
}
func (s *Server) requireFlightHubClient(c *gin.Context) bool {
	if s.flightHub == nil || !s.flightHub.enabled || s.flightHub.client == nil {
		s.flightHubFailure(c, &flightHubError{code: "access_denied"}, false)
		return false
	}
	return true
}
func strictJSON(c *gin.Context, dest any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("INVALID_JSON")
	}
	return nil
}
func utf16Length(value string) int { return len(utf16.Encode([]rune(value))) }
func flightHubInput(c *gin.Context, withProject bool) (string, string, error) {
	var in struct {
		Token       string          `json:"token"`
		ProjectUUID json.RawMessage `json:"projectUuid,omitempty"`
	}
	if err := strictJSON(c, &in); err != nil {
		return "", "", &flightHubError{code: "invalid_request"}
	}
	token := strings.TrimSpace(in.Token)
	if token == "" || utf16Length(token) > 16384 || (!withProject && len(in.ProjectUUID) > 0) {
		return "", "", &flightHubError{code: "invalid_request"}
	}
	if !withProject {
		return token, "", nil
	}
	var project string
	if len(in.ProjectUUID) == 0 || json.Unmarshal(in.ProjectUUID, &project) != nil || project == "" {
		return "", "", &flightHubError{code: "invalid_request"}
	}
	parsed, err := uuid.Parse(project)
	if err != nil || !strings.EqualFold(parsed.String(), project) {
		return "", "", &flightHubError{code: "invalid_request"}
	}
	return token, parsed.String(), nil
}
func connectorID(c *gin.Context) (int64, error) {
	value := c.Param("connectorId")
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, &flightHubError{code: "connector_not_found"}
		}
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, &flightHubError{code: "connector_not_found"}
	}
	return id, nil
}
func scopeFingerprint(value string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(value)))
	return hex.EncodeToString(hash[:])[:12]
}
func syncPayload(id int64, trigger string) gin.H {
	return gin.H{"connectorInstanceId": strconv.FormatInt(id, 10), "connectorKey": flighthub.ConnectorKey, "discoveryMode": "poll", "trigger": trigger}
}
func flightHubAudit(c *gin.Context, a sqlcgen.GetProjectAccessRow, action string, id int64, input gin.H) database.AuditContext {
	resource := ""
	if id > 0 {
		resource = strconv.FormatInt(id, 10)
	}
	return database.AuditContext{ProjectID: a.ProjectID, TeamID: a.TeamID, ActorUserID: currentUser(c).ID, RequestID: c.GetHeader("X-Request-ID"), Action: action, ResourceType: "connector", ResourceID: resource, Input: input, PolicyResult: map[string]any{"permission": "device:configure", "role": a.Role}}
}
func (s *Server) selectFlightHubProject(ctx context.Context, token, id string) (flighthub.Project, error) {
	projects, err := s.flightHub.client.ListProjects(ctx, token)
	if err != nil {
		return flighthub.Project{}, flightHubUpstreamError(err)
	}
	for _, project := range projects {
		if project.UUID == strings.ToLower(id) {
			return project, nil
		}
	}
	return flighthub.Project{}, &flightHubError{code: "project_access_changed"}
}

func flightHubUpstreamError(err error) error {
	if err == nil {
		return nil
	}
	var upstream *flighthub.APIError
	if errors.As(err, &upstream) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &flightHubError{code: "request_timeout"}
	}
	return &flightHubError{code: "upstream_error"}
}
func (s *Server) discoverFlightHub(c *gin.Context) {
	access, ok := s.flightHubManager(c)
	if !ok {
		return
	}
	token, _, err := flightHubInput(c, false)
	if err != nil {
		s.flightHubFailure(c, err, true)
		return
	}
	if !s.requireFlightHubClient(c) {
		return
	}
	var projects []flighthub.Project
	if s.flightHub.discoveryRate.OnLimit(c.Writer, c.Request, fmt.Sprintf("%d:%d", currentUser(c).ID, access.ProjectID)) {
		err = &flightHubError{code: "rate_limited", retryAfter: time.Minute}
	} else {
		projects, err = s.flightHub.client.ListProjects(c.Request.Context(), token)
		err = flightHubUpstreamError(err)
	}
	summary := gin.H{"status": "succeeded", "projectCount": len(projects)}
	if err != nil {
		code := "upstream_error"
		var apiErr *flighthub.APIError
		var safe *flightHubError
		if errors.As(err, &apiErr) {
			code = apiErr.SafeCode
		} else if errors.As(err, &safe) {
			code = safe.code
		}
		summary = gin.H{"status": "failed", "projectCount": 0, "errorCode": code}
	}
	audit := flightHubAudit(c, access, "connector.flighthub.discover", 0, gin.H{"operation": "project-discovery"})
	for k, v := range summary {
		audit.PolicyResult[k] = v
	}
	_, auditErr := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, access.ProjectID, access.TeamID, "device:configure", true), func(*database.WriteTx) (gin.H, error) { return summary, nil })
	if auditErr != nil {
		s.flightHubFailure(c, auditErr, true)
		return
	}
	s.logger.Info("FlightHub project discovery completed", "project_id", access.ProjectID, "status", summary["status"], "project_count", summary["projectCount"])
	if err != nil {
		s.flightHubFailure(c, err, true)
		return
	}
	out := make([]gin.H, 0, len(projects))
	for _, p := range projects {
		out = append(out, gin.H{"uuid": p.UUID, "name": p.Name, "organizationUuid": p.OrganizationUUID})
	}
	c.JSON(200, gin.H{"projects": out})
}

func (s *Server) flightHubLists(c *gin.Context, a sqlcgen.GetProjectAccessRow) (gin.H, error) {
	out := gin.H{}
	for _, read := range []struct {
		key   string
		query func(context.Context, int32) ([]json.RawMessage, error)
	}{{"connectors", s.queries.ListFlightHubConnections}, {"identities", s.queries.ListFlightHubIdentities}, {"syncRuns", s.queries.ListFlightHubSyncRuns}} {
		raw, err := read.query(c.Request.Context(), a.ProjectID)
		if err != nil {
			return nil, err
		}
		rows, err := decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		out[read.key] = rows
	}
	return out, nil
}
func (s *Server) listFlightHub(c *gin.Context) {
	a, ok := s.flightHubManager(c)
	if !ok {
		return
	}
	out, err := s.flightHubLists(c, a)
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	c.JSON(200, out)
}
func (s *Server) listFlightHubActivity(c *gin.Context) {
	a, ok := s.flightHubManager(c)
	if !ok {
		return
	}
	out, err := s.flightHubLists(c, a)
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	delete(out, "connectors")
	c.JSON(200, out)
}

func (s *Server) storeFlightHubToken(ctx context.Context, w *database.WriteTx, pid int32, id int64, token string) error {
	envelope, err := credentials.EncryptJSON(map[string]string{"token": token}, s.flightHub.secret, credentials.AAD("device-adapter", id, pid))
	if err != nil {
		return &flightHubError{code: "configuration_unavailable"}
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	count, err := w.Queries.UpdateFlightHubCredentials(ctx, sqlcgen.UpdateFlightHubCredentialsParams{ID: id, ProjectID: pid, Column3: raw})
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *Server) createFlightHub(c *gin.Context) {
	a, ok := s.flightHubManager(c)
	if !ok {
		return
	}
	token, pid, err := flightHubInput(c, true)
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	if !s.requireFlightHubClient(c) {
		return
	}
	selected, err := s.selectFlightHubProject(c.Request.Context(), token, pid)
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	audit := flightHubAudit(c, a, "connector.flighthub.create", 0, gin.H{"connectorKey": flighthub.ConnectorKey, "externalScopeFingerprint": scopeFingerprint(pid)})
	audit.PolicyResult["upstreamProjectRevalidated"] = true
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
		name := "DJI 司空 2 · " + selected.Name
		units := utf16.Encode([]rune(name))
		if len(units) > 100 {
			name = string(utf16.Decode(units[:97])) + "..."
		}
		scope := gin.H{"projectUuid": selected.UUID, "projectName": selected.Name}
		raw, _ := json.Marshal(scope)
		connector, err := w.Queries.CreateFlightHubConnector(c.Request.Context(), sqlcgen.CreateFlightHubConnectorParams{ProjectID: a.ProjectID, TeamID: a.TeamID, Name: name, DiscoveryScope: raw, ExternalScope: selected.UUID})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &flightHubError{code: "configuration_unavailable"}
		}
		if err != nil {
			return nil, err
		}
		if err = s.storeFlightHubToken(c.Request.Context(), w, a.ProjectID, connector.ID, token); err != nil {
			return nil, err
		}
		if _, err = w.Publish(c.Request.Context(), database.ProjectEvent{ProjectID: a.ProjectID, TeamID: a.TeamID, EventID: fmt.Sprintf("connector-sync:%d:%s", connector.ID, uuid.NewString()), EventType: "connector.sync.requested", Payload: syncPayload(connector.ID, "initial")}); err != nil {
			return nil, err
		}
		return gin.H{"id": strconv.FormatInt(connector.ID, 10), "connectorKey": flighthub.ConnectorKey, "connectorVersion": flighthub.ConnectorVersion, "status": connector.Status, "project": scope, "createdAt": connector.CreatedAt, "initialSyncQueued": true}, nil
	})
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	result["createdAt"] = timestamp(result["createdAt"].(time.Time))
	c.JSON(201, result)
}

func queueFlightHubSync(ctx context.Context, w *database.WriteTx, a sqlcgen.GetProjectAccessRow, id int64, trigger string) (gin.H, error) {
	key := strconv.FormatInt(id, 10)
	if err := w.Queries.LockFlightHubSyncQueue(ctx, "flightHub-sync:"+key); err != nil {
		return nil, err
	}
	event, err := w.Queries.FindQueuedFlightHubSync(ctx, sqlcgen.FindQueuedFlightHubSyncParams{ProjectID: a.ProjectID, Column2: key})
	if err == nil {
		return gin.H{"eventId": event, "deduplicated": true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	event = "connector-sync:" + key + ":" + uuid.NewString()
	if _, err = w.Publish(ctx, database.ProjectEvent{ProjectID: a.ProjectID, TeamID: a.TeamID, EventID: event, EventType: "connector.sync.requested", Payload: syncPayload(id, trigger)}); err != nil {
		return nil, err
	}
	return gin.H{"eventId": event, "deduplicated": false}, nil
}
func (s *Server) updateFlightHubToken(c *gin.Context) {
	a, ok := s.flightHubManager(c)
	if !ok {
		return
	}
	id, err := connectorID(c)
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	token, _, err := flightHubInput(c, false)
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	if !s.requireFlightHubClient(c) {
		return
	}
	current, err := s.queries.FindFlightHubConnector(c.Request.Context(), sqlcgen.FindFlightHubConnectorParams{ID: id, ProjectID: a.ProjectID})
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	selected, err := s.selectFlightHubProject(c.Request.Context(), token, current.ProjectUuid)
	if err != nil {
		safeCode := "upstream_error"
		var safe *flightHubError
		var upstream *flighthub.APIError
		if errors.As(err, &safe) {
			safeCode = safe.code
		} else if errors.As(err, &upstream) {
			safeCode = upstream.SafeCode
		}
		audit := flightHubAudit(c, a, "connector.flighthub.credential.update", id, gin.H{"externalScopeFingerprint": scopeFingerprint(current.ProjectUuid)})
		audit.PolicyResult["projectRevalidated"], audit.PolicyResult["errorCode"] = false, safeCode
		_, auditErr := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
			return gin.H{"tokenUpdated": false, "errorCode": safeCode}, nil
		})
		if auditErr != nil {
			err = auditErr
		}
		s.flightHubFailure(c, err, false)
		return
	}
	audit := flightHubAudit(c, a, "connector.flighthub.credential.update", id, gin.H{"externalScopeFingerprint": scopeFingerprint(selected.UUID)})
	audit.PolicyResult["projectRevalidated"] = true
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
		locked, err := w.Queries.LockFlightHubConnector(c.Request.Context(), sqlcgen.LockFlightHubConnectorParams{ID: id, ProjectID: a.ProjectID})
		if err != nil {
			return nil, err
		}
		if locked.ProjectUuid != selected.UUID {
			return nil, &flightHubError{code: "project_access_changed"}
		}
		if err = s.storeFlightHubToken(c.Request.Context(), w, a.ProjectID, id, token); err != nil {
			return nil, err
		}
		out, err := queueFlightHubSync(c.Request.Context(), w, a, id, "credential-update")
		if err != nil {
			return nil, err
		}
		out["id"] = strconv.FormatInt(id, 10)
		out["status"] = "connecting"
		out["tokenUpdated"] = true
		out["syncQueued"] = true
		return out, nil
	})
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	c.JSON(200, result)
}
func (s *Server) syncFlightHub(c *gin.Context) {
	a, ok := s.flightHubManager(c)
	if !ok {
		return
	}
	id, err := connectorID(c)
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	audit := flightHubAudit(c, a, "connector.flighthub.sync.request", id, gin.H{"trigger": "manual"})
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
		locked, err := w.Queries.LockFlightHubConnector(c.Request.Context(), sqlcgen.LockFlightHubConnectorParams{ID: id, ProjectID: a.ProjectID})
		if err != nil {
			return nil, err
		}
		if locked.Status == "disabled" {
			return nil, &flightHubError{code: "connector_disabled"}
		}
		out, err := queueFlightHubSync(c.Request.Context(), w, a, id, "manual")
		if err != nil {
			return nil, err
		}
		out["id"] = strconv.FormatInt(id, 10)
		out["syncQueued"] = true
		return out, nil
	})
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	c.JSON(202, result)
}
func (s *Server) disconnectFlightHub(c *gin.Context) {
	a, ok := s.flightHubManager(c)
	if !ok {
		return
	}
	id, err := connectorID(c)
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	audit := flightHubAudit(c, a, "connector.flighthub.disconnect", id, gin.H{"operation": "disable"})
	audit.PolicyResult["preservesHistory"] = true
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
		q := w.Queries
		ctx := c.Request.Context()
		if _, err := q.LockFlightHubConnector(ctx, sqlcgen.LockFlightHubConnectorParams{ID: id, ProjectID: a.ProjectID}); err != nil {
			return nil, err
		}
		if _, err := q.DisableFlightHubConnector(ctx, sqlcgen.DisableFlightHubConnectorParams{ID: id, ProjectID: a.ProjectID}); err != nil {
			return nil, err
		}
		if err := q.DisableFlightHubBindings(ctx, sqlcgen.DisableFlightHubBindingsParams{ProjectID: a.ProjectID, ConnectorInstanceID: id}); err != nil {
			return nil, err
		}
		if err := q.CancelFlightHubSyncRuns(ctx, sqlcgen.CancelFlightHubSyncRunsParams{ProjectID: a.ProjectID, ConnectorInstanceID: id}); err != nil {
			return nil, err
		}
		if err := q.CancelFlightHubSyncQueue(ctx, sqlcgen.CancelFlightHubSyncQueueParams{ProjectID: a.ProjectID, Column2: strconv.FormatInt(id, 10)}); err != nil {
			return nil, err
		}
		return gin.H{"id": strconv.FormatInt(id, 10), "status": "disabled", "disconnected": true, "historyPreserved": true}, nil
	})
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	c.JSON(200, result)
}
