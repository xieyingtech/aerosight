package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"testing"
)

func TestRegionGeometry(t *testing.T) {
	for _, raw := range []string{`{"type":"Polygon","coordinates":[]}`, `{"type":"Point","coordinates":[113,22]}`, `{"type":"Polygon","coordinates":[[[113,22],[114,22],[114,23],[113,23]]]}`, `{"type":"Polygon","coordinates":[[[181,22],[114,22],[114,23],[181,22]]]}`} {
		if validRegionGeometry(json.RawMessage(raw)) {
			t.Fatalf("accepted invalid geometry %s", raw)
		}
	}
	if !validRegionGeometry(json.RawMessage(`{"type":"Polygon","coordinates":[[[113,22],[114,22],[114,23],[113,22]]]}`)) {
		t.Fatal("valid polygon rejected")
	}
}

func TestMapRegistration(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	root := fmt.Sprintf("/api/projects/%d", pid)
	region := `{"name":"巡检区域","registrationKey":"area-1","geometry":{"type":"Polygon","coordinates":[[[113.88,22.78],[113.89,22.78],[113.89,22.79],[113.88,22.78]]]}}`
	device := `{"name":"南入口监控","registrationKey":"camera-1","deviceTypeKey":"registered.fixed-camera","position":{"longitude":113.88,"latitude":22.78}}`
	for _, entry := range []struct{ path, body string }{{root + "/map-regions", region}, {root + "/devices/register", device}} {
		var first any
		for i := 0; i < 2; i++ {
			res := f.request(t, "POST", entry.path, entry.body)
			data := decodedResponse(t, res)
			if res.StatusCode != 201 {
				t.Fatalf("registration %d %+v", res.StatusCode, data)
			}
			if i == 0 {
				first = data["id"]
			} else if first != data["id"] {
				t.Fatal("duplicate registration")
			}
		}
	}
	res := f.request(t, "GET", root+"/snapshot", "")
	data := decodedResponse(t, res)
	if res.StatusCode != 200 || len(data["regions"].([]any)) != 1 || len(data["devices"].([]any)) != 1 {
		t.Fatalf("snapshot %+v", data)
	}
	d := data["devices"].([]any)[0].(map[string]any)
	if d["status"] != "offline" || d["positionSource"] != "manual-registration" || d["pose"].(map[string]any)["longitude"] != 113.88 || len(d["capabilities"].([]any)) != 0 {
		t.Fatalf("device %+v", d)
	}
	var observations int
	treeResponse := f.request(t, "GET", root+"/device-tree", "")
	if treeResponse.StatusCode == 200 {
		var tree []map[string]any
		if err := json.NewDecoder(treeResponse.Body).Decode(&tree); err != nil {
			t.Fatal(err)
		}
		treeResponse.Body.Close()
		if len(tree) != 1 || tree[0]["positionSource"] != "manual-registration" {
			t.Fatal("device tree registration missing")
		}
	} else {
		treeResponse.Body.Close()
		t.Fatalf("device tree %d", treeResponse.StatusCode)
	}
	if err := f.db.QueryRow("select count(*) from observations where project_id=$1", pid).Scan(&observations); err != nil || observations != 0 {
		t.Fatal("registration manufactured telemetry", err)
	}
	invalid := `{"name":"无效围栏","registrationKey":"area-bad","geometry":{"type":"Polygon","coordinates":[[[113,22],[114,23],[113,23],[114,22],[113,22]]]}}`
	bad := f.request(t, "POST", root+"/map-regions", invalid)
	bad.Body.Close()
	if bad.StatusCode != 400 {
		t.Fatal("self intersecting polygon accepted")
	}
	_, other := f.project(t)
	otherRes := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/snapshot", other), "")
	otherData := decodedResponse(t, otherRes)
	if len(otherData["regions"].([]any)) != 0 || len(otherData["devices"].([]any)) != 0 {
		t.Fatal("cross project data leaked")
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	denied := f.request(t, "POST", root+"/devices/register", device)
	denied.Body.Close()
	if denied.StatusCode != 403 {
		t.Fatalf("member registration %d", denied.StatusCode)
	}
}

func TestRegisteredPositionPreservesTelemetry(t *testing.T) {
	row := gin.H{"pose": gin.H{"longitude": 114.0}, "registeredPosition": map[string]any{"longitude": 113.0}}
	applyRegisteredPosition(row)
	if row["pose"].(gin.H)["longitude"] != 114.0 || row["registeredPosition"] != nil {
		t.Fatal("telemetry position overridden")
	}
}
