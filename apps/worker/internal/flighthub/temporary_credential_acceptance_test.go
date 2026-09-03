package flighthub

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
)

type temporaryCredentialAcceptanceStore struct {
	fingerprint string
	snapshots   []connector.CapabilitySnapshot
}

func (store *temporaryCredentialAcceptanceStore) SaveCapabilityAccountFingerprint(_ context.Context, _ connector.Instance, fingerprint string) error {
	store.fingerprint = fingerprint
	return nil
}

func (store *temporaryCredentialAcceptanceStore) SaveCapabilitySnapshot(_ context.Context, _ connector.Instance, snapshot connector.CapabilitySnapshot) error {
	store.snapshots = append(store.snapshots, snapshot)
	return nil
}

func TestTemporaryCredentialAcceptanceCallsOnlyCredentialIssuanceEndpointsAndRedactsValues(t *testing.T) {
	t.Parallel()
	var requests atomic.Int64
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		if request.Method != http.MethodPost || strings.Contains(request.URL.Path, "flight-task") || strings.Contains(request.URL.Path, "reconstruction") {
			t.Fatalf("unsafe acceptance request: %s %s", request.Method, request.URL.Path)
		}
		switch request.URL.Path {
		case "/openapi/v2.0/project/sts-token":
			return response(http.StatusOK, []byte(`{"code":0,"data":{"endpoint":"https://objects.vendor.example","provider":"ali","region":"SECRET_REGION","bucket":"SECRET_BUCKET","object_key_prefix":"SECRET_PATH","credentials":{"access_key_id":"SECRET_ACCESS","access_key_secret":"SECRET_KEY","expire":900,"security_token":"SECRET_TOKEN","platform":0}}}`), nil), nil
		case "/openapi/v2.0/open_model/stores/obtain_token":
			expires := time.Now().Add(time.Hour).Unix()
			return response(http.StatusOK, []byte(`{"code":0,"data":{"cloud_name":"ali","access_key_id":"SECRET_ACCESS","secret_access_key":"SECRET_KEY","session_token":"SECRET_TOKEN","region":"SECRET_REGION","cloud_bucket_name":"SECRET_BUCKET","callback_param":"SECRET_CALLBACK","store_path":"SECRET/{fileName}","expire_time":`+jsonNumber(expires)+`,"end_point":"https://objects.vendor.example"}}`), nil), nil
		default:
			t.Fatalf("unexpected acceptance path %s", request.URL.Path)
			return nil, nil
		}
	}), func(config *Config) {
		config.MaxRetries = 0
		config.AllowedLinkHosts = []string{"objects.vendor.example"}
	})
	var guardCalls atomic.Int64
	results := RunTemporaryCredentialAcceptance(context.Background(), client, "TOKEN_SECRET", "00000000-0000-4000-8000-000000000001", "acceptance-file-redacted", func(_ context.Context, endpoint string) error {
		index := guardCalls.Add(1) - 1
		if endpoint != temporaryCredentialAcceptanceEndpoints[index] {
			t.Fatalf("unexpected guarded endpoint %s", endpoint)
		}
		return nil
	})
	if requests.Load() != 2 || guardCalls.Load() != 2 || len(results) != 2 {
		t.Fatalf("requests=%d guards=%d results=%#v", requests.Load(), guardCalls.Load(), results)
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"TOKEN_SECRET", "SECRET_ACCESS", "SECRET_KEY", "SECRET_TOKEN", "SECRET_REGION", "SECRET_BUCKET", "SECRET_PATH", "SECRET_CALLBACK", "00000000-0000-4000-8000-000000000001", "acceptance-file-redacted", "https://"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("acceptance output leaked %q", secret)
		}
	}
	for _, result := range results {
		if result.Category != "succeeded" || result.Count != 1 || len(result.Fields) == 0 {
			t.Fatalf("unexpected result: %#v", result)
		}
	}
}

func TestTemporaryCredentialAcceptanceDoesNotPersistPartialOrUnknownEvidence(t *testing.T) {
	t.Parallel()
	store := &temporaryCredentialAcceptanceStore{}
	results := []TemporaryCredentialAcceptanceResult{
		{Endpoint: "454273351e0", Category: "succeeded", Count: 1, Fields: []string{"provider"}, DurationMS: 1},
		{Endpoint: "458069518e0", Category: "request_timeout", Fields: []string{}, DurationMS: 2},
	}
	err := PersistTemporaryCredentialAcceptanceEvidence(context.Background(), store, connector.Instance{ID: 7, ProjectID: 11}, results, strings.Repeat("a", 64), time.Now().UTC(), time.Hour)
	if !IsSafeCode(err, "acceptance_incomplete") || store.fingerprint != "" || len(store.snapshots) != 0 {
		t.Fatalf("partial acceptance produced evidence: err=%v store=%#v", err, store)
	}
}

