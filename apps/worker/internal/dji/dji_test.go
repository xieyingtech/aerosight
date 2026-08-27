package dji

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCallbackSignatureRejectsForgeryReplayAndExpiredTimestamp(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	body := []byte(`{"event":"device_online","sn":"SANDBOX-UAV-001"}`)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	secret := []byte("fixture-only-secret")
	signature := SignCallback(secret, timestamp, "nonce-1", body)
	store := NewMemoryNonceStore()
	if err := VerifyCallback(secret, timestamp, "nonce-1", signature, body, now, store); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(VerifyCallback(secret, timestamp, "nonce-1", signature, body, now, store), ErrReplayedCallback) {
		t.Fatal("replayed callback accepted")
	}
	if !errors.Is(VerifyCallback(secret, timestamp, "nonce-2", signature, body, now, NewMemoryNonceStore()), ErrInvalidSignature) {
		t.Fatal("forged nonce accepted")
	}
	expired := strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10)
	if !errors.Is(VerifyCallback(secret, expired, "nonce-3", SignCallback(secret, expired, "nonce-3", body), body, now, NewMemoryNonceStore()), ErrExpiredCallback) {
		t.Fatal("expired callback accepted")
	}
}

func TestRedactedTopologyMapsDockAircraftAndCapabilities(t *testing.T) {
	payload, err := os.ReadFile("../../../../test/fixtures/dji-topology-redacted.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "tenant") || strings.Contains(string(payload), "secret") {
		t.Fatal("fixture contains tenant credentials")
	}
	discoveries, err := MapTopology(17, 8, payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(discoveries) != 2 || discoveries[0].ExternalDeviceType != "dock" || discoveries[1].ExternalDeviceType != "drone" {
		t.Fatalf("unexpected topology: %+v", discoveries)
	}
	capabilities := discoveries[1].Identity["capabilities"].([]string)
	expected := map[string]bool{"flight.route": true, "flight.return_home": true, "camera.photo": true, "live.video": true}
	for _, capability := range capabilities {
		delete(expected, capability)
	}
	if len(expected) != 0 {
		t.Fatalf("missing capabilities: %v", expected)
	}
	if discoveries[1].Identity["parentExternalDeviceId"] != "SANDBOX-DOCK-001" {
		t.Fatal("aircraft topology parent was lost")
	}
}
