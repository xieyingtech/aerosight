package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"aerosight/worker/internal/driver"
)

type DiscoveryMode string

const (
	DiscoveryPush         DiscoveryMode = "push"
	DiscoveryPoll         DiscoveryMode = "poll"
	DiscoverySubscribe    DiscoveryMode = "subscribe"
	DiscoveryManualImport DiscoveryMode = "manual-import"
)

type LeasePolicy struct {
	Duration    time.Duration
	RenewBefore time.Duration
}

type CapabilityKind string

const (
	CapabilityRead   CapabilityKind = "read"
	CapabilityAction CapabilityKind = "action"
)

type CapabilityDefinition struct {
	Code             string
	Kind             CapabilityKind
	Risk             driver.RiskLevel
	EndpointDomains  []string
	DriverCapability string
	FeatureFlag      string
	DefaultEnabled   bool
}

type Manifest struct {
	ConnectorKey      string
	Version           string
	DisplayName       string
	ConfigSchema      json.RawMessage
	CredentialSchema  json.RawMessage
	DiscoveryModes    []DiscoveryMode
	Protocols         []string
	CompatibleDrivers []string
	Capabilities      []CapabilityDefinition
	Lease             LeasePolicy
}

type Instance struct {
	ID                 int64
	ProjectID          int
	ConnectorKey       string
	Version            string
	Config             json.RawMessage
	CredentialRef      string
	CredentialEnvelope json.RawMessage
	DiscoveryScope     json.RawMessage
	LeaseOwner         string
	LeaseEpoch         int64
}

type ExternalDevice struct {
	ExternalID       string
	ExternalType     string
	ParentExternalID string
	Attributes       map[string]any
}

type DiscoveryRequest struct {
	Instance Instance
	Mode     DiscoveryMode
	Cursor   json.RawMessage
}

type DiscoveryBatch struct {
	Devices          []ExternalDevice
	Cursor           json.RawMessage
	CompleteSnapshot bool
	SourceVersion    string
}

type Health struct {
	Status  string
	Details map[string]any
}

type DiscoveryHandler func(context.Context, DiscoveryRequest) (DiscoveryBatch, error)
type HealthCheck func(context.Context, Instance) (Health, error)
type ScopeFilter func(Instance, ExternalDevice) bool

type Runtime struct {
	Manifest          Manifest
	DiscoveryHandlers map[DiscoveryMode]DiscoveryHandler
	HealthCheck       HealthCheck
	ScopeFilter       ScopeFilter
}

type Registry struct {
	mu       sync.RWMutex
	runtimes map[string]Runtime
	enabled  map[string]bool
}

var (
	connectorKeyPattern        = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)
	connectorCapabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
)

func NewRegistry() *Registry {
	return &Registry{runtimes: make(map[string]Runtime), enabled: make(map[string]bool)}
}

func runtimeKey(connectorKey, version string) string {
	return connectorKey + "@" + version
}

func validateSchema(raw json.RawMessage, field string) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s is required", field)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return fmt.Errorf("%s is not valid JSON: %w", field, err)
	}
	if schemaType, exists := schema["type"]; !exists || schemaType != "object" {
		return fmt.Errorf("%s must declare an object schema", field)
	}
	return nil
}

