package flighthub

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestDeviceAdminMutationsFailWithoutAutomaticRetry(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name, action, serial string
		request              json.RawMessage
	}{
		{"rtk", "rtk.calibrate", "DOCK_REDACTED", json.RawMessage(`{"host":"ntrip.invalid","port":8002,"account":"a","password":"p","mountPoint":"m"}`)},
		{"relay", "relay.pair", "DOCK_REDACTED", json.RawMessage(`{"pairEnable":true,"pairType":"drone"}`)},
		{"migration", "active_project.update", "DOCK_REDACTED", json.RawMessage(`{"activeProjectUuid":"PROJECT_TARGET_REDACTED"}`)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			calls := 0
			client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return response(http.StatusServiceUnavailable, []byte(`{"code":500000,"message":"redacted","data":null}`), nil), nil
			}), func(config *Config) { config.MaxRetries = 3 })
			_, _, err := client.ExecuteDeviceAdminControl(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", testCase.action, testCase.serial, testCase.request)
			if err == nil || calls != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestSNDecryptFailureIsNeverRetried(t *testing.T) {
	t.Parallel()
	calls := 0
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return response(http.StatusServiceUnavailable, []byte(`{"code":500000,"message":"redacted","data":null}`), nil), nil
	}), func(config *Config) { config.MaxRetries = 3 })
	_, err := client.DecryptSNs(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", SNDecryptRequest{EncryptedSNs: []string{"ENCRYPTED_REDACTED"}})
	if err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestDeviceAdminCapabilitiesAreIndependentlyDefaultClosed(t *testing.T) {
	t.Parallel()
	wanted := map[string]string{"device.rtk.calibrate": FlightHubRTKCalibrateFeatureFlag, "device.relay.pair": FlightHubRelayPairFeatureFlag, "device.active-project.update": FlightHubDeviceMigrationFeatureFlag, "security.sn.decrypt": FlightHubSNDecryptFeatureFlag}
	for _, capability := range Capabilities() {
		if flag, ok := wanted[capability.Code]; ok {
			if capability.DefaultEnabled || capability.FeatureFlag != flag {
				t.Fatalf("capability=%#v", capability)
			}
			delete(wanted, capability.Code)
		}
	}
	if len(wanted) != 0 {
		t.Fatalf("missing capabilities %#v", wanted)
	}
}
