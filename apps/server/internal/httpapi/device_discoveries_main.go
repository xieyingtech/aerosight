package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

func discoveryFailure(c *gin.Context, err error) {
	code := err.Error()
	status := 400
	switch code {
	case "PROJECT_ACCESS_DENIED":
		status = 403
	case "CONNECTOR_DISABLED":
		status = 409
	case "DISCOVERY_ACTION_INVALID", "DEVICE_IDENTITY_NOT_FOUND", "MANAGED_IDENTITY_IMMUTABLE", "INVALID_DEVICE_BINDING", "DEVICE_IDENTITY_NOT_CONFIRMABLE", "DEVICE_TYPE_NOT_AVAILABLE", "TARGET_DEVICE_NOT_FOUND":
	default:
		code = "DEVICE_BINDING_FAILED"
	}
	c.JSON(status, gin.H{"error": code})
}
func (s *Server) discoveryCatalog(c *gin.Context) {
	s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
		ctx := c.Request.Context()
		raw, e := q.DiscoveryList(ctx, a.ProjectID)
		discoveries, e := decodeFHRows(raw, e)
		if e != nil {
			return nil, e
		}
		raw, e = q.DiscoveryTypes(ctx)
		types, e := decodeFHRows(raw, e)
		if e != nil {
			return nil, e
		}
		raw, e = q.DiscoveryConnectors(ctx, a.ProjectID)
		connectors, e := decodeFHRows(raw, e)
		if e != nil {
			return nil, e
		}
		return gin.H{"discoveries": discoveries, "deviceTypes": types, "connectors": connectors, "canManage": a.Role == "owner" || a.Role == "admin"}, nil
	})
}
func (s *Server) updateDiscovery(c *gin.Context) {
	pid, e := projectID(c)
	if e != nil {
		discoveryFailure(c, errors.New("DEVICE_IDENTITY_NOT_FOUND"))
		return
	}
	iid, e := strconv.ParseInt(c.Param("identityId"), 10, 64)
	if e != nil || iid <= 0 {
		discoveryFailure(c, errors.New("DEVICE_IDENTITY_NOT_FOUND"))
		return
	}
	var body map[string]any
	if strictJSON(c, &body) != nil || body["action"] != "ignore" && body["action"] != "review" && body["action"] != "rematch" {
		discoveryFailure(c, errors.New("DISCOVERY_ACTION_INVALID"))
		return
	}
	action := fhString(body["action"])
	ctx, uid := c.Request.Context(), currentUser(c).ID
	a, e := s.projectAccess(ctx, s.queries, uid, pid, "device:configure")
	if e != nil || a.Role != "owner" && a.Role != "admin" {
		discoveryFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "device_identity." + action, ResourceType: "device_external_identity", ResourceID: strconv.FormatInt(iid, 10), Input: gin.H{"action": action}, PolicyResult: map[string]any{"permission": "device:configure", "role": a.Role}}
	result, e := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
		q := w.Queries
		raw, e := q.DiscoveryLockStatus(ctx, sqlcgen.DiscoveryLockStatusParams{P1: pid, P2: iid})
		row, e := fhFirstRow(raw, e, "DEVICE_IDENTITY_NOT_FOUND")
		if e != nil {
			return nil, e
		}
		if row["deviceId"] != nil || row["status"] == "managed" {
			return nil, errors.New("MANAGED_IDENTITY_IMMUTABLE")
		}
		if action == "ignore" {
			e = q.DiscoveryIgnore(ctx, sqlcgen.DiscoveryIgnoreParams{P1: pid, P2: iid})
			return gin.H{"id": iid, "status": "ignored"}, e
		}
		raw, e = q.DiscoveryMatch(ctx, sqlcgen.DiscoveryMatchParams{P1: pid, P2: iid})
		matches, e := decodeFHRows(raw, e)
		if e != nil {
			return nil, e
		}
		var typeID sql.NullInt64
		var typeKey, confidence any
		if len(matches) > 0 {
			id, e := strconv.ParseInt(fhString(matches[0]["id"]), 10, 64)
			if e != nil {
				return nil, e
			}
			typeID = sql.NullInt64{Int64: id, Valid: true}
			typeKey = matches[0]["typeKey"]
			confidence = 1
		}
		status := "discovered"
		if row["status"] == "conflicted" {
			status = "conflicted"
		}
		e = q.DiscoveryRematch(ctx, sqlcgen.DiscoveryRematchParams{P1: pid, P2: iid, P3: typeID, P4: status})
		return gin.H{"id": iid, "status": status, "typeKey": typeKey, "confidence": confidence}, e
	})
	if e != nil {
		discoveryFailure(c, e)
		return
	}
	c.JSON(200, result)
}
func (s *Server) bindDiscoveryMain(c *gin.Context, pid int32, iid int64, body map[string]any) {
	name, key := strings.TrimSpace(fhString(body["name"])), strings.TrimSpace(fhString(body["deviceTypeKey"]))
	if name == "" || utf16Length(name) > 100 || key == "" {
		discoveryFailure(c, errors.New("INVALID_DEVICE_BINDING"))
		return
	}
	target := int64(0)
	if v, present := body["targetDeviceId"]; present && v != nil && v != float64(0) && v != "" {
		var ok bool
		if text, isString := v.(string); isString {
			n, e := strconv.ParseFloat(text, 64)
			if e != nil {
				discoveryFailure(c, errors.New("INVALID_DEVICE_BINDING"))
				return
			}
			v = n
		}
		target, ok = fhSafePositive(v)
		if !ok || target > 2147483647 {
			discoveryFailure(c, errors.New("INVALID_DEVICE_BINDING"))
			return
		}
	}
	ctx, uid := c.Request.Context(), currentUser(c).ID
	a, e := s.projectAccess(ctx, s.queries, uid, pid, "device:configure")
	if e != nil || a.Role != "owner" && a.Role != "admin" {
		discoveryFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	action := "device_identity.bind"
	var targetInput any
	if target > 0 {
		action = "device_identity.migrate"
		targetInput = target
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: action, ResourceType: "device_external_identity", ResourceID: strconv.FormatInt(iid, 10), Input: gin.H{"name": name, "deviceTypeKey": key, "targetDeviceId": targetInput}, PolicyResult: map[string]any{"permission": "device:configure", "role": a.Role}}
	result, e := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
		q := w.Queries
		raw, e := q.DiscoveryLockBinding(ctx, sqlcgen.DiscoveryLockBindingParams{P1: pid, P2: iid})
		identity, e := fhFirstRow(raw, e, "DEVICE_IDENTITY_NOT_FOUND")
		if e != nil {
			return nil, e
		}
		if identity["connectorStatus"] == "disabled" {
			return nil, errors.New("CONNECTOR_DISABLED")
		}
		if identity["deviceId"] != nil {
			return gin.H{"deviceId": identity["deviceId"], "replayed": true}, nil
		}
		if identity["status"] != "discovered" {
			return nil, errors.New("DEVICE_IDENTITY_NOT_CONFIRMABLE")
		}
		raw, e = q.DiscoveryType(ctx, key)
		typ, e := fhFirstRow(raw, e, "DEVICE_TYPE_NOT_AVAILABLE")
		if e != nil {
			return nil, e
		}
		typeID, e := strconv.ParseInt(fhString(typ["id"]), 10, 64)
		if e != nil {
			return nil, e
		}
		cid, e := strconv.ParseInt(fhString(identity["adapterId"]), 10, 64)
		if e != nil {
			return nil, e
		}
		adapter := sql.NullInt64{Int64: cid, Valid: true}
		did := int32(target)
		if target > 0 {
			raw, e = q.DiscoveryLockTarget(ctx, sqlcgen.DiscoveryLockTargetParams{P1: pid, P2: did})
			if e != nil {
				return nil, e
			}
			if len(raw) == 0 {
				return nil, errors.New("TARGET_DEVICE_NOT_FOUND")
			}
			if e = q.DiscoveryStandby(ctx, sqlcgen.DiscoveryStandbyParams{P1: pid, P2: did}); e != nil {
				return nil, e
			}
		} else {
			metadata, _ := json.Marshal(gin.H{"identityId": iid, "onboarding": "review"})
			did, e = q.DiscoveryCreate(ctx, sqlcgen.DiscoveryCreateParams{P1: pid, P2: adapter, P3: typeID, P4: name, P5: fhString(typ["category"]), P6: metadata})
			if e != nil {
				return nil, e
			}
		}
		if e = q.DiscoveryUpdateAdapter(ctx, sqlcgen.DiscoveryUpdateAdapterParams{P1: pid, P2: did, P3: adapter}); e != nil {
			return nil, e
		}
		if e = q.DiscoveryBind(ctx, sqlcgen.DiscoveryBindParams{P1: pid, P2: iid, P3: sql.NullInt32{Int32: did, Valid: true}, P4: sql.NullInt64{Int64: typeID, Valid: true}}); e != nil {
			return nil, e
		}
		role := "direct"
		parent := fhString(identity["parentExternalId"])
		if parent != "" {
			role = "inherited"
		} else if typ["category"] == "dock" || typ["category"] == "gateway" {
			role = "gateway"
		}
		metadata := json.RawMessage(`{"source":"review-onboarding"}`)
		if e = q.DiscoveryRoute(ctx, sqlcgen.DiscoveryRouteParams{P1: pid, P2: a.TeamID, P3: did, P4: cid, P5: iid, P6: role, P7: metadata}); e != nil {
			return nil, e
		}
		if e = q.DiscoveryCapabilities(ctx, sqlcgen.DiscoveryCapabilitiesParams{P1: did, P2: pid, P3: adapter, P4: typeID}); e != nil {
			return nil, e
		}
		if parent != "" {
			if e = q.DiscoveryRelationship(ctx, sqlcgen.DiscoveryRelationshipParams{P1: pid, P2: a.TeamID, P3: did, P4: cid, P5: parent, P6: metadata}); e != nil {
				return nil, e
			}
		}
		return gin.H{"deviceId": did, "replayed": false, "migrated": target > 0}, nil
	})
	if e != nil {
		discoveryFailure(c, e)
		return
	}
	c.JSON(200, result)
}
