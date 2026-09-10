package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"slices"
	"strconv"
	"time"
)

type fhDiscretePolicy struct {
	capability, connectorCapability, flag string
	types                                 []string
}

var fhDiscretePolicies = map[string]fhDiscretePolicy{
	"return_home": {"flight.return_home", "device.control", "device.control", nil}, "return_home_cancel": {"flight.return_home", "device.control", "device.control", nil}, "flighttask_pause": {"mission.execute", "device.control", "device.control", nil}, "flighttask_recovery": {"mission.execute", "device.control", "device.control", nil},
	"camera.change": {"camera.change", "device.camera.change", "flighthub.camera.change", []string{"dji.dock2", "dji.dock3"}}, "camera.change_lens": {"camera.lens.change", "device.lens.change", "flighthub.lens.change", []string{"dji.matrice3d", "dji.matrice3td", "dji.matrice4d", "dji.matrice4td"}},
}

func validFHCommandParameters(key string, m map[string]any) bool {
	identifier := func(v any) bool { return v != nil && fhPayloadIndex.MatchString(fmt.Sprint(v)) }
	if key == "camera.change" {
		for k := range m {
			if k != "cameraIndex" && k != "cameraPosition" {
				return false
			}
		}
		if !identifier(m["cameraIndex"]) {
			return false
		}
		if v, ok := m["cameraPosition"]; ok && !identifier(v) {
			return false
		}
		return true
	}
	if key == "camera.change_lens" {
		return len(m) == 2 && identifier(m["cameraIndex"]) && identifier(m["lensType"])
	}
	return len(m) == 0
}
func fhCommandSafety(ctx context.Context, q *sqlcgen.Queries, pid, team int32, input deviceCommandInput, target sqlcgen.LockDeviceCommandTargetRow, route gin.H) (gin.H, error) {
	p, ok := fhDiscretePolicies[input.Key]
	if !ok || p.capability != input.Capability {
		return nil, errors.New("FLIGHTHUB_COMMAND_POLICY_MISMATCH")
	}
	if !validFHCommandParameters(input.Key, input.Parameters) {
		return nil, errors.New("FLIGHTHUB_COMMAND_PARAMETERS_INVALID")
	}
	if p.types != nil && !slices.Contains(p.types, target.TypeKey) {
		return nil, errors.New("FLIGHTHUB_COMMAND_MODEL_UNSUPPORTED")
	}
	if target.Status != "online" {
		return nil, errors.New("FLIGHTHUB_COMMAND_DEVICE_OFFLINE")
	}
	if route["connectorStatus"] != "connected" {
		return nil, errors.New("FLIGHTHUB_CONNECTOR_NOT_CONNECTED")
	}
	cid, e := strconv.ParseInt(fhString(route["connectorInstanceId"]), 10, 64)
	if e != nil {
		return nil, e
	}
	approval := uuid.NullUUID{}
	if input.ApprovalRequestID != "" {
		id, e := uuid.Parse(input.ApprovalRequestID)
		if e != nil {
			return nil, e
		}
		approval = uuid.NullUUID{UUID: id, Valid: true}
	}
	raw, e := q.FHCommandGovernance(ctx, sqlcgen.FHCommandGovernanceParams{P1: pid, P2: input.DeviceID, P3: cid, P4: approval, P5: p.flag, P6: p.connectorCapability})
	row, e := fhFirstRow(raw, e, "FLIGHTHUB_COMMAND_SCOPE_DENIED")
	if e != nil {
		return nil, e
	}
	if row["featureEnabled"] != true {
		return nil, errors.New("FLIGHTHUB_COMMAND_FEATURE_DISABLED")
	}
	if row["capabilityFieldVerified"] != true {
		return nil, errors.New("FLIGHTHUB_COMMAND_NOT_FIELD_VERIFIED")
	}
	captured, e := time.Parse(time.RFC3339Nano, fhString(row["stateCapturedAt"]))
	now := time.Now()
	if e != nil || now.Sub(captured) > 30*time.Second || captured.After(now.Add(time.Second)) {
		return nil, errors.New("FLIGHTHUB_COMMAND_STATE_STALE")
	}
	if input.SafetyPolicyVersionID <= 0 || row["currentSafetyPolicyVersionId"] != strconv.FormatInt(input.SafetyPolicyVersionID, 10) {
		return nil, errors.New("FLIGHTHUB_COMMAND_SAFETY_POLICY_STALE")
	}
	if row["approvalProjectId"] != float64(pid) || row["approvalTeamId"] != float64(team) || row["approvalResourceType"] != "device" || row["approvalResourceId"] != strconv.Itoa(int(input.DeviceID)) || row["approvalAction"] != "flighthub.device."+input.Key || row["approvalStatus"] != "approved" || row["approvalUnexpired"] != true {
		return nil, errors.New("FLIGHTHUB_COMMAND_APPROVAL_REQUIRED")
	}
	return gin.H{"connectorKey": "dji.flighthub2", "connectorInstanceId": route["connectorInstanceId"], "connectorCapabilityCode": p.connectorCapability, "featureFlag": p.flag, "approvalRequestId": input.ApprovalRequestID, "safetyPolicyVersionId": input.SafetyPolicyVersionID, "stateFresh": true, "capabilityFieldVerified": true}, nil
}
