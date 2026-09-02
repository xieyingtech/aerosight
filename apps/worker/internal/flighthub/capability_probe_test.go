package flighthub

import (
	"context"
	"encoding/csv"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func probeResult(t *testing.T, results []CapabilityProbeResult, code string) CapabilityProbeResult {
	t.Helper()
	for _, result := range results {
		if result.CapabilityCode == code {
			return result
		}
	}
	t.Fatalf("capability %q missing from probe results", code)
	return CapabilityProbeResult{}
}

func TestCapabilityProbeUsesOnlyReleasedGETContracts(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve endpoint manifest")
	}
	file, err := os.Open(filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/endpoints.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = 11
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	manifest := make(map[string][]string, len(rows)-1)
	for _, row := range rows[1:] {
		manifest[row[0]] = row
	}
	covered := make(map[string]bool)
	for _, endpoint := range defaultCapabilityProbeEndpoints {
		row, exists := manifest[endpoint.ID]
		if !exists || row[1] != http.MethodGet || endpoint.Method != http.MethodGet || row[2] != endpoint.Path || row[3] != "released" || row[9] != endpoint.Deployments[0] || len(endpoint.Regions) != 1 || endpoint.Regions[0] != "cn" {
			t.Fatalf("probe endpoint is not an exact released GET contract: %#v row=%#v", endpoint, row)
		}
		for _, capabilityCode := range endpoint.CapabilityCodes {
			covered[capabilityCode] = true
		}
	}
	for _, capability := range Capabilities() {
		if !covered[capability.Code] {
			t.Fatalf("capability %q has no safe evidence endpoint", capability.Code)
		}
		readiness, exists := defaultCapabilityReadiness[capability.Code]
		if !exists {
			t.Fatalf("capability %q has no implementation/acceptance layer", capability.Code)
		}
		if capability.Kind == "action" && readiness.Accepted {
			t.Fatalf("action capability %q bypassed field acceptance", capability.Code)
		}
	}
}

func TestCapabilityProbeIntersectsDeploymentAccountImplementationAndAcceptance(t *testing.T) {
	t.Parallel()
	var mutex sync.Mutex
	requested := make([]string, 0, 4)
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			t.Fatalf("capability probe issued side-effecting method %s", request.Method)
		}
		mutex.Lock()
		requested = append(requested, request.URL.Path)
		mutex.Unlock()
		switch request.URL.Path {
		case "/openapi/v2.0/organizations":
			return response(http.StatusForbidden, []byte(`{"code":200403,"message":"redacted","data":{}}`), nil), nil
		case "/openapi/v2.0/live-shares":
			return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{"list":[]}}`), nil), nil
		case "/openapi/v2.0/model":
			return response(http.StatusOK, []byte(`{"code":299999,"message":"redacted","data":{}}`), nil), nil
		case "/openapi/v2.0/wayline":
			return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{"list":[{"id":"redacted"}]}}`), nil), nil
		default:
			t.Fatalf("unexpected probe path %s", request.URL.Path)
			return nil, nil
		}
	}), nil)
	endpoints := []capabilityProbeEndpoint{
		{ID: "private-only", Method: http.MethodGet, Path: "/openapi/v2.0/project/device", Scope: "project", Released: true, Deployments: []string{"cn-private"}, CapabilityCodes: []string{"inventory.read"}},
		{ID: "forbidden", Method: http.MethodGet, Path: "/openapi/v2.0/organizations", Scope: "global", Released: true, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"organization.read"}},
		{ID: "empty", Method: http.MethodGet, Path: "/openapi/v2.0/live-shares", Scope: "project", Released: true, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"live.read"}},
		{ID: "unknown-business", Method: http.MethodGet, Path: "/openapi/v2.0/model", Scope: "project", Released: true, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"model.read"}},
		{ID: "not-implemented", Method: http.MethodGet, Path: "/openapi/v2.0/wayline", Scope: "project", Released: true, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"flight.read"}},
		{ID: "write", Method: http.MethodPost, Path: "/openapi/v2.0/device/redacted/command", Scope: "project", Released: true, Deployments: []string{"cn-public-cloud"}, CapabilityCodes: []string{"device.control"}},
	}
	readiness := map[string]capabilityReadiness{
		"inventory.read":    {Implemented: true, Accepted: true},
		"organization.read": {Implemented: true, Accepted: true},
		"live.read":         {Implemented: true, Accepted: true},
		"model.read":        {Implemented: true, Accepted: true},
		"flight.read":       {Accepted: true},
		"device.control":    {Implemented: true},
	}
	results, err := client.probeCapabilities(context.Background(), CapabilityProbeInput{
		Token: "TOKEN_REDACTED", Region: "cn", Deployment: "cn-public-cloud", ProjectUUID: "PROJECT_REDACTED",
	}, endpoints, readiness)
	if err != nil {
		t.Fatal(err)
	}
	if result := probeResult(t, results, "inventory.read"); result.Status != ProbeNotApplicable || result.Layers.Deployment != ProbeNotApplicable {
		t.Fatalf("private-only endpoint was exposed in public cloud: %#v", result)
	}
	if result := probeResult(t, results, "organization.read"); result.Status != ProbeForbidden || result.Layers.Account != ProbeForbidden {
		t.Fatalf("403 was not preserved as forbidden evidence: %#v", result)
	}
	if result := probeResult(t, results, "live.read"); result.Status != ProbeEmpty || result.Layers.Account != ProbeEmpty {
		t.Fatalf("empty list degraded connector health: %#v", result)
	}
	if result := probeResult(t, results, "model.read"); result.Status != ProbeUnverified || result.Layers.Account != ProbeUnverified {
		t.Fatalf("unknown business code widened support: %#v", result)
	}
	if result := probeResult(t, results, "flight.read"); result.Status != ProbeUnverified || result.Layers.Account != ProbeSupported || result.Layers.Implementation != ProbeUnverified {
		t.Fatalf("account success bypassed local implementation layer: %#v", result)
	}
	if result := probeResult(t, results, "device.control"); result.Status != ProbeUnverified || result.Layers.Account != ProbeUnverified || result.Layers.Acceptance != ProbeUnverified {
		t.Fatalf("write capability bypassed GET-only acceptance boundary: %#v", result)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(requested) != 4 {
		t.Fatalf("expected exactly four safe GET probes, got %v", requested)
	}
	for _, forbiddenPath := range []string{"/openapi/v2.0/project/device", "/openapi/v2.0/device/redacted/command"} {
		for _, path := range requested {
			if path == forbiddenPath {
				t.Fatalf("inapplicable or write endpoint was called: %s", path)
			}
		}
	}
}

