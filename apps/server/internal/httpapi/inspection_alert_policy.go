package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"aerosight/server/internal/database"
	"aerosight/server/internal/inspection"
	"github.com/gin-gonic/gin"
)

func (s *Server) inspectionPolicyFailure(c *gin.Context, err error) {
	code := err.Error()
	status := 400
	if errors.Is(err, sql.ErrNoRows) || strings.Contains(code, "SCOPE_INVALID") {
		code = "INSPECTION_RESOURCE_NOT_FOUND"
		status = 404
	}
	if strings.Contains(code, "ACCESS_DENIED") {
		status = 403
	}
	if strings.Contains(code, "IMMUTABLE") || strings.Contains(code, "IDEMPOTENCY") {
		status = 409
	}
	if !strings.HasPrefix(code, "INSPECTION_") && !strings.Contains(code, "IDEMPOTENCY") && !strings.Contains(code, "ACCESS_DENIED") {
		code = "INSPECTION_POLICY_FAILED"
		status = 500
	}
	s.failure(c, status, code)
}

func (s *Server) inspectionAlertPolicy(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, "INSPECTION_RESOURCE_NOT_FOUND")
		return
	}
	cid, err := strconv.ParseInt(c.Param("connectorId"), 10, 64)
	if err != nil || cid <= 0 {
		s.failure(c, 404, "INSPECTION_RESOURCE_NOT_FOUND")
		return
	}
	uid := currentUser(c).ID
	ctx := c.Request.Context()
	permission := "project:view"
	if c.Request.Method != "GET" {
		permission = "device:configure"
	}
	access, err := s.projectAccess(ctx, s.queries, uid, pid, permission)
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	if c.Request.Method == "GET" {
		var managed bool
		err = s.db.QueryRowContext(ctx, `select coalesce(policy.task_managed_alerts,false) from device_adapters adapter left join inspection_connector_policies policy on policy.project_id=adapter.project_id and policy.connector_instance_id=adapter.id where adapter.project_id=$1 and adapter.id=$2 and adapter.adapter_type='dji-flighthub2'`, pid, cid).Scan(&managed)
		if err != nil {
			s.inspectionPolicyFailure(c, err)
			return
		}
		rows, err := s.db.QueryContext(ctx, `select source.remote_resource_id,ownership.ownership,ownership.task_run_id,resource.summary_json,resource.status
   from inspection_alert_sources source join inspection_flight_ownership ownership on ownership.project_id=source.project_id and ownership.connector_instance_id=source.connector_instance_id and ownership.remote_flight_id=source.remote_flight_id
   join connector_remote_resources resource on resource.project_id=source.project_id and resource.id=source.remote_resource_id
   where source.project_id=$1 and source.connector_instance_id=$2 and ownership.ownership in('pending','task') order by source.remote_resource_id limit 101`, pid, cid)
		if err != nil {
			s.inspectionPolicyFailure(c, err)
			return
		}
		defer rows.Close()
		records := []gin.H{}
		for rows.Next() {
			var id int64
			var state, status string
			var run sql.NullInt64
			var summary []byte
			if err = rows.Scan(&id, &state, &run, &summary, &status); err != nil {
				s.inspectionPolicyFailure(c, err)
				return
			}
			var runID any
			if run.Valid {
				runID = run.Int64
			}
			records = append(records, gin.H{"resourceId": id, "ownership": state, "taskRunId": runID, "summary": json.RawMessage(summary), "status": status})
		}
		if err = rows.Err(); err != nil {
			s.inspectionPolicyFailure(c, err)
			return
		}
		truncated := len(records) > 100
		if truncated {
			records = records[:100]
		}
		c.JSON(200, gin.H{"taskManagedAlerts": managed, "heldAlerts": records, "truncated": truncated, "canConfigure": effectivePermissions(access.Role, access.Permissions)["device:configure"]})
		return
	}
	var body struct {
		Action            string `json:"action"`
		TaskManagedAlerts *bool  `json:"taskManagedAlerts"`
		ResourceID        int64  `json:"resourceId"`
		IdempotencyKey    string `json:"idempotencyKey"`
	}
	if strictJSON(c, &body) != nil || len(body.IdempotencyKey) < 1 || len(body.IdempotencyKey) > 200 || strings.TrimSpace(body.IdempotencyKey) != body.IdempotencyKey ||
		(body.Action != "set-policy" && body.Action != "confirm-legacy") || body.Action == "set-policy" && (body.TaskManagedAlerts == nil || body.ResourceID != 0) || body.Action == "confirm-legacy" && (body.ResourceID <= 0 || body.TaskManagedAlerts != nil) {
		s.failure(c, 400, "INSPECTION_POLICY_INPUT_INVALID")
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: body.IdempotencyKey, Action: "inspection.alert_policy." + body.Action, ResourceType: "connector", ResourceID: strconv.FormatInt(cid, 10), Input: body}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "device:configure", false), func(w *database.WriteTx) (database.IdempotentResult[gin.H], error) {
		return database.Idempotent(ctx, w, database.IdempotencyContext{ProjectID: pid, TeamID: access.TeamID, ActorKey: "user:" + strconv.Itoa(int(uid)), Operation: "inspection.alert-policy:" + strconv.FormatInt(cid, 10), Key: body.IdempotencyKey, Request: body}, func() (gin.H, error) {
			if _, err := inspection.LockAlertConnector(ctx, w.Tx, int(pid), cid); err != nil {
				return nil, err
			}
			if body.Action == "set-policy" {
				if err := inspection.SetAlertPolicy(ctx, w.Tx, int(pid), cid, *body.TaskManagedAlerts); err != nil {
					return nil, err
				}
				return gin.H{"taskManagedAlerts": *body.TaskManagedAlerts}, nil
			}
			var flightID string
			if err := w.Tx.QueryRowContext(ctx, "select remote_flight_id from inspection_alert_sources where project_id=$1 and connector_instance_id=$2 and remote_resource_id=$3", pid, cid, body.ResourceID).Scan(&flightID); err != nil {
				return nil, err
			}
			if err := inspection.ReleaseLegacyFlight(ctx, w.Tx, int(pid), cid, flightID); err != nil {
				return nil, err
			}
			return gin.H{"resourceId": body.ResourceID, "ownership": "legacy"}, nil
		})
	})
	if err != nil {
		s.inspectionPolicyFailure(c, err)
		return
	}
	result.Value["replayed"] = result.Replayed
	c.JSON(200, result.Value)
}
