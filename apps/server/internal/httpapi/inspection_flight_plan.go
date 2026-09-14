package httpapi

import (
	"aerosight/server/internal/flighthub"
	"aerosight/server/internal/taskdefinition"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
)

// Catalogue preview has no dispatch path. The frozen version still needs a
// fresh remote comparison, preflight and authorization before any flight.
func (s *Server) previewInspectionFlightPlan(c *gin.Context) {
	pid, err := projectID(c)
	cid, cerr := strconv.ParseInt(c.Query("connectorId"), 10, 64)
	wid, werr := strconv.ParseInt(c.Query("waylineResourceId"), 10, 64)
	did, derr := strconv.ParseInt(c.Query("deviceId"), 10, 32)
	if err != nil || cerr != nil || werr != nil || derr != nil || cid <= 0 || wid <= 0 || did <= 0 {
		s.failure(c, 400, "INSPECTION_FLIGHT_PLAN_INPUT_INVALID")
		return
	}
	ctx := c.Request.Context()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		s.failure(c, 500, "INSPECTION_FLIGHT_PLAN_READ_FAILED")
		return
	}
	defer tx.Rollback()
	access, err := s.projectAccess(ctx, s.queries.WithTx(tx), currentUser(c).ID, pid, "project:view")
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	var raw []byte
	var remoteID, remoteVersion string
	err = tx.QueryRowContext(ctx, `select r.remote_id,coalesce(r.remote_version,''),r.summary_json
 from connector_remote_resources r join device_adapters a on a.id=r.connector_instance_id and a.project_id=r.project_id and a.team_id=r.team_id
 where r.project_id=$1 and r.team_id=$2 and r.connector_instance_id=$3 and r.id=$4 and r.resource_kind='wayline' and r.status='active'
 and a.adapter_type='dji-flighthub2' and a.status in('connecting','connected','degraded')
 and exists(select 1 from device_external_identities i join devices d on d.id=i.device_id and d.project_id=i.project_id where i.project_id=r.project_id and i.adapter_id=a.id and i.device_id=$5 and i.team_id=r.team_id and i.discovery_status='managed')`, pid, access.TeamID, cid, wid, did).Scan(&remoteID, &remoteVersion, &raw)
	if err == sql.ErrNoRows {
		s.failure(c, 404, "INSPECTION_FLIGHT_PLAN_SCOPE_INVALID")
		return
	}
	if err != nil {
		s.failure(c, 500, "INSPECTION_FLIGHT_PLAN_READ_FAILED")
		return
	}
	var summary struct {
		Name      string `json:"name"`
		UpdatedAt int64  `json:"updatedAt"`
		SizeBytes int64  `json:"sizeBytes"`
	}
	if json.Unmarshal(raw, &summary) != nil {
		s.failure(c, 409, "INSPECTION_WAYLINE_VERSION_UNVERIFIABLE")
		return
	}
	version, err := flighthub.FreezeInspectionWayline(flighthub.WaylineSummary{ID: remoteID, UpdatedAt: summary.UpdatedAt, SizeBytes: summary.SizeBytes})
	if err != nil || version.RemoteVersion != remoteVersion {
		s.failure(c, 409, "INSPECTION_WAYLINE_VERSION_UNVERIFIABLE")
		return
	}
	if err = tx.Commit(); err != nil {
		s.failure(c, 500, "INSPECTION_FLIGHT_PLAN_READ_FAILED")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, gin.H{"projectId": pid, "connectorId": cid, "waylineResourceId": wid, "deviceId": did, "name": summary.Name, "waylineVersion": version, "source": "dji.flighthub2", "verification": "catalogue-only", "schedulerOwner": "aerosight", "taskType": "immediate"})
}

