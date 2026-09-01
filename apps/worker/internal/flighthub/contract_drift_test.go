package flighthub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type contractLock struct {
	ContractVersion string `json:"contractVersion"`
	Files           []struct {
		Path    string   `json:"path"`
		SHA256  string   `json:"sha256"`
		Domains []string `json:"domains"`
	} `json:"files"`
}

func contractRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve FlightHub contract root")
	}
	return filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2")
}

func TestFlightHubManifestAndFixturesMatchReviewedContractLock(t *testing.T) {
	t.Parallel()
	root := contractRoot(t)
	contents, err := os.ReadFile(filepath.Join(root, "contract-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock contractLock
	if json.Unmarshal(contents, &lock) != nil || lock.ContractVersion != ContractVersion || len(lock.Files) != 5 {
		t.Fatalf("invalid FlightHub contract lock: %#v", lock)
	}
	for _, file := range lock.Files {
		contents, err := os.ReadFile(filepath.Join(root, file.Path))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(contents)
		if hex.EncodeToString(digest[:]) != file.SHA256 || len(file.Domains) == 0 {
			t.Fatalf("FlightHub contract drift requires review: %s", file.Path)
		}
		if filepath.Ext(file.Path) == ".json" {
			var metadata struct {
				ContractVersion string `json:"contractVersion"`
			}
			if json.Unmarshal(contents, &metadata) != nil || metadata.ContractVersion != lock.ContractVersion {
				t.Fatalf("fixture version drift: %s", file.Path)
			}
		}
	}
}

func TestChangedMethodPathOrSchemaMarksCapabilitiesUnverifiedAndActionsClosed(t *testing.T) {
	t.Parallel()
	expected := []EndpointContractFingerprint{
		{ID: "control-status", Method: "GET", Path: "/openapi/v2.0/control/status", Domain: "control", SchemaFingerprint: "schema-v1"},
		{ID: "model-list", Method: "GET", Path: "/openapi/v2.0/model", Domain: "model", SchemaFingerprint: "schema-v1"},
	}
	for _, mutate := range []func([]EndpointContractFingerprint){
		func(current []EndpointContractFingerprint) { current[0].Method = "POST" },
		func(current []EndpointContractFingerprint) { current[0].Path += "/changed" },
		func(current []EndpointContractFingerprint) { current[0].SchemaFingerprint = "schema-v2" },
	} {
		current := append([]EndpointContractFingerprint(nil), expected...)
		mutate(current)
		drift := DetectContractDrift(expected, current)
		baseline := make([]CapabilityProbeResult, 0, len(Capabilities()))
		for _, capability := range Capabilities() {
			baseline = append(baseline, CapabilityProbeResult{
				CapabilityCode: capability.Code, Status: ProbeSupported, Reason: "previously_verified",
				Layers: CapabilityProbeLayers{Contract: ProbeSupported, Deployment: ProbeSupported, Account: ProbeSupported, Implementation: ProbeSupported, Acceptance: ProbeSupported},
			})
		}
		effective := ApplyContractDrift(baseline, drift)
		control := probeResult(t, effective, "device.control")
		if control.Status != ProbeUnverified || control.Layers.Contract != ProbeUnverified || control.Layers.Acceptance != ProbeUnverified {
			t.Fatalf("contract drift left high-risk action open: %#v drift=%#v", control, drift)
		}
		if model := probeResult(t, effective, "model.read"); model.Status != ProbeSupported {
			t.Fatalf("unrelated domain was narrowed: %#v", model)
		}
	}
}
