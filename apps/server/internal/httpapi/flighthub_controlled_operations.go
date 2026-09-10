package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
)

//go:embed flighthub_controlled_operations.json
var fhControlledDefinitions []byte

func (s *Server) fhControlledOperations(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store, max-age=0")
	fail := func() { c.JSON(404, gin.H{"error": "FLIGHTHUB_CONTROLLED_OPERATIONS_NOT_FOUND"}) }
	pid, e := projectID(c)
	if e != nil {
		fail()
		return
	}
	cid, e := connectorID(c)
	if e != nil {
		fail()
		return
	}
	ctx, uid := c.Request.Context(), currentUser(c).ID
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		fail()
		return
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	a, e := s.projectAccess(ctx, q, uid, pid, "project:view")
	if e != nil {
		fail()
		return
	}
	raw, e := q.FHControlledAccess(ctx, sqlcgen.FHControlledAccessParams{P1: uid, P2: pid, P3: cid})
	row, e := fhFirstRow(raw, e, "FLIGHTHUB_CONTROLLED_OPERATIONS_NOT_FOUND")
	if e != nil || row["connectorProjectId"] != float64(pid) || row["connectorTeamId"] != float64(a.TeamID) {
		fail()
		return
	}
	raw, e = q.FHControlledCapabilities(ctx, sqlcgen.FHControlledCapabilitiesParams{P1: pid, P2: cid, P3: sql.NullString{String: fhString(row["accountFingerprint"]), Valid: row["accountFingerprint"] != nil}})
	caps, e := decodeFHRows(raw, e)
	if e != nil {
		fail()
		return
	}
	verified := map[string]bool{}
	for _, r := range caps {
		verified[fhString(r["capabilityCode"])] = true
	}
	raw, e = q.FHControlledJobs(ctx, sqlcgen.FHControlledJobsParams{P1: pid, P2: cid})
	jobs, e := decodeFHRows(raw, e)
	if e != nil {
		fail()
		return
	}
	manifest := fhObject(row["manifest"])
	available := map[string]bool{}
	items, _ := manifest["capabilities"].([]any)
	for _, item := range items {
		m := fhObject(item)
		if m["kind"] == "action" {
			available[fhString(m["code"])] = true
		}
	}
	flags := fhObject(row["featureFlags"])
	var definitions []gin.H
	if json.Unmarshal(fhControlledDefinitions, &definitions) != nil {
		fail()
		return
	}
	actions := []gin.H{}
	for _, d := range definitions {
		code := fhString(d["capabilityCode"])
		if !available[code] {
			continue
		}
		permission := fhString(d["permission"])
		allowed := effectivePermissions(a.Role, a.Permissions)["mission:operate"]
		if permission == "organization:manage" {
			allowed = row["managementGranted"] == true
		} else if permission == "project:admin" {
			allowed = a.Role == "owner" || a.Role == "admin"
		}
		ready := row["connectorStatus"] == "connected"
		enabled := flags[fhString(d["featureFlag"])] == true
		missing := []string{}
		if !ready {
			missing = append(missing, "连接器未连接")
		}
		if !allowed {
			missing = append(missing, "缺少 "+permission)
		}
		if !enabled {
			missing = append(missing, "功能开关 "+fhString(d["featureFlag"])+" 未开启")
		}
		if !verified[code] {
			missing = append(missing, "缺少 field-write 现场验收")
		}
		d["href"] = fmt.Sprintf("/projects/%s?projectId=%d", fhString(d["href"]), pid)
		d["connectorReady"], d["permissionReady"], d["featureEnabled"], d["capabilityVerified"], d["available"], d["missing"] = ready, allowed, enabled, verified[code], len(missing) == 0, missing
		actions = append(actions, d)
	}
	if tx.Commit() != nil {
		fail()
		return
	}
	c.JSON(200, gin.H{"actions": actions, "jobs": jobs})
}
func decodeFHRows(raw []json.RawMessage, err error) ([]gin.H, error) {
	if err != nil {
		return nil, err
	}
	rows, e := decodeSnapshotRows(raw)
	if e != nil {
		return nil, errors.New("invalid workspace data")
	}
	return rows, nil
}

func fhObject(v any) map[string]any { m, _ := v.(map[string]any); return m }
