package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

func (s *Server) bindDiscoveredDevice(c *gin.Context) {
	fail := func() { s.failure(c, 400, "Invalid or unauthorized device binding") }
	pid, err := projectID(c)
	if err != nil {
		fail()
		return
	}
	identityID, err := strconv.ParseInt(c.Param("identityId"), 10, 64)
	if err != nil || identityID <= 0 {
		fail()
		return
	}
	var body map[string]any
	if strictJSON(c, &body) != nil {
		fail()
		return
	}
	name, nameOK := body["name"].(string)
	kind, kindOK := body["deviceType"].(string)
	name, kind = strings.TrimSpace(name), strings.TrimSpace(kind)
	if !nameOK || !kindOK || name == "" {
		fail()
		return
	}
	switch kind {
	case "drone", "dock", "ground_robot", "fixed_sensor", "other":
	default:
		fail()
		return
	}
	uid := currentUser(c).ID
	access, err := s.projectAccess(c.Request.Context(), s.queries, uid, pid, "device:configure")
	if err != nil || (access.Role != "owner" && access.Role != "admin") {
		fail()
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "device_identity.bind", ResourceType: "device_external_identity", ResourceID: strconv.FormatInt(identityID, 10), Input: gin.H{"name": name, "deviceType": kind}, PolicyResult: map[string]any{"permission": "device:configure", "role": access.Role}}
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
		return bindDiscoveredDevice(c.Request.Context(), w.Queries, pid, identityID, name, kind)
	})
	if err != nil {
		fail()
		return
	}
	c.JSON(200, result)
}

func bindDiscoveredDevice(ctx context.Context, q *sqlcgen.Queries, pid int32, identityID int64, name, kind string) (gin.H, error) {
	identity, err := q.LockDiscoveredDevice(ctx, sqlcgen.LockDiscoveredDeviceParams{ProjectID: pid, ID: identityID})
	if err != nil {
		return nil, err
	}
	if identity.DeviceID.Valid {
		return gin.H{"deviceId": identity.DeviceID.Int32, "replayed": true}, nil
	}
	var metadata map[string]any
	if err = json.Unmarshal(identity.IdentityJson, &metadata); err != nil {
		return nil, err
	}
	typeKey, ok := metadata["deviceTypeKey"].(string)
	if !ok {
		typeKey = "legacy.device"
	}
	adapterID := sql.NullInt64{Int64: identity.AdapterID, Valid: true}
	deviceID, err := q.InsertDiscoveredDevice(ctx, sqlcgen.InsertDiscoveredDeviceParams{ProjectID: pid, AdapterID: adapterID, Name: name, DeviceType: kind, IdentityID: identityID, TypeKey: typeKey})
	if err != nil {
		return nil, err
	}
	if err = q.BindDiscoveredDevice(ctx, sqlcgen.BindDiscoveredDeviceParams{ProjectID: pid, ID: identityID, DeviceID: sql.NullInt32{Int32: deviceID, Valid: true}}); err != nil {
		return nil, err
	}
	capabilities, _ := metadata["capabilities"].([]any)
	for _, value := range capabilities {
		capability, ok := value.(string)
		if !ok {
			continue
		}
		if err = q.DeclareDiscoveredCapability(ctx, sqlcgen.DeclareDiscoveredCapabilityParams{DeviceID: deviceID, ProjectID: pid, CapabilityCode: capability, DeclaredByAdapterID: adapterID}); err != nil {
			return nil, err
		}
	}
	return gin.H{"deviceId": deviceID, "replayed": false}, nil
}
