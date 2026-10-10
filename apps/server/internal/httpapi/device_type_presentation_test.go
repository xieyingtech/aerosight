package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDeviceTypeIconValidation(t *testing.T) {
	for _, icon := range []string{"cpu", "drone", "warehouse", "camera", "scan-eye"} {
		if !validDeviceTypeIcon(icon) {
			t.Fatalf("rejected %q", icon)
		}
	}
	for _, icon := range []string{"", "../camera", "https://example.com/icon", "Camera", "camera<script>", strings.Repeat("a", 81)} {
		if validDeviceTypeIcon(icon) {
			t.Fatalf("accepted %q", icon)
		}
	}
}

func TestDeviceTypeIconAPI(t *testing.T) {
	f := newAPIFixture(t)
	res := f.request(t, "GET", "/api/admin/device-types", "")
	var types []struct {
		ID   string `json:"id"`
		Icon string `json:"icon"`
	}
	err := json.NewDecoder(res.Body).Decode(&types)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 || len(types) == 0 {
		t.Fatalf("list %d %v", res.StatusCode, err)
	}
	path := "/api/admin/device-types/" + types[0].ID
	for _, tc := range []struct {
		body   string
		status int
	}{{`{"icon":"camera"}`, 200}, {`{"icon":"../camera"}`, 400}} {
		res = f.request(t, "PATCH", path, tc.body)
		res.Body.Close()
		if res.StatusCode != tc.status {
			t.Fatalf("patch %d want %d", res.StatusCode, tc.status)
		}
	}
	var stored string
	if err = f.db.QueryRow("select icon from device_types where id=$1", types[0].ID).Scan(&stored); err != nil || stored != "camera" {
		t.Fatalf("stored %q %v", stored, err)
	}
	_, projectID := f.project(t)
	if _, err = f.db.Exec("insert into devices(project_id,name,type,device_type_id) values($1,'typed device','legacy',$2)", projectID, types[0].ID); err != nil {
		t.Fatal(err)
	}
	devices, err := f.server.queries.SnapshotDevices(context.Background(), int32(projectID))
	if err != nil || len(devices) != 1 {
		t.Fatalf("snapshot devices %d %v", len(devices), err)
	}
	var projected map[string]any
	if err = json.Unmarshal(devices[0], &projected); err != nil || projected["typeIcon"] != "camera" || projected["deviceTypeId"] != types[0].ID {
		t.Fatalf("type projection %v %v", projected, err)
	}
	if _, err = f.db.Exec("update users set role='user' where email='admin@example.com'"); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "PATCH"} {
		target := "/api/admin/device-types"
		if method == "PATCH" {
			target = path
		}
		res = f.request(t, method, target, `{"icon":"drone"}`)
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatal(fmt.Sprintf("%s authorization %d", method, res.StatusCode))
		}
	}
}

func TestDevicePlatformCatalog(t *testing.T) {
	f := newAPIFixture(t)
	_, err := f.db.Exec(`INSERT INTO driver_definitions(driver_key,version,display_name,manifest_json) VALUES('catalog.test','1.0.0','Catalog test','{"capabilities":{"state.read":{"kind":"read","risk":"low","outputSchema":{"type":"object","properties":{"testKey":{"type":"string"}}}}},"streams":[{"channelKey":"test.channel","capabilityCode":"state.read","dataType":"telemetry","unit":"m"}]}')`)
	if err != nil {
		t.Fatal(err)
	}
	res := f.request(t, "GET", "/api/admin/device-types/catalog", "")
	var catalog struct{ Drivers, Capabilities, Streams, Actions []map[string]any }
	err = json.NewDecoder(res.Body).Decode(&catalog)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 || len(catalog.Drivers) == 0 || len(catalog.Actions) == 0 {
		t.Fatal("catalog", res.StatusCode, err)
	}
	found, duplicates := false, 0
	for _, cap := range catalog.Capabilities {
		if cap["code"] == "state.read" {
			duplicates++
		}
		if cap["driverKey"] == "catalog.test" {
			encoded, _ := json.Marshal(cap)
			found = cap["code"] == "state.read" && cap["driverVersion"] == "1.0.0" && strings.Contains(string(encoded), "testKey")
		}
	}
	if !found || duplicates < 2 {
		t.Fatal("historical schema/source or duplicates lost", catalog.Capabilities)
	}
	found = false
	for _, stream := range catalog.Streams {
		if stream["channelKey"] == "test.channel" {
			found = stream["capabilityCode"] == "state.read" && stream["unit"] == "m" && stream["driverKey"] == "catalog.test"
		}
	}
	if !found {
		t.Fatal("stream relationships missing")
	}
	found = false
	for _, action := range catalog.Actions {
		if action["key"] == "mission.create" && action["capabilityCode"] == "mission.execute" {
			found = true
		}
	}
	if !found {
		t.Fatal("actual action catalog missing")
	}
	res = f.request(t, "GET", "/api/admin/device-types", "")
	var types []map[string]any
	err = json.NewDecoder(res.Body).Decode(&types)
	res.Body.Close()
	if err != nil || len(types) == 0 || types[0]["driverKey"] == nil || types[0]["capabilityProfile"] == nil || types[0]["category"] == nil {
		t.Fatal("type relationships missing", types, err)
	}
	if _, err = f.db.Exec(`UPDATE users SET role='user' WHERE email='admin@example.com'`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/admin/device-types", "/api/admin/device-types/catalog"} {
		res = f.request(t, "GET", path, "")
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatal("non-admin catalog access", path, res.StatusCode)
		}
	}
}
