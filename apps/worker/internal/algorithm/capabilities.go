package algorithm

import (
	"errors"
	"fmt"
)

var ErrAdapterUnavailable = errors.New("algorithm adapter is unavailable")

type Capability struct {
	ProviderType            string
	ImplementationStatus    string
	ExecutionModes          []string
	SupportsPolling         bool
	SupportsSignedCallbacks bool
	ContractVersion         string
	UnavailableReason       string
}

var capabilities = map[string]Capability{
	"http-json": {
		ProviderType: "http-json", ImplementationStatus: "enabled",
		ExecutionModes:  []string{"synchronous", "asynchronous"},
		ContractVersion: InputSchemaVersionV1,
	},
	"kserve-v2": {
		ProviderType: "kserve-v2", ImplementationStatus: "unavailable",
		ContractVersion: InputSchemaVersionV1, UnavailableReason: "KServe V2 adapter is not enabled in this release",
	},
	"ogc-processes": {
		ProviderType: "ogc-processes", ImplementationStatus: "unavailable",
		ContractVersion: InputSchemaVersionV1, UnavailableReason: "OGC API Processes adapter is not enabled in this release",
	},
	"ai-sdk": {
		ProviderType: "ai-sdk", ImplementationStatus: "unavailable",
		ContractVersion: InputSchemaVersionV1, UnavailableReason: "AI SDK adapter is not enabled in this release",
	},
}

func ListCapabilities() []Capability {
	result := make([]Capability, 0, len(capabilities))
	for _, providerType := range []string{"http-json", "kserve-v2", "ogc-processes", "ai-sdk"} {
		result = append(result, capabilities[providerType])
	}
	return result
}

func CapabilityFor(providerType string) (Capability, error) {
	capability, ok := capabilities[providerType]
	if !ok {
		return Capability{}, fmt.Errorf("unknown algorithm provider type %q", providerType)
	}
	return capability, nil
}

func RequireEnabled(providerType string) (Capability, error) {
	capability, err := CapabilityFor(providerType)
	if err != nil {
		return Capability{}, err
	}
	if capability.ImplementationStatus != "enabled" {
		return capability, fmt.Errorf("%w: %s: %s", ErrAdapterUnavailable, providerType, capability.UnavailableReason)
	}
	return capability, nil
}
