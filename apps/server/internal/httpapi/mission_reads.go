package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"strconv"
)

func availableMissionActions(status string, permissions map[string]bool) []string {
	actions := []string{}
	if permissions["mission:operate"] {
		if status == "running" || status == "dispatching" {
			actions = append(actions, "pause")
		}
		if status == "paused" {
			actions = append(actions, "resume")
		}
		switch status {
		case "queued", "blocked", "ready", "dispatching", "running", "paused":
			actions = append(actions, "cancel")
		}
		switch status {
		case "dispatching", "running", "paused", "canceling":
			actions = append(actions, "emergency_stop")
		}
	}
	if permissions["mission:approve"] && (status == "blocked" || status == "ready") {
		actions = append(actions, "approve")
	}
	return actions
}
func (s *Server) missionReadRoutes() {
	tasks := s.router.Group("/api/projects/:id/tasks", s.requireUser, s.timeout)
	tasks.GET("/:taskId/workbench", s.taskWorkbench)
	tasks.POST("/:taskId/runs", s.triggerTaskRun)
	tasks.GET("", func(c *gin.Context) {
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			raw, err := q.ListTaskDefinitions(c.Request.Context(), a.ProjectID)
			if err != nil {
				return nil, err
			}
			return decodeSnapshotRows(raw)
		})
	})
	tasks.GET("/:taskId", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("taskId"), 10, 32)
		if err != nil || id <= 0 {
			s.failure(c, 404, "TASK_NOT_FOUND")
			return
		}
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			raw, err := q.GetTaskDefinition(c.Request.Context(), sqlcgen.GetTaskDefinitionParams{ProjectID: a.ProjectID, ID: int32(id)})
			if err != nil {
				return nil, err
			}
			rows, err := decodeSnapshotRows([]json.RawMessage{raw})
			if err != nil {
				return nil, err
			}
			return rows[0], nil
		})
	})
	group := s.router.Group("/api/projects/:id/task-runs", s.requireUser, s.timeout)
	group.GET("", func(c *gin.Context) {
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			raw, err := q.ListMissionRuns(c.Request.Context(), a.ProjectID)
			if err != nil {
				return nil, err
			}
			return decodeSnapshotRows(raw)
		})
	})
	group.GET("/:runId", func(c *gin.Context) {
		id, err := missionRunID(c)
		if err != nil {
			s.failure(c, 404, "TASK_RUN_NOT_FOUND")
			return
		}
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			raw, err := q.GetMissionWorkbenchRun(c.Request.Context(), sqlcgen.GetMissionWorkbenchRunParams{ProjectID: a.ProjectID, ID: id})
			if err != nil {
				return nil, err
			}
			runs, err := decodeSnapshotRows([]json.RawMessage{raw})
			if err != nil {
				return nil, err
			}
			rawSteps, err := q.GetMissionWorkbenchSteps(c.Request.Context(), sqlcgen.GetMissionWorkbenchStepsParams{ProjectID: a.ProjectID, TaskRunID: id})
			if err != nil {
				return nil, err
			}
			steps, err := decodeSnapshotRows(rawSteps)
			if err != nil {
				return nil, err
			}
			rawAudit, err := q.GetMissionWorkbenchAudit(c.Request.Context(), sqlcgen.GetMissionWorkbenchAuditParams{ProjectID: a.ProjectID, ID: id})
			if err != nil {
				return nil, err
			}
			audit, err := decodeSnapshotRows(rawAudit)
			if err != nil {
				return nil, err
			}
			status, _ := runs[0]["status"].(string)
			var deviceID int32
			var remoteID string
			err = s.db.QueryRowContext(c.Request.Context(), `select job.device_id,resource.remote_id
			 from connector_jobs job
			 join connector_remote_resources resource on resource.id=job.remote_result_resource_id and resource.project_id=job.project_id
			 where job.project_id=$1 and job.job_type='flight' and (job.task_run_id=$2 or job.business_run_id=$2)
			 and job.action_kind='flight-task-create' and job.accepted_at is not null
			 order by job.accepted_at desc limit 1`, a.ProjectID, id).Scan(&deviceID, &remoteID)
			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}
			if err == nil {
				runs[0]["realtimeFlight"] = gin.H{"deviceId": deviceID, "taskUuid": remoteID}
			}
			return gin.H{"run": runs[0], "steps": steps, "audit": audit, "actions": availableMissionActions(status, effectivePermissions(a.Role, a.Permissions))}, nil
		})
	})
}
