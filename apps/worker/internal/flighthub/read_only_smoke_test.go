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
	"time"

	"aerosight/worker/internal/connector"
)

type readOnlySmokeEvidenceStore struct {
	fingerprint string
	snapshots   []connector.CapabilitySnapshot
}

func (store *readOnlySmokeEvidenceStore) SaveCapabilityAccountFingerprint(_ context.Context, _ connector.Instance, fingerprint string) error {
	store.fingerprint = fingerprint
	return nil
}

func (store *readOnlySmokeEvidenceStore) SaveCapabilitySnapshot(_ context.Context, _ connector.Instance, snapshot connector.CapabilitySnapshot) error {
	store.snapshots = append(store.snapshots, snapshot)
	return nil
}

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

func TestReadOnlySmokeAcceptsMissingDataAndPreservesConfigurationRequired(t *testing.T) {
	t.Parallel()
	responses := [][]byte{
		[]byte(`{"code":0,"message":"success"}`),
		[]byte(`{"code":200610,"message":"sensitive upstream details"}`),
	}
	var requestIndex atomic.Int64
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			t.Fatalf("smoke attempted method %s", request.Method)
		}
		index := int(requestIndex.Add(1)) - 1
		status := http.StatusOK
		if index == 1 {
			status = http.StatusBadRequest
		}
		return response(status, responses[index], nil), nil
	}), func(config *Config) { config.MaxRetries = 0 })
	results := RunReadOnlySmoke(context.Background(), client, "TOKEN_REDACTED", []ReadOnlySmokeEndpoint{
		{ID: "missing-data", Path: "/openapi/v2.0/health", Domain: "system", Scope: "global"},
		{ID: "configuration", Path: "/openapi/v2.0/cloud-controls", Domain: "control", Scope: "project"},
	}, ReadOnlySmokeContext{ProjectUUID: "00000000-0000-4000-8000-000000000001", Values: map[string]string{}})
	if len(results) != 2 || results[0].Category != "empty" || results[1].Category != "configuration_required" {
		t.Fatalf("unexpected safe categories: %#v", results)
	}
}

func TestReadOnlySmokeTreatsNullListAsEmpty(t *testing.T) {
	t.Parallel()
	category, count, fields := summarizeSmokePayload(envelope{Data: json.RawMessage(`{"list":null}`)})
	if category != "empty" || count != 0 || strings.Join(fields, ",") != "list" {
		t.Fatalf("null list was not summarized as an empty collection: category=%s count=%d fields=%v", category, count, fields)
	}
}

func TestReadOnlySmokeHydratesAccountScopeWithoutExposingVendorIDs(t *testing.T) {
	t.Parallel()
	projectUUID := "00000000-0000-4000-8000-000000000001"
	organizationUUID := "00000000-0000-4000-8000-000000000010"
	userID := "CURRENT_VENDOR_USER_MUST_NOT_ESCAPE"
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/openapi/v2.0/project":
			return response(http.StatusOK, []byte(`{"code":0,"data":{"list":[{"uuid":"`+projectUUID+`","name":"synthetic","org_uuid":"`+organizationUUID+`"}]}}`), nil), nil
		case "/openapi/v2.0/organizations/" + organizationUUID + "/users/current":
			return response(http.StatusOK, []byte(`{"code":0,"data":{"user_id":"`+userID+`","org_uuid":"`+organizationUUID+`","role":"organization-admin"}}`), nil), nil
		default:
			t.Fatalf("unexpected hydration path %s", request.URL.Path)
			return nil, nil
		}
	}), func(config *Config) { config.MaxRetries = 0 })
	hydrated, err := HydrateReadOnlySmokeContext(context.Background(), client, "TOKEN_REDACTED", ReadOnlySmokeContext{
		ProjectUUID: projectUUID, Values: map[string]string{"workspace_id": projectUUID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hydrated.OrganizationUUID != organizationUUID || !validAccountFingerprint(hydrated.AccountFingerprint) {
		t.Fatalf("scope was not hydrated: %#v", hydrated)
	}
	encoded, err := json.Marshal(struct {
		Fingerprint string `json:"fingerprint"`
	}{Fingerprint: hydrated.AccountFingerprint})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), organizationUUID) || strings.Contains(string(encoded), userID) || strings.Contains(string(encoded), projectUUID) {
		t.Fatal("hydration evidence exposed a raw vendor identifier")
	}
}

