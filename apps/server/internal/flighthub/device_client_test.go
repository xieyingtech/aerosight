package flighthub

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type deviceContractFixture struct {
	ContractVersion string               `json:"contractVersion"`
	Cases           []deviceContractCase `json:"cases"`
}

type deviceContractCase struct {
	Name   string          `json:"name"`
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body"`
}

func loadDeviceFixture(t *testing.T) map[string]deviceContractCase {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve device fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/fixtures/device_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture deviceContractFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.ContractVersion == "" || len(fixture.Cases) != 9 {
		t.Fatalf("invalid device fixture metadata: version=%q cases=%d", fixture.ContractVersion, len(fixture.Cases))
	}
	byName := make(map[string]deviceContractCase, len(fixture.Cases))
	for _, item := range fixture.Cases {
		byName[item.Name] = item
	}
	return byName
}

func deviceFixtureClient(t *testing.T, item deviceContractCase) *Client {
	t.Helper()
	return testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != item.Method || request.URL.RequestURI() != item.Path {
			t.Fatalf("request = %s %s, want %s %s", request.Method, request.URL.RequestURI(), item.Method, item.Path)
		}
		if request.Header.Get("X-User-Token") != "TOKEN_REDACTED" || request.Header.Get("X-Project-Uuid") != "PROJECT_REDACTED" || request.Header.Get("X-Request-Id") != "request-redacted" || request.Header.Get("X-Language") != "zh" {
			t.Fatalf("missing required project headers: %#v", request.Header)
		}
		return response(http.StatusOK, item.Body, nil), nil
	}), nil)
}

func TestDeviceReadClientsParseOfficialDock2AndM3TDFixtures(t *testing.T) {
	cases := loadDeviceFixture(t)
	ctx := context.Background()

	detail, err := deviceFixtureClient(t, cases["device-detail-dock2"]).GetDeviceDetail(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "DOCK_REDACTED_01")
	if err != nil || detail.Model.Key != "3-2-0" || detail.Model.Class != "airport" {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
	dock, err := deviceFixtureClient(t, cases["state-dock2"]).GetDeviceState(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "DOCK_REDACTED_01")
	if err != nil || dock.Model.Key != "3-2-0" || dock.State["latitude"] == nil || dock.State["network_state"] == nil {
		t.Fatalf("dock state=%#v err=%v", dock, err)
	}
	aircraft, err := deviceFixtureClient(t, cases["state-m3td"]).GetDeviceState(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "AIRCRAFT_REDACTED_01")
	if err != nil || aircraft.Model.Key != "0-91-1" || aircraft.State["attitude_head"] == nil || aircraft.State["battery"] == nil {
		t.Fatalf("aircraft state=%#v err=%v", aircraft, err)
	}

	hms, err := deviceFixtureClient(t, cases["device-hms"]).ListDeviceHMS(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", []string{"DOCK_REDACTED_01", "AIRCRAFT_REDACTED_01"})
	if err != nil || len(hms) != 1 || len(hms[0].Alerts.List) != 1 || hms[0].Alerts.List[0].ID == "" {
		t.Fatalf("hms=%#v err=%v", hms, err)
	}
	topologies, err := deviceFixtureClient(t, cases["historical-topology"]).ListHistoricalTopologies(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
	if err != nil || len(topologies) != 1 || topologies[0].Host == nil || len(topologies[0].Parents) != 1 {
		t.Fatalf("topologies=%#v err=%v", topologies, err)
	}
	recording, err := deviceFixtureClient(t, cases["auto-record-config"]).GetAutoRecordingConfig(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
	if err != nil || recording.ID != 8 || len(recording.Items) != 1 {
		t.Fatalf("recording=%#v err=%v", recording, err)
	}
}

func TestDeviceStateSchemaFailsClosedButPreservesUnknownModelsForMapperDiagnostics(t *testing.T) {
	cases := loadDeviceFixture(t)
	ctx := context.Background()

	_, err := deviceFixtureClient(t, cases["state-missing-fields"]).GetDeviceState(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "MISSING_REDACTED")
	if !IsSafeCode(err, "schema_incompatible") {
		t.Fatalf("missing state error=%v", err)
	}
	unknown, err := deviceFixtureClient(t, cases["state-unknown-model"]).GetDeviceState(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "UNKNOWN_REDACTED")
	if err != nil || unknown.Model.Key != "9-9-9" || unknown.State["future_field"] == nil {
		t.Fatalf("unknown model=%#v err=%v", unknown, err)
	}
	invalidCoordinates, err := deviceFixtureClient(t, cases["state-invalid-coordinates"]).GetDeviceState(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "INVALID_COORD_REDACTED")
	if err != nil || invalidCoordinates.State["latitude"] == nil || invalidCoordinates.State["longitude"] == nil {
		t.Fatalf("invalid coordinate fixture should reach mapper: %#v err=%v", invalidCoordinates, err)
	}
}

func TestDeviceReadClientsRejectInvalidScopeAndHMSBatchBeforeNetwork(t *testing.T) {
	called := false
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	}), nil)
	if _, err := client.GetDeviceState(context.Background(), "TOKEN_REDACTED", "", "DOCK_REDACTED_01"); !IsSafeCode(err, "scope_forbidden") {
		t.Fatalf("scope error=%v", err)
	}
	if _, err := client.ListDeviceHMS(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", nil); !IsSafeCode(err, "request_invalid") {
		t.Fatalf("HMS batch error=%v", err)
	}
	if called {
		t.Fatal("invalid device request reached upstream")
	}
}
