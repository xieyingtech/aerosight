package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/device"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"testing"
)

func djiSetupBody() map[string]any {
	return map[string]any{"name": "DJI setup", "mode": "lan", "mqttEndpoint": "mqtt://gateway.test", "apiPublicBaseUrl": "http://gateway.test/api", "websocketPublicUrl": "ws://gateway.test/ws", "mediaIngestBaseUrl": "rtmp://gateway.test/live", "mediaPlaybackBaseUrl": "http://gateway.test/media", "tlsRequired": false, "mqttUsername": "pilot", "mqttPassword": "mqtt-secret", "appId": "application", "appKey": "app-secret", "appLicense": "license-secret", "mediaPublishUser": "publisher", "mediaPublishPassword": "publish-secret", "ntpServerHost": "ntp.test", "ntpServerPort": "123", "gatewaySerials": []string{" GW-1 "}}
}
func fixtureNetwork(f *apiFixture) {
	f.server.AttachDeviceCredentials("0123456789abcdef0123456789abcdef")
	f.server.networkResolver = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("192.168.1.2")}, nil
	}
	f.server.networkProbe = func(context.Context, device.NetworkEndpoint) error { return nil }
}
func TestDJISetupAndConnectionCheck(t *testing.T) {
	f := newAPIFixture(t)
	fixtureNetwork(f)
	team, pid := f.project(t)
	path := fmt.Sprintf("/api/projects/%d/device-adapters", pid)
	raw, _ := json.Marshal(djiSetupBody())
	res := f.request(t, "POST", path+"/dji-setup", string(raw))
	data := decodedResponse(t, res)
	if res.StatusCode != 201 || data["status"] != "connecting" || data["protocolVersion"] != "cloud-api-mqtt5" {
		t.Fatalf("setup %d %+v", res.StatusCode, data)
	}
	id := data["id"].(string)
	network := data["network"].(map[string]any)
	config := data["config"].(map[string]any)
	if network["status"] != "unverified" || len(config["topics"].([]any)) != 6 || config["gatewaySerials"].([]any)[0] != "GW-1" {
		t.Fatalf("setup config %+v", data)
	}
	summary := data["configurationSummary"].(map[string]any)
	if summary["mqtt_broker"].(map[string]any)["password"] != "[ENCRYPTED]" {
		t.Fatalf("summary %+v", summary)
	}
	var profileID int64
	var envelopeRaw []byte
	if err := f.db.QueryRow("select network_profile_id,credential_envelope_json from device_adapters where id=$1", id).Scan(&profileID, &envelopeRaw); err != nil {
		t.Fatal(err)
	}
	var envelope credentials.Envelope
	if err := json.Unmarshal(envelopeRaw, &envelope); err != nil {
		t.Fatal(err)
	}
	var secrets map[string]string
	if err := credentials.DecryptJSON(envelope, f.server.credentialSecret, credentials.AAD("device-adapter", id, pid), &secrets); err != nil {
		t.Fatal(err)
	}
	if secrets["mqttPassword"] != "mqtt-secret" || len(secrets) != 7 {
		t.Fatal("credential mismatch")
	}
	testPath := path + "/" + id + "/test"
	res = f.request(t, "POST", testPath, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || data["ok"] != true || data["deviceVerification"] != "pending" || data["serverVerification"] != "verified" || len(data["diagnostics"].([]any)) != 5 {
		t.Fatalf("test %d %+v", res.StatusCode, data)
	}
	var state string
	if err := f.db.QueryRow("select status from device_network_profiles where id=$1", profileID).Scan(&state); err != nil || state != "valid" {
		t.Fatalf("profile %s %v", state, err)
	}
	f.server.networkProbe = func(context.Context, device.NetworkEndpoint) error { return errors.New("private endpoint detail") }
	res = f.request(t, "POST", testPath, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || data["ok"] != false || data["status"] != "invalid" {
		t.Fatalf("failed probe %d %+v", res.StatusCode, data)
	}
	var health string
	if err := f.db.QueryRow("select last_health_json::text from device_adapters where id=$1", id).Scan(&health); err != nil || strings.Contains(health, "private endpoint") {
		t.Fatalf("health %s %v", health, err)
	}
	// A database failure after the profile update must roll back health, profile and audit together.
	if _, err := f.db.Exec(`create function reject_health() returns trigger language plpgsql as $$ begin raise exception 'private storage detail'; end $$;create trigger reject_health before update on device_adapters for each row execute function reject_health()`); err != nil {
		t.Fatal(err)
	}
	f.server.networkProbe = func(context.Context, device.NetworkEndpoint) error { return nil }
	res = f.request(t, "POST", testPath, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 400 || data["error"] != "DEVICE_ADAPTER_TEST_FAILED" {
		t.Fatalf("rollback %d %+v", res.StatusCode, data)
	}
	if err := f.db.QueryRow("select status from device_network_profiles where id=$1", profileID).Scan(&state); err != nil || state != "invalid" {
		t.Fatalf("rollback profile %s %v", state, err)
	}
	if _, err := f.db.Exec("drop trigger reject_health on device_adapters"); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	f.server.networkProbe = func(ctx context.Context, _ device.NetworkEndpoint) error {
		var err error
		once.Do(func() { _, err = f.db.ExecContext(ctx, "delete from team_members where team_id=$1", team) })
		return err
	}
	res = f.request(t, "POST", testPath, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 403 || data["error"] != "PROJECT_ACCESS_DENIED" {
		t.Fatalf("revoked during probe %d %+v", res.StatusCode, data)
	}
	var count int
	if err := f.db.QueryRow("select count(*) from audit_events where project_id=$1", pid).Scan(&count); err != nil || count != 3 {
		t.Fatalf("audits %d %v", count, err)
	}
}

func TestDJISetupValidationAndRollback(t *testing.T) {
	f := newAPIFixture(t)
	fixtureNetwork(f)
	_, pid := f.project(t)
	path := fmt.Sprintf("/api/projects/%d/device-adapters", pid)
	body := djiSetupBody()
	body["mode"] = "public"
	raw, _ := json.Marshal(body)
	res := f.request(t, "POST", path+"/dji-setup", string(raw))
	data := decodedResponse(t, res)
	if res.StatusCode != 400 || data["error"] != "NETWORK_PROFILE_INVALID" || len(data["issues"].([]any)) == 0 {
		t.Fatalf("policy %d %+v", res.StatusCode, data)
	}
	body = djiSetupBody()
	raw, _ = json.Marshal(body)
	f.server.AttachDeviceCredentials("")
	res = f.request(t, "POST", path+"/dji-setup", string(raw))
	data = decodedResponse(t, res)
	if res.StatusCode != 400 || data["error"] != "DJI_ADAPTER_SETUP_INVALID" {
		t.Fatalf("no key %d %+v", res.StatusCode, data)
	}
	for _, table := range []string{"device_network_profiles", "device_adapters", "audit_events"} {
		var count int
		if err := f.db.QueryRow("select count(*) from "+table+" where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rollback %s %d %v", table, count, err)
		}
	}
	for _, kind := range []string{"simulator", "dji"} {
		res = f.request(t, "POST", path, fmt.Sprintf(`{"name":"%s","adapterType":"%s"}`, kind, kind))
		data = decodedResponse(t, res)
		if res.StatusCode != 201 {
			t.Fatalf("adapter %d %+v", res.StatusCode, data)
		}
		res = f.request(t, "POST", path+"/"+data["id"].(string)+"/test", "")
		data = decodedResponse(t, res)
		expected := "NETWORK_PROFILE_REQUIRED"
		if kind == "simulator" {
			expected = "SIMULATOR_READY"
		}
		if res.StatusCode != 200 || data["code"] != expected {
			t.Fatalf("no profile %d %+v", res.StatusCode, data)
		}
	}
	res = f.request(t, "POST", path+"/999999/test", "")
	data = decodedResponse(t, res)
	if res.StatusCode != 400 || data["error"] != "DEVICE_ADAPTER_NOT_FOUND" {
		t.Fatalf("missing %d %+v", res.StatusCode, data)
	}
}