func TestReadOnlySmokeLearnsSafeListPrerequisitesBeforeDetails(t *testing.T) {
	t.Parallel()
	var requests atomic.Int64
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		switch request.URL.Path {
		case "/openapi/v2.0/wayline":
			return response(http.StatusOK, []byte(`{"code":0,"data":{"list":[{"id":"WAYLINE_VENDOR_ID_MUST_NOT_ESCAPE","name":"synthetic"}]}}`), nil), nil
		case "/openapi/v2.0/wayline/WAYLINE_VENDOR_ID_MUST_NOT_ESCAPE":
			return response(http.StatusOK, []byte(`{"code":0,"data":{"id":"WAYLINE_VENDOR_ID_MUST_NOT_ESCAPE","name":"synthetic"}}`), nil), nil
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
			return nil, nil
		}
	}), func(config *Config) { config.MaxRetries = 0 })
	results := RunReadOnlySmoke(context.Background(), client, "TOKEN_REDACTED", []ReadOnlySmokeEndpoint{
		{ID: "456680825e0", Path: "/openapi/v2.0/wayline/{wayline_id}", Domain: "flight", Scope: "project"},
		{ID: "456680824e0", Path: "/openapi/v2.0/wayline", Domain: "flight", Scope: "project"},
	}, ReadOnlySmokeContext{ProjectUUID: "00000000-0000-4000-8000-000000000001", Values: map[string]string{}})
	encoded, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 || len(results) != 2 || results[0].Endpoint != "456680824e0" || results[1].Category != "succeeded" {
		t.Fatalf("list prerequisite did not unlock detail safely: requests=%d results=%#v", requests.Load(), results)
	}
	if strings.Contains(string(encoded), "WAYLINE_VENDOR_ID_MUST_NOT_ESCAPE") {
		t.Fatal("learned prerequisite leaked into smoke output")
	}
}

func TestReadOnlySmokePersistsSanitizedLiveReadEvidenceWithoutEnablingWrites(t *testing.T) {
	t.Parallel()
	endpoints := readSmokeManifestFixture(t)
	results := make([]ReadOnlySmokeResult, 0, len(endpoints))
	for _, endpoint := range endpoints {
		category := "succeeded"
		switch endpoint.ID {
		case "456458604e0":
			category = "configuration_required"
		case "457494965e0":
			category = "empty"
		case "454273416e0":
			category = "scope_forbidden"
		}
		results = append(results, ReadOnlySmokeResult{Endpoint: endpoint.ID, Category: category, Count: 1, Fields: []string{"status", "uuid"}, DurationMS: 7})
	}
	store := &readOnlySmokeEvidenceStore{}
	verifiedAt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	fingerprint := strings.Repeat("a", 64)
	if err := PersistReadOnlySmokeEvidence(context.Background(), store, connector.Instance{ID: 7, ProjectID: 11}, endpoints, results,
		ReadOnlySmokeContext{AccountFingerprint: fingerprint}, verifiedAt, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	if store.fingerprint != fingerprint || len(store.snapshots) == 0 {
		t.Fatalf("live evidence was not persisted: fingerprint=%q snapshots=%d", store.fingerprint, len(store.snapshots))
	}
	var controlEvidence *connector.CapabilitySnapshot
	for index := range store.snapshots {
		snapshot := &store.snapshots[index]
		if snapshot.EvidenceLevel != "live-read" || snapshot.AccountFingerprint != fingerprint || snapshot.ExpiresAt == nil ||
			!snapshot.ExpiresAt.Equal(verifiedAt.Add(15*time.Minute)) || snapshot.Details["source"] != "read-only-smoke" {
			t.Fatalf("unsafe or incomplete evidence: %#v", snapshot)
		}
		if snapshot.CapabilityCode == "device.control" {
			controlEvidence = snapshot
		}
	}
	encoded, err := json.Marshal(store.snapshots)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"TOKEN_REDACTED", "CURRENT_VENDOR_USER", "00000000-0000-4000-8000-000000000001", "22.22", "113.11", "https://"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("persisted evidence leaked %q", forbidden)
		}
	}
	if controlEvidence == nil {
		t.Fatal("cloud-control GET evidence was not recorded")
	}
	baseline := []CapabilityProbeResult{{CapabilityCode: "device.control", Status: ProbeUnverified,
		Layers: CapabilityProbeLayers{Contract: ProbeSupported, Deployment: ProbeSupported, Account: ProbeSupported, Implementation: ProbeSupported, Acceptance: ProbeUnverified}}}
	effective := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{*controlEvidence}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: fingerprint, DeviceModel: "dock", FirmwareVersion: "1", Now: verifiedAt,
	})
	if effective[0].Status == ProbeSupported || effective[0].Layers.Acceptance == ProbeSupported {
		t.Fatalf("live-read evidence enabled a high-risk write: %#v", effective[0])
	}
}
