package flighthub

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func readSmokeManifestFixture(t *testing.T) []ReadOnlySmokeEndpoint {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve smoke manifest")
	}
	file, err := os.Open(filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/endpoints.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	endpoints, err := LoadReadOnlySmokeManifest(file)
	if err != nil {
		t.Fatal(err)
	}
	return endpoints
}

func TestReadOnlySmokeManifestCoversEveryReleasedGET(t *testing.T) {
	t.Parallel()
	endpoints := readSmokeManifestFixture(t)
	if len(endpoints) != ReadOnlySmokeEndpointCount {
		t.Fatalf("read smoke endpoint count=%d", len(endpoints))
	}
	seen := map[string]bool{}
	for _, endpoint := range endpoints {
		if seen[endpoint.ID] || endpoint.ID == "" || endpoint.Domain == "" || !strings.HasPrefix(endpoint.Path, "/openapi/v2.0/") {
			t.Fatalf("invalid read smoke endpoint: %#v", endpoint)
		}
		seen[endpoint.ID] = true
	}
}

func TestReadOnlySmokeUsesOnlyGETAndEmitsShapeWithoutResponseValues(t *testing.T) {
	t.Parallel()
	endpoints := readSmokeManifestFixture(t)
	var requests atomic.Int64
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.Body != nil {
			t.Fatalf("smoke attempted a mutating request: method=%s body=%v", request.Method, request.Body)
		}
		requests.Add(1)
		return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{"list":[{"name":"TOKEN_SECRET_UUID_SN_22.22_113.11","status":"https://signed.example/private"}]}}`), nil), nil
	}), func(config *Config) { config.MaxRetries = 0 })
	scope := ReadOnlySmokeContext{
		ProjectUUID:      "00000000-0000-4000-8000-000000000001",
		OrganizationUUID: "00000000-0000-4000-8000-000000000010",
		Values: map[string]string{
			"workspace_id": "00000000-0000-4000-8000-000000000001", "device_sn": "DEVICE_REDACTED", "sn": "DEVICE_REDACTED",
			"task_uuid": "TASK_REDACTED", "wayline_id": "WAYLINE_REDACTED", "model_id": "17",
			"model_uuid": "MODEL_REDACTED", "resource_uuid": "RESOURCE_REDACTED", "file_id": "19",
		},
	}
	results := RunReadOnlySmoke(context.Background(), client, "TOKEN_REDACTED", endpoints, scope)
	if len(results) != ReadOnlySmokeEndpointCount || requests.Load() != ReadOnlySmokeEndpointCount {
		t.Fatalf("read smoke coverage results=%d requests=%d", len(results), requests.Load())
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	output := string(encoded)
	for _, forbidden := range []string{"TOKEN_SECRET", "00000000-0000-4000-8000-000000000001", "DEVICE_REDACTED", "22.22", "113.11", "https://signed.example"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("smoke output leaked forbidden response or request value %q", forbidden)
		}
	}
	for _, result := range results {
		if result.Category != "succeeded" || result.Count != 1 || strings.Join(result.Fields, ",") != "name,status" {
			t.Fatalf("unexpected safe smoke result: %#v", result)
		}
	}
}

func TestReadOnlySmokeSkipsUnresolvedPathWithoutCallingUpstream(t *testing.T) {
	t.Parallel()
	var requests atomic.Int64
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return response(http.StatusOK, []byte(`{"code":0,"data":{}}`), nil), nil
	}), nil)
	results := RunReadOnlySmoke(context.Background(), client, "TOKEN_REDACTED", []ReadOnlySmokeEndpoint{{
		ID: "endpoint-redacted", Path: "/openapi/v2.0/device/{device_sn}/state", Domain: "device", Scope: "device",
	}}, ReadOnlySmokeContext{ProjectUUID: "00000000-0000-4000-8000-000000000001", Values: map[string]string{}})
	if requests.Load() != 0 || len(results) != 1 || results[0].Category != "prerequisite_missing" {
		t.Fatalf("unresolved endpoint was not failed closed: requests=%d results=%#v", requests.Load(), results)
	}
}
