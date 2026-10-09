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
