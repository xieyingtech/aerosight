package flighthub

import "sort"

type EndpointContractFingerprint struct {
	ID                string
	Method            string
	Path              string
	Domain            string
	SchemaFingerprint string
}

type ContractDrift struct {
	EndpointIDs     []string
	Domains         []string
	CapabilityCodes []string
}

func DetectContractDrift(expected, current []EndpointContractFingerprint) ContractDrift {
	expectedByID := make(map[string]EndpointContractFingerprint, len(expected))
	currentByID := make(map[string]EndpointContractFingerprint, len(current))
	for _, endpoint := range expected {
		expectedByID[endpoint.ID] = endpoint
	}
	for _, endpoint := range current {
		currentByID[endpoint.ID] = endpoint
	}
	domainSet := map[string]struct{}{}
	endpointSet := map[string]struct{}{}
	for id, baseline := range expectedByID {
		observed, exists := currentByID[id]
		if exists && baseline.Method == observed.Method && baseline.Path == observed.Path && baseline.Domain == observed.Domain && baseline.SchemaFingerprint == observed.SchemaFingerprint {
			continue
		}
		endpointSet[id] = struct{}{}
		domainSet[baseline.Domain] = struct{}{}
		if exists {
			domainSet[observed.Domain] = struct{}{}
		}
	}
	for id, endpoint := range currentByID {
		if _, exists := expectedByID[id]; !exists {
			endpointSet[id] = struct{}{}
			domainSet[endpoint.Domain] = struct{}{}
		}
	}
	capabilitySet := map[string]struct{}{}
	for _, capability := range Capabilities() {
		for _, domain := range capability.EndpointDomains {
			if _, changed := domainSet[domain]; changed {
				capabilitySet[capability.Code] = struct{}{}
				break
			}
		}
	}
	return ContractDrift{
		EndpointIDs: sortedKeys(endpointSet), Domains: sortedKeys(domainSet), CapabilityCodes: sortedKeys(capabilitySet),
	}
}

func ApplyContractDrift(results []CapabilityProbeResult, drift ContractDrift) []CapabilityProbeResult {
	affected := make(map[string]struct{}, len(drift.CapabilityCodes))
	for _, code := range drift.CapabilityCodes {
		affected[code] = struct{}{}
	}
	effective := append([]CapabilityProbeResult(nil), results...)
	for index := range effective {
		if _, changed := affected[effective[index].CapabilityCode]; !changed {
			continue
		}
		effective[index].Layers.Contract = ProbeUnverified
		effective[index].Layers.Acceptance = ProbeUnverified
		effective[index].Status = ProbeUnverified
		effective[index].Reason = "contract_drift_pending_verification"
	}
	return effective
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
