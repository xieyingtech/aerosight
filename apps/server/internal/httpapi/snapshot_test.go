package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSnapshotEmptyLayersScopeAndDiagnostics(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	if _, err := f.db.Exec("insert into device_adapters(project_id,team_id,name,adapter_type,status) values($1,$2,'broken adapter','simulator','failed')", pid, team); err != nil {
		t.Fatal(err)
	}
	res := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/snapshot", pid), "")
	defer res.Body.Close()
	var data map[string]any
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("snapshot %d %+v", res.StatusCode, data)
	}
	for _, key := range []string{"devices", "tracks", "activeTasks", "liveStreams", "realtimeChannels", "mediaPoints", "suspectedConstruction", "openAlerts", "openIssues", "regions"} {
		rows, ok := data[key].([]any)
		if !ok || len(rows) != 0 {
			t.Fatalf("empty layer %s: %+v", key, data[key])
		}
	}
	diagnostics := data["diagnostics"].([]any)
	if len(diagnostics) != 1 || diagnostics[0].(map[string]any)["deviceId"] != nil {
		t.Fatalf("nullable diagnostic: %+v", diagnostics)
	}
	project := data["project"].(map[string]any)
	if _, ok := project["role"]; ok {
		t.Fatal("internal membership exposed")
	}
	if data["consistency"] != "repeatable-read" || data["freshness"].(map[string]any)["latestCapturedAt"] != nil {
		t.Fatalf("snapshot metadata: %+v", data)
	}
	if _, err := f.db.Exec("delete from team_members where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	denied := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/snapshot", pid), "")
	denied.Body.Close()
	if denied.StatusCode != 404 {
		t.Fatalf("revoked scope: %d", denied.StatusCode)
	}
}

func TestCapabilityProjectionDenyAndOffline(t *testing.T) {
	device := gin.H{"id": float64(7), "deviceTypeId": "9", "status": "offline", "rawCapabilities": []any{map[string]any{"code": "dock.debug.control", "availability": "available", "reason": nil, "risk": "high"}}}
	projectCapabilities(device, "owner", nil)
	cap := device["capabilities"].([]gin.H)[0]
	actions := cap["actions"].([]gin.H)
	if cap["authorized"] != true || len(actions) != 11 || actions[0]["enabled"] != false || actions[0]["unavailableReason"] != "设备不在线，暂时无法执行" {
		t.Fatalf("offline projection: %+v", cap)
	}
	device["rawCapabilities"] = []any{map[string]any{"code": "dock.debug.control", "availability": "available", "reason": nil, "risk": "high"}}
	projectCapabilities(device, "owner", []gin.H{{"scopeType": "device", "deviceId": float64(7), "actionPattern": "dock.*", "effect": "deny"}})
	cap = device["capabilities"].([]gin.H)[0]
	if cap["authorized"] != false || len(cap["actions"].([]gin.H)) != 0 {
		t.Fatal("owner bypassed explicit deny")
	}
}

func TestSnapshotDependencyHealth(t *testing.T) {
	health, layers := snapshotHealth(json.RawMessage(`{"model_service":"disabled","device_adapter":"unavailable"}`))
	if health["status"] != "degraded" || health["ready"] != true || layers["liveStreams"] != "degraded" || health["capabilityAvailability"].(gin.H)["ai_generation"] != "disabled" {
		t.Fatalf("health: %+v %+v", health, layers)
	}
}

func TestSnapshotPostGISDevicePoseAndTrack(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	adapter, device := f.device(t, team, pid)
	for n := 0; n < 2; n++ {
		var observation int
		if err := f.db.QueryRow("insert into observations(project_id,team_id,adapter_id,device_id,observation_type,source_event_id,captured_at,received_at) values($1,$2,$3,$4,'pose',$5,now()+$6*interval '1 second',now()) returning id", pid, team, adapter, device, fmt.Sprintf("pose-%d", n), n).Scan(&observation); err != nil {
			t.Fatal(err)
		}
		geometry := fmt.Sprintf(`{"type":"Point","coordinates":[%g,30,50]}`, 120+float64(n)/10)
		if _, err := f.db.Exec("insert into poses(observation_id,project_id,device_id,captured_at,standard_position) values($1,$2,$3,now()+$4*interval '1 second',ST_SetSRID(ST_GeomFromGeoJSON($5),4326))", observation, pid, device, n, geometry); err != nil {
			t.Fatal(err)
		}
	}
	res := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/snapshot", pid), "")
	defer res.Body.Close()
	var data map[string]any
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("snapshot %+v", data)
	}
	devices := data["devices"].([]any)
	pose := devices[0].(map[string]any)["pose"].(map[string]any)
	if pose["longitude"] != 120.1 || pose["latitude"] != float64(30) || pose["altitudeMeters"] != float64(50) {
		t.Fatalf("pose %+v", pose)
	}
	tracks := data["tracks"].([]any)
	geometry := tracks[0].(map[string]any)["geometry"].(map[string]any)
	if geometry["type"] != "LineString" || len(geometry["coordinates"].([]any)) != 2 {
		t.Fatalf("track %+v", geometry)
	}
	for n, point := range geometry["coordinates"].([]any) {
		coordinates := point.([]any)
		if len(coordinates) != 3 || coordinates[0] != 120+float64(n)/10 || coordinates[1] != float64(30) || coordinates[2] != float64(50) {
			t.Fatalf("GeoJSON round trip point %d: %+v", n, coordinates)
		}
	}
	if data["freshness"].(map[string]any)["isRealtime"] != true {
		t.Fatal("device freshness lost")
	}
	for _, suffix := range []string{"devices", "device-tree"} {
		response := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/%s", pid, suffix), "")
		var rows []map[string]any
		if err := json.NewDecoder(response.Body).Decode(&rows); err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 || len(rows) != 1 || rows[0]["id"] != float64(device) {
			t.Fatalf("%s rows %+v", suffix, rows)
		}
	}
	if _, err := f.db.Exec("insert into project_events(project_id,team_id,event_id,event_type) select $1,$2,'replay-'||n,'pose' from generate_series(1,2002) n", pid, team); err != nil {
		t.Fatal(err)
	}
	for _, filter := range []struct {
		query  string
		points int
	}{{"bbox=119,29,121,31&deviceTypes=drone", 2}, {"bbox=0,0,1,1", 0}, {"deviceTypes=unknown.type", 0}} {
		response := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/replay?%s&to=%s", pid, filter.query, url.QueryEscape(timestamp(time.Now().Add(time.Minute)))), "")
		var replay map[string]any
		if err := json.NewDecoder(response.Body).Decode(&replay); err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 || replay["mode"] != "replay" || replay["truncated"] != true || len(replay["events"].([]any)) != 2000 {
			t.Fatalf("replay metadata %+v", replay)
		}
		if len(replay["poses"].([]any)) != filter.points {
			t.Fatalf("filter ignored %+v", replay["poses"])
		}
	}
	bad := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/replay?bbox=190,0,200,10", pid), "")
	raw, _ := io.ReadAll(bad.Body)
	bad.Body.Close()
	if bad.StatusCode != 400 || !strings.Contains(string(raw), "INVALID_REPLAY_BBOX") {
		t.Fatalf("invalid bbox %d %s", bad.StatusCode, raw)
	}
	if _, err := f.db.Exec("delete from team_members where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"devices", "device-tree", "replay", fmt.Sprintf("devices/%d", device)} {
		res := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/%s", pid, suffix), "")
		res.Body.Close()
		if res.StatusCode != 404 {
			t.Fatalf("scope %s %d", suffix, res.StatusCode)
		}
	}
}
