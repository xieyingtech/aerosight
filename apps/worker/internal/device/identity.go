package device

import (
	"errors"
	"fmt"
	"sync"

	"aerosight/worker/internal/driver"
)

var (
	ErrIdentityConflict   = errors.New("external identity is already claimed by another device")
	ErrAdapterUnavailable = errors.New("adapter is unavailable or incompatible")
)

type AdapterRegistration struct {
	ID            int64
	ProjectID     int
	DriverKey     string
	DriverVersion string
	Enabled       bool
}

type IdentityClaim struct {
	ProjectID       int
	AdapterID       int64
	ExternalID      string
	RequestedDevice string
	Type            TypeReference
	Class           Class
	DisplayName     string
	GatewayDeviceID string
}

type BoundIdentity struct {
	Device      Device
	DriverKey   string
	ExternalID  string
	GatewayID   string
	AdapterID   int64
	AdapterSeen []int64
}

type DeviceRoute struct {
	ProjectID  int
	AdapterID  int64
	DeviceID   string
	ExternalID string
}

// IdentityRegistry models the transactional rules used by persistence:
// a vendor identity is stable within project + Driver, while Adapter is a
// replaceable connection instance and gateway routes may be inherited.
type IdentityRegistry struct {
	mu         sync.RWMutex
	types      TypeResolver
	nextID     int64
	adapters   map[int64]AdapterRegistration
	identities map[string]BoundIdentity
	byDevice   map[string]string
}

func NewIdentityRegistry(types TypeResolver) *IdentityRegistry {
	return &IdentityRegistry{
		types: types, nextID: 1, adapters: make(map[int64]AdapterRegistration),
		identities: make(map[string]BoundIdentity), byDevice: make(map[string]string),
	}
}

func externalIdentityKey(projectID int, driverKey, externalID string) string {
	return fmt.Sprintf("%d\x00%s\x00%s", projectID, driverKey, externalID)
}

func deviceScopeKey(projectID int, deviceID string) string {
	return fmt.Sprintf("%d\x00%s", projectID, deviceID)
}

