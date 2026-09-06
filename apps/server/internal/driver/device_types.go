package driver

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
)

type DeviceTypeStatus string

const (
	DeviceTypeActive  DeviceTypeStatus = "active"
	DeviceTypeRetired DeviceTypeStatus = "retired"
)

type DeviceTypeDefinition struct {
	TypeKey                 string                     `json:"typeKey"`
	Version                 int                        `json:"version"`
	DisplayName             string                     `json:"displayName"`
	Category                string                     `json:"category"`
	DriverKey               string                     `json:"driverKey"`
	DriverVersionConstraint string                     `json:"driverVersionConstraint"`
	CapabilityProfile       map[string]json.RawMessage `json:"capabilityProfile"`
	Status                  DeviceTypeStatus           `json:"status"`
}

type ResolvedDeviceType struct {
	Definition DeviceTypeDefinition
	Runtime    Runtime
	Available  bool
	ReadOnly   bool
	Reason     string
}

type DeviceTypeRegistry struct {
	mu      sync.RWMutex
	drivers *Registry
	types   map[string]DeviceTypeDefinition
}

var typeKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

func NewDeviceTypeRegistry(drivers *Registry) *DeviceTypeRegistry {
	return &DeviceTypeRegistry{drivers: drivers, types: make(map[string]DeviceTypeDefinition)}
}

func deviceTypeKey(typeKey string, version int) string {
	return fmt.Sprintf("%s@%d", typeKey, version)
}

func validateDeviceType(definition DeviceTypeDefinition, runtime Runtime) error {
	if !typeKeyPattern.MatchString(definition.TypeKey) {
		return errors.New("device type key must be a stable lowercase namespace")
	}
	if definition.Version <= 0 || definition.DisplayName == "" || definition.Category == "" {
		return errors.New("device type version, display name, and category are required")
	}
	if definition.Status != DeviceTypeActive && definition.Status != DeviceTypeRetired {
		return fmt.Errorf("unsupported device type status %q", definition.Status)
	}
	known := make(map[string]struct{}, len(runtime.Manifest.Capabilities))
	for _, capability := range runtime.Manifest.Capabilities {
		known[capability.Code] = struct{}{}
	}
	for code, profile := range definition.CapabilityProfile {
		if _, exists := known[code]; !exists {
			return fmt.Errorf("device type capability %q is not declared by driver", code)
		}
		if err := validateJSONObject(profile, "device type capability profile"); err != nil {
			return fmt.Errorf("device type capability %q: %w", code, err)
		}
	}
	return nil
}

func (registry *DeviceTypeRegistry) Register(definition DeviceTypeDefinition) error {
	runtime, err := registry.drivers.ResolveCompatible(definition.DriverKey, definition.DriverVersionConstraint)
	if err != nil {
		return fmt.Errorf("device type driver is unavailable: %w", err)
	}
	if err := validateDeviceType(definition, runtime); err != nil {
		return err
	}
	key := deviceTypeKey(definition.TypeKey, definition.Version)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.types[key]; exists {
		return fmt.Errorf("device type %s is already registered", key)
	}
	registry.types[key] = definition
	return nil
}

func (registry *DeviceTypeRegistry) Resolve(typeKey string, version int) ResolvedDeviceType {
	key := deviceTypeKey(typeKey, version)
	registry.mu.RLock()
	definition, exists := registry.types[key]
	registry.mu.RUnlock()
	if !exists {
		return ResolvedDeviceType{Available: false, ReadOnly: true, Reason: "DEVICE_TYPE_NOT_REGISTERED"}
	}
	runtime, err := registry.drivers.ResolveCompatible(definition.DriverKey, definition.DriverVersionConstraint)
	if err != nil {
		return ResolvedDeviceType{Definition: definition, Available: false, ReadOnly: true, Reason: "DEVICE_DRIVER_UNAVAILABLE"}
	}
	if definition.Status == DeviceTypeRetired {
		return ResolvedDeviceType{Definition: definition, Runtime: runtime, Available: false, ReadOnly: true, Reason: "DEVICE_TYPE_RETIRED"}
	}
	return ResolvedDeviceType{Definition: definition, Runtime: runtime, Available: true}
}

func (registry *DeviceTypeRegistry) Keys() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	keys := make([]string, 0, len(registry.types))
	for key := range registry.types {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