func TestTemporaryCredentialAcceptancePersistsOnlyAccountBoundTemporaryCredentialEvidence(t *testing.T) {
	t.Parallel()
	store := &temporaryCredentialAcceptanceStore{}
	results := []TemporaryCredentialAcceptanceResult{
		{Endpoint: "454273351e0", Category: "succeeded", Count: 1, Fields: []string{"provider"}, DurationMS: 1},
		{Endpoint: "458069518e0", Category: "succeeded", Count: 1, Fields: []string{"cloud_name"}, DurationMS: 2},
	}
	verifiedAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	fingerprint := strings.Repeat("a", 64)
	if err := PersistTemporaryCredentialAcceptanceEvidence(context.Background(), store, connector.Instance{ID: 7, ProjectID: 11}, results, fingerprint, verifiedAt, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if store.fingerprint != fingerprint || len(store.snapshots) != 1 {
		t.Fatalf("unexpected evidence store: %#v", store)
	}
	snapshot := store.snapshots[0]
	if snapshot.CapabilityCode != TemporaryCredentialCapability || snapshot.EvidenceLevel != "field-write" || snapshot.Status != "supported" ||
		snapshot.DeviceModel != "" || snapshot.FirmwareVersion != "" || snapshot.ExpiresAt == nil || !snapshot.ExpiresAt.Equal(verifiedAt.Add(24*time.Hour)) {
		t.Fatalf("unsafe acceptance scope: %#v", snapshot)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"TOKEN_", "SECRET_", "00000000-", "https://"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("evidence leaked %q", forbidden)
		}
	}
	baseline := []CapabilityProbeResult{{CapabilityCode: TemporaryCredentialCapability, Status: ProbeUnverified,
		Layers: CapabilityProbeLayers{Contract: ProbeSupported, Deployment: ProbeSupported, Account: ProbeSupported, Implementation: ProbeSupported, Acceptance: ProbeUnverified}}}
	effective := ApplyCapabilitySnapshots(baseline, store.snapshots, CapabilityEvaluationScope{Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: fingerprint, Now: verifiedAt})
	if len(effective) != 1 || effective[0].Status != ProbeSupported {
		t.Fatalf("current acceptance was not applied: %#v", effective)
	}
}

func TestTemporaryCredentialAcceptanceMapsFailuresToSafeCategories(t *testing.T) {
	t.Parallel()
	client := &temporaryCredentialAcceptanceClientFixture{err: &APIError{SafeCode: "scope_forbidden"}}
	results := RunTemporaryCredentialAcceptance(context.Background(), client, "TOKEN_SECRET", "project-redacted", "file-redacted", func(context.Context, string) error { return nil })
	encoded, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Category != "scope_forbidden" || results[1].Category != "scope_forbidden" || strings.Contains(string(encoded), "DJI_FLIGHTHUB") {
		t.Fatalf("failure was not safely classified: %#v", results)
	}
}

func TestTemporaryCredentialAcceptanceGuardBlocksEveryRemoteRequest(t *testing.T) {
	t.Parallel()
	client := &temporaryCredentialAcceptanceClientFixture{}
	results := RunTemporaryCredentialAcceptance(context.Background(), client, "TOKEN_SECRET", "project-redacted", "file-redacted", func(context.Context, string) error {
		return &APIError{SafeCode: "connector_changed"}
	})
	if client.calls.Load() != 0 || len(results) != 2 || results[0].Category != "connector_changed" || results[1].Category != "connector_changed" {
		t.Fatalf("guard did not fail closed: calls=%d results=%#v", client.calls.Load(), results)
	}
}

type temporaryCredentialAcceptanceClientFixture struct {
	err   error
	calls atomic.Int64
}

func (client *temporaryCredentialAcceptanceClientFixture) CreateStorageSTS(context.Context, string, string, StorageSTSRequest) (StorageSTS, error) {
	client.calls.Add(1)
	return StorageSTS{}, client.err
}

func (client *temporaryCredentialAcceptanceClientFixture) ObtainOpenModelUploadCredential(context.Context, string, string) (OpenModelUploadCredential, error) {
	client.calls.Add(1)
	return OpenModelUploadCredential{}, client.err
}

func jsonNumber(value int64) string {
	return strconv.FormatInt(value, 10)
}