func TestTCAProbePersistsOnlyBoundedCountAndEmptyState(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name, data string
		status     CapabilityProbeStatus
		count      int
	}{
		{"items", `[{"vendor_field":"discarded"},{"another":true}]`, ProbeSupported, 2},
		{"null", `null`, ProbeEmpty, 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path != "/openapi/v2.0/workspaces/WORKSPACE_REDACTED/groups/tcas" {
					t.Fatalf("unexpected path %s", request.URL.Path)
				}
				return response(http.StatusOK, []byte(`{"code":0,"message":"","data":`+testCase.data+`}`), nil), nil
			}), nil)
			observation := client.probeEndpoint(context.Background(), CapabilityProbeInput{Token: "TOKEN_REDACTED", Region: "cn", Deployment: "cn-public-cloud", ProjectUUID: "WORKSPACE_REDACTED"},
				capabilityProbeEndpoint{ID: "454273421e0", Method: http.MethodGet, Path: "/openapi/v2.0/workspaces/{workspace_id}/groups/tcas", Scope: "workspace", Released: true,
					Regions: []string{"cn"}, Deployments: []string{"cn-public-cloud"}, TemplateParameter: "workspace_id"})
			if observation.Status != testCase.status || observation.ItemCount == nil || *observation.ItemCount != testCase.count {
				t.Fatalf("observation=%#v", observation)
			}
		})
	}
}
