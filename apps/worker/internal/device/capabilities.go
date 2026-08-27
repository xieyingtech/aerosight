package device

import (
	"encoding/json"
	"fmt"
	"sort"

	"aerosight/worker/internal/driver"
)

type CapabilityState struct {
	Available  *bool
	Reason     string
	Parameters map[string]any
}

// CapabilityReport is a narrowing observation from firmware or current
// runtime state. When Authoritative is true, omitted known capabilities are
// retained in the catalog but made unavailable.
type CapabilityReport struct {
	Authoritative bool
	Capabilities  map[string]CapabilityState
}

type EffectiveCapability struct {
	Code         string
	Kind         driver.CapabilityKind
	Risk         driver.RiskLevel
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
	Parameters   map[string]any
	Available    bool
	Reason       string
}

type EffectiveStream struct {
	ChannelKey     string
	CapabilityCode string
	DataType       driver.StreamDataType
	Schema         json.RawMessage
	Unit           string
	Available      bool
	Reason         string
}

type UnknownCapability struct {
	Source string
	Code   string
}

type EffectiveCapabilities struct {
	Capabilities []EffectiveCapability
	Streams      []EffectiveStream
	Quarantined  []UnknownCapability
}

func capabilityProfile(raw json.RawMessage) (bool, map[string]any, error) {
	profile := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &profile); err != nil {
			return false, nil, fmt.Errorf("decode device type capability profile: %w", err)
		}
	}
	enabled := true
	if value, exists := profile["enabled"]; exists {
		parsed, ok := value.(bool)
		if !ok {
			return false, nil, fmt.Errorf("device type capability enabled flag must be boolean")
		}
		enabled = parsed
		delete(profile, "enabled")
	}
	return enabled, profile, nil
}

func copyParameters(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func applyCapabilityState(capability *EffectiveCapability, state CapabilityState, fallbackReason string) {
	if state.Available != nil && !*state.Available {
		capability.Available = false
		if state.Reason != "" {
			capability.Reason = state.Reason
		} else if capability.Reason == "" {
			capability.Reason = fallbackReason
		}
	}
	for key, value := range state.Parameters {
		capability.Parameters[key] = value
	}
}

func reportUnknown(source string, report CapabilityReport, known map[string]struct{}, result *EffectiveCapabilities) {
	for code := range report.Capabilities {
		if _, exists := known[code]; !exists {
			result.Quarantined = append(result.Quarantined, UnknownCapability{Source: source, Code: code})
		}
	}
}

func CalculateEffectiveCapabilities(deviceType driver.ResolvedDeviceType, firmware, runtime CapabilityReport) (EffectiveCapabilities, error) {
	result := EffectiveCapabilities{}
	known := make(map[string]struct{}, len(deviceType.Runtime.Manifest.Capabilities))
	for _, definition := range deviceType.Runtime.Manifest.Capabilities {
		known[definition.Code] = struct{}{}
	}
	reportUnknown("firmware", firmware, known, &result)
	reportUnknown("runtime", runtime, known, &result)

	byCode := make(map[string]EffectiveCapability)
	for _, definition := range deviceType.Runtime.Manifest.Capabilities {
		profile, selected := deviceType.Definition.CapabilityProfile[definition.Code]
		if !selected {
			continue
		}
		enabled, parameters, err := capabilityProfile(profile)
		if err != nil {
			return EffectiveCapabilities{}, fmt.Errorf("capability %q: %w", definition.Code, err)
		}
		effective := EffectiveCapability{
			Code: definition.Code, Kind: definition.Kind, Risk: definition.Risk,
			InputSchema: definition.InputSchema, OutputSchema: definition.OutputSchema,
			Parameters: copyParameters(parameters), Available: enabled,
		}
		if !enabled {
			effective.Reason = "DEVICE_TYPE_CAPABILITY_DISABLED"
		}
		if !deviceType.Available {
			effective.Available = false
			effective.Reason = deviceType.Reason
		}
		firmwareState, firmwareReported := firmware.Capabilities[definition.Code]
		if firmwareReported {
			applyCapabilityState(&effective, firmwareState, "FIRMWARE_CAPABILITY_UNAVAILABLE")
		} else if firmware.Authoritative {
			effective.Available = false
			effective.Reason = "FIRMWARE_CAPABILITY_NOT_REPORTED"
		}
		runtimeState, runtimeReported := runtime.Capabilities[definition.Code]
		if runtimeReported {
			applyCapabilityState(&effective, runtimeState, "RUNTIME_CAPABILITY_UNAVAILABLE")
		} else if runtime.Authoritative {
			effective.Available = false
			effective.Reason = "RUNTIME_CAPABILITY_NOT_REPORTED"
		}
		byCode[definition.Code] = effective
	}

	for _, capability := range byCode {
		result.Capabilities = append(result.Capabilities, capability)
	}
	sort.Slice(result.Capabilities, func(i, j int) bool { return result.Capabilities[i].Code < result.Capabilities[j].Code })
	for _, stream := range deviceType.Runtime.Manifest.Streams {
		capability, selected := byCode[stream.CapabilityCode]
		if !selected {
			continue
		}
		result.Streams = append(result.Streams, EffectiveStream{
			ChannelKey: stream.ChannelKey, CapabilityCode: stream.CapabilityCode, DataType: stream.DataType,
			Schema: stream.Schema, Unit: stream.Unit, Available: capability.Available, Reason: capability.Reason,
		})
	}
	sort.Slice(result.Streams, func(i, j int) bool { return result.Streams[i].ChannelKey < result.Streams[j].ChannelKey })
	sort.Slice(result.Quarantined, func(i, j int) bool {
		if result.Quarantined[i].Source == result.Quarantined[j].Source {
			return result.Quarantined[i].Code < result.Quarantined[j].Code
		}
		return result.Quarantined[i].Source < result.Quarantined[j].Source
	})
	return result, nil
}
