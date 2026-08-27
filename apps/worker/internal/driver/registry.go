package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type CapabilityKind string

const (
	CapabilityRead    CapabilityKind = "read"
	CapabilityCommand CapabilityKind = "command"
	CapabilityStream  CapabilityKind = "stream"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type StreamDataType string

const (
	StreamVideo     StreamDataType = "video"
	StreamAudio     StreamDataType = "audio"
	StreamTelemetry StreamDataType = "telemetry"
	StreamSensor    StreamDataType = "sensor"
	StreamEvents    StreamDataType = "events"
)

type CapabilityDefinition struct {
	Code         string          `json:"code"`
	Kind         CapabilityKind  `json:"kind"`
	Risk         RiskLevel       `json:"risk"`
	InputSchema  json.RawMessage `json:"inputSchema,omitempty"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
}

type StreamDefinition struct {
	ChannelKey     string          `json:"channelKey"`
	CapabilityCode string          `json:"capabilityCode"`
	DataType       StreamDataType  `json:"dataType"`
	Schema         json.RawMessage `json:"schema,omitempty"`
	Unit           string          `json:"unit,omitempty"`
}

type Manifest struct {
	DriverKey    string                 `json:"driverKey"`
	Version      string                 `json:"version"`
	DisplayName  string                 `json:"displayName"`
	Protocols    []string               `json:"protocols"`
	Capabilities []CapabilityDefinition `json:"capabilities"`
	Streams      []StreamDefinition     `json:"streams"`
}

type AdapterConfig struct {
	AdapterID int64
	ProjectID int
	Config    json.RawMessage
}

type Command struct {
	CapabilityCode string
	Payload        json.RawMessage
}

type CommandResult struct {
	Payload json.RawMessage
}

type StreamRequest struct {
	CapabilityCode string
	ChannelKey     string
}

type ProtocolHandler func(context.Context, AdapterConfig) error
type CommandHandler func(context.Context, Command) (CommandResult, error)
type StreamHandler func(context.Context, StreamRequest) error

type Runtime struct {
	Manifest         Manifest
	ProtocolHandlers map[string]ProtocolHandler
	CommandHandlers  map[string]CommandHandler
	StreamHandlers   map[string]StreamHandler
}

type Registry struct {
	mu       sync.RWMutex
	runtimes map[string]Runtime
	enabled  map[string]bool
}

var (
	driverKeyPattern  = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)
	capabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
	semverPattern     = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)

func NewRegistry() *Registry {
	return &Registry{runtimes: make(map[string]Runtime), enabled: make(map[string]bool)}
}

func runtimeKey(driverKey, version string) string {
	return driverKey + "@" + version
}

func validateJSONObject(raw json.RawMessage, field string) error {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("%s is not valid JSON: %w", field, err)
	}
	if _, ok := value.(map[string]any); !ok {
		return fmt.Errorf("%s must be a JSON object", field)
	}
	return nil
}

func ValidateManifest(manifest Manifest) error {
	if !driverKeyPattern.MatchString(manifest.DriverKey) {
		return errors.New("driver key must be a stable lowercase namespace")
	}
	if !semverPattern.MatchString(manifest.Version) {
		return errors.New("driver version must be semantic versioning")
	}
	if strings.TrimSpace(manifest.DisplayName) == "" {
		return errors.New("driver display name is required")
	}
	if len(manifest.Protocols) == 0 {
		return errors.New("driver must declare at least one protocol")
	}

	protocols := make(map[string]struct{}, len(manifest.Protocols))
	for _, protocol := range manifest.Protocols {
		protocol = strings.TrimSpace(protocol)
		if protocol == "" {
			return errors.New("driver protocol cannot be empty")
		}
		if _, exists := protocols[protocol]; exists {
			return fmt.Errorf("duplicate driver protocol %q", protocol)
		}
		protocols[protocol] = struct{}{}
	}

	capabilities := make(map[string]CapabilityDefinition, len(manifest.Capabilities))
	for _, capability := range manifest.Capabilities {
		if !capabilityPattern.MatchString(capability.Code) {
			return fmt.Errorf("invalid capability code %q", capability.Code)
		}
		if _, exists := capabilities[capability.Code]; exists {
			return fmt.Errorf("duplicate capability %q", capability.Code)
		}
		switch capability.Kind {
		case CapabilityRead, CapabilityCommand, CapabilityStream:
		default:
			return fmt.Errorf("capability %q has unsupported kind %q", capability.Code, capability.Kind)
		}
		switch capability.Risk {
		case RiskLow, RiskMedium, RiskHigh, RiskCritical:
		default:
			return fmt.Errorf("capability %q has unsupported risk %q", capability.Code, capability.Risk)
		}
		if err := validateJSONObject(capability.InputSchema, "capability input schema"); err != nil {
			return fmt.Errorf("capability %q: %w", capability.Code, err)
		}
		if err := validateJSONObject(capability.OutputSchema, "capability output schema"); err != nil {
			return fmt.Errorf("capability %q: %w", capability.Code, err)
		}
		capabilities[capability.Code] = capability
	}

	channels := make(map[string]struct{}, len(manifest.Streams))
	for _, stream := range manifest.Streams {
		if !capabilityPattern.MatchString(stream.ChannelKey) {
			return fmt.Errorf("invalid stream channel key %q", stream.ChannelKey)
		}
		if _, exists := channels[stream.ChannelKey]; exists {
			return fmt.Errorf("duplicate stream channel %q", stream.ChannelKey)
		}
		capability, exists := capabilities[stream.CapabilityCode]
		if !exists || capability.Kind != CapabilityStream {
			return fmt.Errorf("stream %q references a non-stream capability %q", stream.ChannelKey, stream.CapabilityCode)
		}
		switch stream.DataType {
		case StreamVideo, StreamAudio, StreamTelemetry, StreamSensor, StreamEvents:
		default:
			return fmt.Errorf("stream %q has unsupported data type %q", stream.ChannelKey, stream.DataType)
		}
		if err := validateJSONObject(stream.Schema, "stream schema"); err != nil {
			return fmt.Errorf("stream %q: %w", stream.ChannelKey, err)
		}
		channels[stream.ChannelKey] = struct{}{}
	}
	return nil
}

func validateRuntime(runtime Runtime) error {
	if err := ValidateManifest(runtime.Manifest); err != nil {
		return err
	}
	for _, protocol := range runtime.Manifest.Protocols {
		if runtime.ProtocolHandlers[protocol] == nil {
			return fmt.Errorf("driver protocol %q has no handler", protocol)
		}
	}
	for _, capability := range runtime.Manifest.Capabilities {
		if capability.Kind == CapabilityCommand && runtime.CommandHandlers[capability.Code] == nil {
			return fmt.Errorf("command capability %q has no handler", capability.Code)
		}
	}
	for _, stream := range runtime.Manifest.Streams {
		if runtime.StreamHandlers[stream.ChannelKey] == nil {
			return fmt.Errorf("stream channel %q has no handler", stream.ChannelKey)
		}
	}
	return nil
}

func (registry *Registry) Register(runtime Runtime) error {
	if err := validateRuntime(runtime); err != nil {
		return err
	}
	key := runtimeKey(runtime.Manifest.DriverKey, runtime.Manifest.Version)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.runtimes[key]; exists {
		return fmt.Errorf("driver runtime %s is already registered", key)
	}
	registry.runtimes[key] = runtime
	registry.enabled[key] = true
	return nil
}

func (registry *Registry) Resolve(driverKey, version string) (Runtime, error) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	runtime, exists := registry.runtimes[runtimeKey(driverKey, version)]
	if !exists {
		return Runtime{}, fmt.Errorf("driver runtime %s is not registered", runtimeKey(driverKey, version))
	}
	if !registry.enabled[runtimeKey(driverKey, version)] {
		return Runtime{}, fmt.Errorf("driver runtime %s is disabled", runtimeKey(driverKey, version))
	}
	return runtime, nil
}

func (registry *Registry) SetEnabled(driverKey, version string, enabled bool) error {
	key := runtimeKey(driverKey, version)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.runtimes[key]; !exists {
		return fmt.Errorf("driver runtime %s is not registered", key)
	}
	registry.enabled[key] = enabled
	return nil
}

func (registry *Registry) ResolveCompatible(driverKey, constraint string) (Runtime, error) {
	rangeRule, err := ParseVersionConstraint(constraint)
	if err != nil {
		return Runtime{}, err
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	var selected Runtime
	var selectedVersion Version
	found := false
	for key, runtime := range registry.runtimes {
		if runtime.Manifest.DriverKey != driverKey || !registry.enabled[key] {
			continue
		}
		version, parseErr := ParseVersion(runtime.Manifest.Version)
		if parseErr != nil || !rangeRule.Matches(version) {
			continue
		}
		if !found || version.Compare(selectedVersion) > 0 {
			selected = runtime
			selectedVersion = version
			found = true
		}
	}
	if !found {
		return Runtime{}, fmt.Errorf("no enabled driver runtime %s matches %q", driverKey, constraint)
	}
	return selected, nil
}

func (registry *Registry) ResolveCapability(driverKey, version, capabilityCode string) (CapabilityDefinition, error) {
	runtime, err := registry.Resolve(driverKey, version)
	if err != nil {
		return CapabilityDefinition{}, err
	}
	for _, capability := range runtime.Manifest.Capabilities {
		if capability.Code == capabilityCode {
			return capability, nil
		}
	}
	return CapabilityDefinition{}, fmt.Errorf("driver %s does not declare capability %q", runtimeKey(driverKey, version), capabilityCode)
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
