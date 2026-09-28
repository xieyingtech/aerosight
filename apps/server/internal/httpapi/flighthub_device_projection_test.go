package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"github.com/gin-gonic/gin"
	"testing"
)

func TestFlightHubActionProjectionUsesConnectorCommands(t *testing.T) {
	for _, connector := range []string{"dji.flighthub2", "dji.cloud"} {
		device := gin.H{"id": float64(7), "status": "online", "connectorKey": connector, "typeKey": "dji.dock2", "rawCapabilities": []any{
			map[string]any{"code": "dock.debug.control", "availability": "available", "risk": "high"},
			map[string]any{"code": "flight.return_home", "availability": "available", "risk": "critical"},
			map[string]any{"code": "mission.execute", "availability": "available", "risk": "high"},
		}}
		projectCapabilities(device, "owner", nil)
		caps := device["capabilities"].([]gin.H)
		if connector == "dji.flighthub2" && len(caps[0]["actions"].([]gin.H)) != 0 {
			t.Fatal("Cloud-only dock commands exposed in FlightHub menu")
		}
		if connector == "dji.cloud" && len(caps[0]["actions"].([]gin.H)) != 11 {
			t.Fatal("Cloud API dock commands lost")
		}
		for _, action := range caps[0]["actions"].([]gin.H) {
			if action["enabled"] != (connector != "dji.flighthub2") {
				t.Fatalf("%s: %+v", connector, action)
			}
			if connector == "dji.flighthub2" && action["unavailableReason"] == nil {
				t.Fatal("unsupported action needs reason")
			}
		}
		if caps[1]["actions"].([]gin.H)[0]["enabled"] != true || caps[2]["actions"].([]gin.H)[0]["enabled"] != true {
			t.Fatal("supported command or task workflow disabled")
		}
	}
}

func TestFlightHubUnsupportedCommandFailsBeforeGovernanceOrDispatch(t *testing.T) {
	for _, input := range []struct{ key, capability, expected string }{
		{"cover.open", "dock.debug.control", "FLIGHTHUB_COMMAND_UNSUPPORTED"},
		{"return_home", "dock.debug.control", "FLIGHTHUB_COMMAND_POLICY_MISMATCH"},
	} {
		_, err := fhCommandSafety(context.Background(), nil, 1, 1, deviceCommandInput{Key: input.key, Capability: input.capability}, sqlcgen.LockDeviceCommandTargetRow{}, gin.H{})
		if err == nil || err.Error() != input.expected {
			t.Fatalf("%s: %v", input.key, err)
		}
	}
}
