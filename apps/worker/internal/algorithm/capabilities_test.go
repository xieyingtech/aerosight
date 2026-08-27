package algorithm

import (
	"errors"
	"reflect"
	"testing"
)

func TestAllProviderProtocolsDeclareCapabilities(t *testing.T) {
	got := ListCapabilities()
	if len(got) != 4 {
		t.Fatalf("expected four registered protocols, got %d", len(got))
	}
	for _, capability := range got {
		if capability.ProviderType == "" || capability.ContractVersion != InputSchemaVersionV1 || capability.ImplementationStatus == "" {
			t.Fatalf("incomplete capability: %+v", capability)
		}
	}
}

func TestHTTPJSONCapabilityMatchesTestedContract(t *testing.T) {
	capability, err := RequireEnabled("http-json")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(capability.ExecutionModes, []string{"synchronous", "asynchronous"}) || capability.SupportsPolling || capability.SupportsSignedCallbacks {
		t.Fatalf("http-json overclaims protocol support: %+v", capability)
	}
}

func TestUnavailableAdaptersFailExplicitly(t *testing.T) {
	for _, providerType := range []string{"kserve-v2", "ogc-processes", "ai-sdk"} {
		capability, err := RequireEnabled(providerType)
		if !errors.Is(err, ErrAdapterUnavailable) || capability.ImplementationStatus != "unavailable" || capability.UnavailableReason == "" {
			t.Fatalf("%s did not fail explicitly: capability=%+v err=%v", providerType, capability, err)
		}
	}
}
