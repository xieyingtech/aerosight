package dji

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aerosight/worker/internal/device"
	"aerosight/worker/internal/driver"
)

const UnknownDeviceTypeKey = "dji.unknown"

type Compatibility struct {
	State      string
	ReadOnly   bool
	Reason     string
	Family     string
	ProductKey ProductKey
	TypeKey    string
	Firmware   string
}

func resolveFamilyProduct(family string, key ProductKey) (ProductDescriptor, bool) {
	switch family {
	case "dock2":
		return ResolveDock2Product(key)
	case "dock3":
		return ResolveDock3Product(key)
	default:
		return ProductDescriptor{}, false
	}
}

func CheckProductCompatibility(family string, key ProductKey, firmware string) Compatibility {
	result := Compatibility{State: "degraded", ReadOnly: true, Family: family, ProductKey: key, Firmware: strings.TrimSpace(firmware), TypeKey: UnknownDeviceTypeKey}
	product, exists := resolveFamilyProduct(family, key)
	if !exists {
		result.Reason = "DJI_PRODUCT_ENUM_UNKNOWN"
		return result
	}
	result.TypeKey = product.TypeKey
	if product.ValidatedFirmware == "" {
		result.State, result.ReadOnly, result.Reason = "compatible", false, ""
		return result
	}
	if result.Firmware == "" {
		result.Reason = "DJI_FIRMWARE_UNKNOWN"
		return result
	}
	if result.Firmware != product.ValidatedFirmware {
		result.Reason = "DJI_FIRMWARE_NOT_VALIDATED"
		return result
	}
	result.State, result.ReadOnly, result.Reason = "compatible", false, ""
	return result
}

func RestrictCapabilitiesForCompatibility(resolved driver.ResolvedDeviceType, compatibility Compatibility) device.CapabilityReport {
	report := device.CapabilityReport{Authoritative: true, Capabilities: make(map[string]device.CapabilityState)}
	for _, capability := range resolved.Runtime.Manifest.Capabilities {
		if _, selected := resolved.Definition.CapabilityProfile[capability.Code]; !selected {
			continue
		}
		available := !compatibility.ReadOnly || capability.Kind != driver.CapabilityCommand
		reason := ""
		if !available {
			reason = compatibility.Reason
		}
		report.Capabilities[capability.Code] = device.CapabilityState{Available: &available, Reason: reason}
	}
	return report
}

func RegisterUnknownDJIDeviceType(registry *driver.DeviceTypeRegistry) error {
	if registry == nil {
		return errors.New("DJI_DEVICE_TYPE_REGISTRY_REQUIRED")
	}
	if err := registry.Register(driver.DeviceTypeDefinition{
		TypeKey: UnknownDeviceTypeKey, Version: 1, DisplayName: "Unknown DJI device", Category: "unknown",
		DriverKey: DriverKey, DriverVersionConstraint: "^1.0.0", Status: driver.DeviceTypeActive,
		CapabilityProfile: map[string]json.RawMessage{"state.read": json.RawMessage(`{"enabled":true,"diagnosticOnly":true}`)},
	}); err != nil {
		return fmt.Errorf("register unknown DJI DeviceType: %w", err)
	}
	return nil
}
