package flighthub

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
)

type liveAcceptanceClientFixture struct {
	calls int
	input LiveStreamStartRequest
	value LiveStreamAuthorization
	err   error
}

func (fixture *liveAcceptanceClientFixture) StartLiveStream(_ context.Context, _, _ string, input LiveStreamStartRequest) (LiveStreamAuthorization, error) {
	fixture.calls++
	fixture.input = input
	return fixture.value, fixture.err
}

type liveAcceptanceNormalizerFixture struct{ err error }

func (fixture liveAcceptanceNormalizerFixture) Normalize(input LiveStreamAuthorization) (NormalizedLivePlayback, error) {
	if fixture.err != nil {
		return NormalizedLivePlayback{}, fixture.err
	}
	return NormalizedLivePlayback{Description: LivePlaybackDescription{Supplier: input.URLType, Protocol: "test-rtc"}}, nil
}

func TestLiveControlAcceptanceGuardsExactDockStartAndRedactsCredential(t *testing.T) {
	client := &liveAcceptanceClientFixture{value: LiveStreamAuthorization{URL: "SECRET_PLAYBACK_CREDENTIAL", URLType: "volc"}}
	result := RunLiveControlAcceptance(context.Background(), client, liveAcceptanceNormalizerFixture{}, "SECRET_TOKEN", "project", "SERIAL", "165-0-7", func(context.Context) error { return nil })
	if result.Category != "succeeded" || result.Supplier != "volc" || result.Protocol != "test-rtc" || client.calls != 1 ||
		client.input.SN != "SERIAL" || client.input.CameraIndex != "165-0-7" || client.input.VideoExpire != 3600 || client.input.QualityType != LiveQualityAdaptive {
		t.Fatalf("unexpected live acceptance: result=%#v input=%#v", result, client.input)
	}
	if strings.Contains(result.Supplier+result.Protocol+strings.Join(result.Fields, ","), "SECRET") {
		t.Fatal("acceptance result leaked a credential")
	}
}

func TestLiveControlAcceptanceFailsClosedBeforeOrAfterRemoteStart(t *testing.T) {
	client := &liveAcceptanceClientFixture{}
	guarded := RunLiveControlAcceptance(context.Background(), client, liveAcceptanceNormalizerFixture{}, "token", "project", "serial", "camera", func(context.Context) error {
		return &APIError{SafeCode: "connector_changed"}
	})
	if guarded.Category != "connector_changed" || client.calls != 0 {
		t.Fatalf("guard did not fail before remote start: %#v calls=%d", guarded, client.calls)
	}
	client.value = LiveStreamAuthorization{URL: "secret", URLType: "unknown"}
	unsupported := RunLiveControlAcceptance(context.Background(), client, liveAcceptanceNormalizerFixture{err: &APIError{SafeCode: "live_supplier_unsupported"}}, "token", "project", "serial", "camera", func(context.Context) error { return nil })
	if unsupported.Category != "live_supplier_unsupported" || unsupported.Fields == nil || len(unsupported.Fields) != 0 {
		t.Fatalf("unsupported supplier was accepted: %#v", unsupported)
	}
}

func TestPersistLiveControlAcceptanceEvidenceRequiresCompleteRealResult(t *testing.T) {
	store := &temporaryCredentialAcceptanceStore{}
	now := time.Now().UTC()
	result := LiveControlAcceptanceResult{Endpoint: LiveControlAcceptanceEndpoint, Category: "succeeded", Fields: []string{"url_type", "expire_ts", "url"}, Supplier: "volc", Protocol: "volc-rtc", DurationMS: 42}
	instance := connector.Instance{ID: 7, ProjectID: 11}
	fingerprint := strings.Repeat("a", 64)
	if err := PersistLiveControlAcceptanceEvidence(context.Background(), store, instance, result, fingerprint, "dock-model", "01.00", "165-0-7", "run-1", now, time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(store.snapshots) != 1 || store.snapshots[0].CapabilityCode != "live.control" || store.snapshots[0].EvidenceLevel != "field-write" ||
		store.snapshots[0].DeviceModel != "dock-model" || store.snapshots[0].FirmwareVersion != "01.00" || store.snapshots[0].Details["cameraIndexDigest"] == "165-0-7" {
		t.Fatalf("unsafe live evidence: %#v", store.snapshots)
	}
	failed := result
	failed.Category = "remote_response_unknown"
	if err := PersistLiveControlAcceptanceEvidence(context.Background(), store, instance, failed, fingerprint, "dock-model", "01.00", "165-0-7", "run-2", now, time.Hour); !IsSafeCode(err, "acceptance_incomplete") {
		t.Fatalf("incomplete result persisted: %v", err)
	}
	if len(store.snapshots) != 1 {
		t.Fatal(errors.New("failed acceptance added evidence"))
	}
}