func (registry *IdentityRegistry) RegisterAdapter(adapter AdapterRegistration) error {
	if adapter.ID <= 0 || adapter.ProjectID <= 0 || adapter.DriverKey == "" {
		return errors.New("adapter identity, project, and driver are required")
	}
	if _, err := driver.ParseVersion(adapter.DriverVersion); err != nil {
		return fmt.Errorf("adapter driver version: %w", err)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if existing, exists := registry.adapters[adapter.ID]; exists && (existing.ProjectID != adapter.ProjectID || existing.DriverKey != adapter.DriverKey) {
		return errors.New("adapter identity cannot change project or driver")
	}
	for _, identity := range registry.identities {
		if identity.AdapterID != adapter.ID || !adapter.Enabled {
			continue
		}
		if err := compatibleAdapter(registry.types, identity.Device.Type, adapter); err != nil {
			return fmt.Errorf("adapter upgrade would strand device %q: %w", identity.Device.ID, err)
		}
	}
	registry.adapters[adapter.ID] = adapter
	return nil
}

func compatibleAdapter(types TypeResolver, typeReference TypeReference, adapter AdapterRegistration) error {
	if !adapter.Enabled {
		return ErrAdapterUnavailable
	}
	resolved := types.Resolve(typeReference.Key, typeReference.Version)
	if resolved.Definition.TypeKey == "" || resolved.Definition.DriverKey != adapter.DriverKey {
		return ErrAdapterUnavailable
	}
	rule, err := driver.ParseVersionConstraint(resolved.Definition.DriverVersionConstraint)
	if err != nil {
		return err
	}
	version, err := driver.ParseVersion(adapter.DriverVersion)
	if err != nil || !rule.Matches(version) {
		return ErrAdapterUnavailable
	}
	return nil
}

func (registry *IdentityRegistry) Claim(claim IdentityClaim) (BoundIdentity, bool, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	adapter, exists := registry.adapters[claim.AdapterID]
	if !exists || adapter.ProjectID != claim.ProjectID {
		return BoundIdentity{}, false, ErrAdapterUnavailable
	}
	if claim.ExternalID == "" || claim.DisplayName == "" {
		return BoundIdentity{}, false, errors.New("external identity and display name are required")
	}
	if err := compatibleAdapter(registry.types, claim.Type, adapter); err != nil {
		return BoundIdentity{}, false, err
	}
	key := externalIdentityKey(claim.ProjectID, adapter.DriverKey, claim.ExternalID)
	if existing, claimed := registry.identities[key]; claimed {
		if claim.RequestedDevice != "" && claim.RequestedDevice != existing.Device.ID {
			return BoundIdentity{}, false, ErrIdentityConflict
		}
		if existing.Device.Type != claim.Type || existing.Device.Class != claim.Class {
			return BoundIdentity{}, false, ErrIdentityConflict
		}
		if existing.AdapterID != claim.AdapterID {
			existing.AdapterID = claim.AdapterID
			existing.AdapterSeen = appendAdapterOnce(existing.AdapterSeen, claim.AdapterID)
			if existing.GatewayID == "" {
				existing.Device.AdapterID = claim.AdapterID
			}
			registry.identities[key] = existing
		}
		return existing, true, nil
	}

	deviceID := claim.RequestedDevice
	if deviceID == "" {
		deviceID = fmt.Sprintf("device-%d", registry.nextID)
		registry.nextID++
	}
	deviceKey := deviceScopeKey(claim.ProjectID, deviceID)
	if _, claimed := registry.byDevice[deviceKey]; claimed {
		return BoundIdentity{}, false, ErrIdentityConflict
	}
	if claim.GatewayDeviceID != "" {
		gatewayKey, claimed := registry.byDevice[deviceScopeKey(claim.ProjectID, claim.GatewayDeviceID)]
		if !claimed || registry.identities[gatewayKey].AdapterID != claim.AdapterID {
			return BoundIdentity{}, false, errors.New("gateway must be a claimed device on the same adapter")
		}
	}
	bound := BoundIdentity{
		Device: Device{
			ID: deviceID, ProjectID: claim.ProjectID, Type: claim.Type, Class: claim.Class,
			DisplayName: claim.DisplayName,
		},
		DriverKey: adapter.DriverKey, ExternalID: claim.ExternalID, GatewayID: claim.GatewayDeviceID,
		AdapterID: claim.AdapterID, AdapterSeen: []int64{claim.AdapterID},
	}
	if claim.GatewayDeviceID == "" {
		bound.Device.AdapterID = claim.AdapterID
	}
	registry.identities[key] = bound
	registry.byDevice[deviceKey] = key
	return bound, false, nil
}

func appendAdapterOnce(history []int64, adapterID int64) []int64 {
	for _, existing := range history {
		if existing == adapterID {
			return history
		}
	}
	return append(history, adapterID)
}

func (registry *IdentityRegistry) Rebind(projectID int, deviceID string, adapterID int64) (BoundIdentity, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	key, claimed := registry.byDevice[deviceScopeKey(projectID, deviceID)]
	if !claimed {
		return BoundIdentity{}, errors.New("device identity is not claimed")
	}
	identity := registry.identities[key]
	adapter, exists := registry.adapters[adapterID]
	if !exists || adapter.ProjectID != projectID || adapter.DriverKey != identity.DriverKey {
		return BoundIdentity{}, ErrAdapterUnavailable
	}
	if err := compatibleAdapter(registry.types, identity.Device.Type, adapter); err != nil {
		return BoundIdentity{}, err
	}
	identity.AdapterID = adapterID
	identity.AdapterSeen = appendAdapterOnce(identity.AdapterSeen, adapterID)
	if identity.GatewayID == "" {
		identity.Device.AdapterID = adapterID
	}
	registry.identities[key] = identity
	return identity, nil
}

func (registry *IdentityRegistry) ResolveRoute(projectID int, deviceID string) (DeviceRoute, error) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return registry.resolveRoute(projectID, deviceID, map[string]struct{}{})
}

func (registry *IdentityRegistry) resolveRoute(projectID int, deviceID string, seen map[string]struct{}) (DeviceRoute, error) {
	deviceKey := deviceScopeKey(projectID, deviceID)
	if _, cycle := seen[deviceKey]; cycle {
		return DeviceRoute{}, errors.New("device gateway route contains a cycle")
	}
	seen[deviceKey] = struct{}{}
	key, claimed := registry.byDevice[deviceKey]
	if !claimed {
		return DeviceRoute{}, errors.New("device identity is not claimed")
	}
	identity := registry.identities[key]
	adapterID := identity.AdapterID
	if identity.GatewayID != "" {
		gateway, err := registry.resolveRoute(projectID, identity.GatewayID, seen)
		if err != nil {
			return DeviceRoute{}, err
		}
		adapterID = gateway.AdapterID
	}
	adapter, exists := registry.adapters[adapterID]
	if !exists || !adapter.Enabled {
		return DeviceRoute{}, ErrAdapterUnavailable
	}
	return DeviceRoute{ProjectID: projectID, AdapterID: adapterID, DeviceID: deviceID, ExternalID: identity.ExternalID}, nil
}