func ValidateManifest(manifest Manifest) error {
	if !connectorKeyPattern.MatchString(manifest.ConnectorKey) {
		return errors.New("connector key must be a stable lowercase namespace")
	}
	if _, err := driver.ParseVersion(manifest.Version); err != nil {
		return fmt.Errorf("connector version must use semantic versioning: %w", err)
	}
	if strings.TrimSpace(manifest.DisplayName) == "" {
		return errors.New("connector display name is required")
	}
	if err := validateSchema(manifest.ConfigSchema, "connector config schema"); err != nil {
		return err
	}
	if err := validateSchema(manifest.CredentialSchema, "connector credential schema"); err != nil {
		return err
	}
	if len(manifest.DiscoveryModes) == 0 {
		return errors.New("connector must declare at least one discovery mode")
	}
	modes := make(map[DiscoveryMode]struct{}, len(manifest.DiscoveryModes))
	for _, mode := range manifest.DiscoveryModes {
		switch mode {
		case DiscoveryPush, DiscoveryPoll, DiscoverySubscribe, DiscoveryManualImport:
		default:
			return fmt.Errorf("unsupported connector discovery mode %q", mode)
		}
		if _, exists := modes[mode]; exists {
			return fmt.Errorf("duplicate connector discovery mode %q", mode)
		}
		modes[mode] = struct{}{}
	}
	if err := validateUniqueStrings(manifest.Protocols, "protocol", false); err != nil {
		return err
	}
	if err := validateUniqueStrings(manifest.CompatibleDrivers, "compatible driver", true); err != nil {
		return err
	}
	seenCapabilities := make(map[string]struct{}, len(manifest.Capabilities))
	for _, capability := range manifest.Capabilities {
		if !connectorCapabilityPattern.MatchString(capability.Code) {
			return fmt.Errorf("invalid connector capability %q", capability.Code)
		}
		if _, duplicate := seenCapabilities[capability.Code]; duplicate {
			return fmt.Errorf("duplicate connector capability %q", capability.Code)
		}
		seenCapabilities[capability.Code] = struct{}{}
		if capability.Kind != CapabilityRead && capability.Kind != CapabilityAction {
			return fmt.Errorf("connector capability %q has unsupported kind %q", capability.Code, capability.Kind)
		}
		switch capability.Risk {
		case driver.RiskLow, driver.RiskMedium, driver.RiskHigh, driver.RiskCritical:
		default:
			return fmt.Errorf("connector capability %q has unsupported risk %q", capability.Code, capability.Risk)
		}
		if err := validateUniqueStrings(capability.EndpointDomains, "endpoint domain", false); err != nil {
			return fmt.Errorf("connector capability %q: %w", capability.Code, err)
		}
		if capability.DriverCapability != "" && !connectorCapabilityPattern.MatchString(capability.DriverCapability) {
			return fmt.Errorf("connector capability %q has invalid driver capability %q", capability.Code, capability.DriverCapability)
		}
		if capability.Kind == CapabilityAction {
			if !connectorCapabilityPattern.MatchString(capability.FeatureFlag) {
				return fmt.Errorf("connector action capability %q requires a stable feature flag", capability.Code)
			}
			if capability.DefaultEnabled {
				return fmt.Errorf("connector action capability %q must default closed", capability.Code)
			}
		} else if capability.FeatureFlag != "" {
			return fmt.Errorf("connector read capability %q cannot require a write feature flag", capability.Code)
		}
	}
	if manifest.Lease.Duration <= 0 {
		return errors.New("connector lease duration must be positive")
	}
	if manifest.Lease.RenewBefore <= 0 || manifest.Lease.RenewBefore >= manifest.Lease.Duration {
		return errors.New("connector lease renewal must be positive and shorter than its duration")
	}
	return nil
}

func validateUniqueStrings(values []string, field string, allowWildcard bool) error {
	if len(values) == 0 {
		return fmt.Errorf("connector must declare at least one %s", field)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || (!allowWildcard && value == "*") {
			return fmt.Errorf("connector %s cannot be empty", field)
		}
		if value != "*" && !connectorKeyPattern.MatchString(value) {
			return fmt.Errorf("invalid connector %s %q", field, value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate connector %s %q", field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateRuntime(runtime Runtime) error {
	if err := ValidateManifest(runtime.Manifest); err != nil {
		return err
	}
	if runtime.HealthCheck == nil {
		return errors.New("connector health check is required")
	}
	if runtime.ScopeFilter == nil {
		return errors.New("connector scope filter is required")
	}
	for _, mode := range runtime.Manifest.DiscoveryModes {
		if runtime.DiscoveryHandlers[mode] == nil {
			return fmt.Errorf("connector discovery mode %q has no handler", mode)
		}
	}
	return nil
}

func (registry *Registry) Register(runtime Runtime) error {
	if err := validateRuntime(runtime); err != nil {
		return err
	}
	key := runtimeKey(runtime.Manifest.ConnectorKey, runtime.Manifest.Version)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.runtimes[key]; exists {
		return fmt.Errorf("connector runtime %s is already registered", key)
	}
	registry.runtimes[key] = runtime
	registry.enabled[key] = true
	return nil
}

func (registry *Registry) Resolve(connectorKey, version string) (Runtime, error) {
	key := runtimeKey(connectorKey, version)
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	runtime, exists := registry.runtimes[key]
	if !exists {
		return Runtime{}, fmt.Errorf("connector runtime %s is not registered", key)
	}
	if !registry.enabled[key] {
		return Runtime{}, fmt.Errorf("connector runtime %s is disabled", key)
	}
	return runtime, nil
}

func (registry *Registry) SetEnabled(connectorKey, version string, enabled bool) error {
	key := runtimeKey(connectorKey, version)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.runtimes[key]; !exists {
		return fmt.Errorf("connector runtime %s is not registered", key)
	}
	registry.enabled[key] = enabled
	return nil
}

func (registry *Registry) Keys() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	keys := make([]string, 0, len(registry.runtimes))
	for key := range registry.runtimes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
