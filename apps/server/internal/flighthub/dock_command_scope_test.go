package flighthub

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestFlightCommandsRejectAircraftSNRouteBeforeUpstream(t *testing.T) {
	now := time.Now()
	for _, key := range []string{"return_home", "return_home_cancel", "flighttask_pause", "flighttask_recovery"} {
		p := discreteControlPolicies[key]
		command := loadedControlCommand{CommandKey: key, CapabilityCode: p.capabilityCode, RecordedConnectorCapabilityCode: p.connectorCapabilityCode, RecordedFeatureFlag: p.featureFlag, DeviceTypeKey: "dji.matrice3td", ConnectorStatus: "connected", FeatureEnabled: true, CapabilityVerified: true, DeviceOnline: true, StateFresh: true, ApprovalValid: true, SafetyPolicyCurrent: true, Deadline: now.Add(time.Minute), Priority: 100, Parameters: json.RawMessage(`{}`)}
		client := &governedControlClientFixture{}
		if _, err := invokeGovernedDeviceControl(context.Background(), client, "TOKEN_REDACTED", command, p, now); err == nil || client.calls != 0 {
			t.Fatalf("%s called aircraft route", key)
		}
		command.DeviceTypeKey = "dji.dock2"
		if gate := controlCommandGate(command, p, true, now); gate != "" {
			t.Fatalf("valid dock gated %s: %s", key, gate)
		}
	}
}