func (s *Server) inspectionFlightPlanOptions(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	access, err := s.projectAccess(c.Request.Context(), s.queries, currentUser(c).ID, pid, "project:view")
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	var raw []byte
	err = s.db.QueryRowContext(c.Request.Context(), `select jsonb_build_object('waylines',coalesce((select jsonb_agg(row) from (
 select r.id::text as id,r.connector_instance_id::text as "connectorId",coalesce(r.summary_json->>'name',r.remote_id) as name
 from connector_remote_resources r join device_adapters a on a.id=r.connector_instance_id and a.project_id=r.project_id and a.team_id=r.team_id
 where r.project_id=$1 and r.team_id=$2 and r.resource_kind='wayline' and r.status='active' and a.adapter_type='dji-flighthub2' and a.status in('connecting','connected','degraded') order by r.id limit 200) row),'[]'::jsonb),
 'devices',coalesce((select jsonb_agg(row) from (
 select distinct d.id,d.name,i.adapter_id::text as "connectorId" from device_external_identities i join devices d on d.id=i.device_id and d.project_id=i.project_id
 join device_adapters a on a.id=i.adapter_id and a.project_id=i.project_id and a.team_id=i.team_id
 where i.project_id=$1 and i.team_id=$2 and i.discovery_status='managed' and a.adapter_type='dji-flighthub2' and a.status in('connecting','connected','degraded') order by d.id limit 200) row),'[]'::jsonb),'limit',200)`, pid, access.TeamID).Scan(&raw)
	if err != nil {
		s.failure(c, 500, "INSPECTION_FLIGHT_PLAN_READ_FAILED")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, json.RawMessage(raw))
}

// Validation uses the canonical contract even if an author supplies a weaker
// inputSchema. A valid catalogue selection still does not enable flight runtime.
func validateInspectionFlightSelection(ctx context.Context, tx *sql.Tx, pid int32, input map[string]any) error {
	if _, err := taskdefinition.MergeInputs(taskJSON(inspectionFlightInputSchema()), nil, input); err != nil {
		return errors.New("TASK_FLIGHT_SELECTION_INVALID")
	}
	var frozen flighthub.InspectionWaylineVersion
	if json.Unmarshal(taskJSON(input["waylineVersion"]), &frozen) != nil {
		return errors.New("TASK_FLIGHT_SELECTION_INVALID")
	}
	var remoteID, remoteVersion string
	var raw []byte
	err := tx.QueryRowContext(ctx, `select r.remote_id,coalesce(r.remote_version,''),r.summary_json
 from connector_remote_resources r join device_adapters a on a.id=r.connector_instance_id and a.project_id=r.project_id and a.team_id=r.team_id
 where r.project_id=$1 and r.connector_instance_id=$2 and r.id=$3 and r.resource_kind='wayline' and r.status='active'
 and a.adapter_type='dji-flighthub2' and a.status in('connecting','connected','degraded')
 and exists(select 1 from device_external_identities i join devices d on d.id=i.device_id and d.project_id=i.project_id
 where i.project_id=r.project_id and i.team_id=r.team_id and i.adapter_id=a.id and i.device_id=$4 and i.discovery_status='managed')`, pid, input["connectorId"], input["waylineResourceId"], input["deviceId"]).Scan(&remoteID, &remoteVersion, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("TASK_RESOURCE_SCOPE_INVALID")
	}
	if err != nil {
		return err
	}
	var summary struct {
		UpdatedAt int64 `json:"updatedAt"`
		SizeBytes int64 `json:"sizeBytes"`
	}
	if json.Unmarshal(raw, &summary) != nil || remoteVersion != frozen.RemoteVersion {
		return errors.New("TASK_WAYLINE_VERSION_CHANGED")
	}
	if err := flighthub.VerifyInspectionWayline(frozen, flighthub.WaylineSummary{ID: remoteID, UpdatedAt: summary.UpdatedAt, SizeBytes: summary.SizeBytes}); err != nil {
		return errors.New("TASK_WAYLINE_VERSION_INVALID")
	}
	return nil
}
