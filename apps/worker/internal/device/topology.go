package device

import (
	"errors"
	"fmt"
	"sort"

	"aerosight/worker/internal/driver"
)

// Class describes an independently identifiable hardware role. It does not
// create a separate persistence model: every class is represented by Device.
type Class string

const (
	ClassDock     Class = "dock"
	ClassAircraft Class = "aircraft"
	ClassRobot    Class = "robot"
	ClassCamera   Class = "camera"
	ClassSensor   Class = "sensor"
	ClassGateway  Class = "gateway"
)

type TypeReference struct {
	Key     string
	Version int
}

type Device struct {
	ID          string
	ProjectID   int
	Type        TypeReference
	Class       Class
	DisplayName string
	AdapterID   int64
}

type RelationshipKind string

const (
	RelationshipGatewayFor     RelationshipKind = "gateway-for"
	RelationshipDockedAircraft RelationshipKind = "docked-aircraft"
	RelationshipMountedOn      RelationshipKind = "mounted-on"
	RelationshipContains       RelationshipKind = "contains"
)

type Relationship struct {
	ProjectID int
	FromID    string
	ToID      string
	Kind      RelationshipKind
}

type TypeResolver interface {
	Resolve(typeKey string, version int) driver.ResolvedDeviceType
}

// Topology is a graph containing only Device nodes. Cameras, sensors and other
// attached hardware stay first-class devices; attachment and routing are edges.
type Topology struct {
	devices       map[string]Device
	relationships []Relationship
}

var validClasses = map[Class]struct{}{
	ClassDock: {}, ClassAircraft: {}, ClassRobot: {}, ClassCamera: {}, ClassSensor: {}, ClassGateway: {},
}

var validRelationshipKinds = map[RelationshipKind]struct{}{
	RelationshipGatewayFor: {}, RelationshipDockedAircraft: {}, RelationshipMountedOn: {}, RelationshipContains: {},
}

func NewTopology(types TypeResolver, devices []Device, relationships []Relationship) (*Topology, error) {
	if types == nil {
		return nil, errors.New("device type resolver is required")
	}
	topology := &Topology{devices: make(map[string]Device, len(devices))}
	for _, candidate := range devices {
		if err := validateDevice(types, candidate); err != nil {
			return nil, err
		}
		if _, exists := topology.devices[candidate.ID]; exists {
			return nil, fmt.Errorf("device %q is duplicated", candidate.ID)
		}
		topology.devices[candidate.ID] = candidate
	}

	seen := make(map[string]struct{}, len(relationships))
	for _, relationship := range relationships {
		if err := topology.validateRelationship(relationship); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%d\x00%s\x00%s\x00%s", relationship.ProjectID, relationship.FromID, relationship.ToID, relationship.Kind)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("device relationship %s %q -> %q is duplicated", relationship.Kind, relationship.FromID, relationship.ToID)
		}
		seen[key] = struct{}{}
		topology.relationships = append(topology.relationships, relationship)
	}
	if err := topology.validateGatewayGraph(); err != nil {
		return nil, err
	}
	return topology, nil
}

func validateDevice(types TypeResolver, candidate Device) error {
	if candidate.ID == "" || candidate.ProjectID <= 0 || candidate.DisplayName == "" {
		return errors.New("device identity, project, and display name are required")
	}
	if candidate.Type.Key == "" || candidate.Type.Version <= 0 {
		return fmt.Errorf("device %q must reference a versioned device type", candidate.ID)
	}
	if _, valid := validClasses[candidate.Class]; !valid {
		return fmt.Errorf("device %q has unsupported class %q", candidate.ID, candidate.Class)
	}
	resolved := types.Resolve(candidate.Type.Key, candidate.Type.Version)
	if resolved.Definition.TypeKey == "" {
		return fmt.Errorf("device %q references unknown device type %s@%d", candidate.ID, candidate.Type.Key, candidate.Type.Version)
	}
	if resolved.Definition.Category != string(candidate.Class) {
		return fmt.Errorf("device %q class %q does not match device type category %q", candidate.ID, candidate.Class, resolved.Definition.Category)
	}
	return nil
}

func (topology *Topology) validateRelationship(relationship Relationship) error {
	if _, valid := validRelationshipKinds[relationship.Kind]; !valid {
		return fmt.Errorf("unsupported device relationship %q", relationship.Kind)
	}
	if relationship.FromID == relationship.ToID {
		return errors.New("device relationship cannot reference itself")
	}
	from, fromExists := topology.devices[relationship.FromID]
	to, toExists := topology.devices[relationship.ToID]
	if !fromExists || !toExists {
		return fmt.Errorf("device relationship %q -> %q has an unknown endpoint", relationship.FromID, relationship.ToID)
	}
	if relationship.ProjectID <= 0 || from.ProjectID != relationship.ProjectID || to.ProjectID != relationship.ProjectID {
		return errors.New("device relationship endpoints must belong to the relationship project")
	}
	if relationship.Kind == RelationshipDockedAircraft && (from.Class != ClassDock || to.Class != ClassAircraft) {
		return errors.New("docked-aircraft relationship must point from a dock to an aircraft")
	}
	return nil
}

func (topology *Topology) validateGatewayGraph() error {
	gateways := make(map[string]string)
	for _, relationship := range topology.relationships {
		if relationship.Kind != RelationshipGatewayFor {
			continue
		}
		if existing, exists := gateways[relationship.ToID]; exists && existing != relationship.FromID {
			return fmt.Errorf("device %q has more than one gateway", relationship.ToID)
		}
		gateways[relationship.ToID] = relationship.FromID
	}
	for deviceID := range gateways {
		seen := map[string]struct{}{deviceID: {}}
		for current := deviceID; gateways[current] != ""; {
			current = gateways[current]
			if _, exists := seen[current]; exists {
				return fmt.Errorf("gateway-for relationships contain a cycle at device %q", current)
			}
			seen[current] = struct{}{}
		}
	}
	return nil
}

func (topology *Topology) Device(id string) (Device, bool) {
	device, exists := topology.devices[id]
	return device, exists
}

func (topology *Topology) Devices() []Device {
	devices := make([]Device, 0, len(topology.devices))
	for _, device := range topology.devices {
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].ID < devices[j].ID })
	return devices
}

func (topology *Topology) Relationships() []Relationship {
	result := make([]Relationship, len(topology.relationships))
	copy(result, topology.relationships)
	return result
}

// ResolveAdapter follows only explicit gateway-for edges. Physical attachment
// does not implicitly grant a device access to another device's connection.
func (topology *Topology) ResolveAdapter(deviceID string) (int64, bool) {
	device, exists := topology.devices[deviceID]
	if !exists {
		return 0, false
	}
	if device.AdapterID > 0 {
		return device.AdapterID, true
	}
	for _, relationship := range topology.relationships {
		if relationship.Kind == RelationshipGatewayFor && relationship.ToID == deviceID {
			return topology.ResolveAdapter(relationship.FromID)
		}
	}
	return 0, false
}
