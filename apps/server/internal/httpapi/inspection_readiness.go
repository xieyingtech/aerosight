package httpapi

import (
	"database/sql"
	"encoding/json"
	"github.com/gin-gonic/gin"
)

// Catalogue checks expose no credentials and perform no remote actions. They
// describe available configuration, not a successful flight or provider call.
func (s *Server) inspectionReadiness(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	ctx := c.Request.Context()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		s.failure(c, 500, "INSPECTION_READINESS_FAILED")
		return
	}
	defer tx.Rollback()
	access, err := s.projectAccess(ctx, s.queries.WithTx(tx), currentUser(c).ID, pid, "project:view")
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	var raw []byte
	err = tx.QueryRowContext(ctx, `select jsonb_build_object(
 'availableImages',(select count(*) from assets where project_id=$1 and team_id=$2 and kind='image' and status='available' and deleted_at is null),
 'flightHubConnectors',(select count(*) from device_adapters where project_id=$1 and team_id=$2 and adapter_type='dji-flighthub2' and status in('connecting','connected','degraded')),
 'completedFlights',(select count(*) from connector_remote_resources r join task_runs t on t.id::text=r.canonical_target_id and t.project_id=r.project_id and t.team_id=r.team_id where r.project_id=$1 and r.team_id=$2 and r.resource_kind='flight-task' and r.status='active' and r.canonical_target_type='task_run' and t.status='succeeded'),
 'detectionVersions',(select count(*) from algorithm_definition_versions v join algorithm_definitions d on d.id=v.algorithm_definition_id and d.project_id=v.project_id join algorithm_providers p on p.id=d.provider_id and p.project_id=d.project_id where v.project_id=$1 and v.team_id=$2 and v.status='published' and d.capability_code='detection' and p.status='active' and p.provider_type='http-json'),
 'copilotConfigured',exists(select 1 from agents where project_id=$1 and status='active' and config_json->>'kind'='copilot'),
 'modelConfigured',exists(select 1 from ai_providers where enabled and is_default and provider_type='openai'),
 'verification','catalogue-only')`, pid, access.TeamID).Scan(&raw)
	if err != nil {
		s.failure(c, 500, "INSPECTION_READINESS_FAILED")
		return
	}
	if err = tx.Commit(); err != nil {
		s.failure(c, 500, "INSPECTION_READINESS_FAILED")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, json.RawMessage(raw))
}
