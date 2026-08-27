package device

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aerosight/worker/internal/driver"
)

func topologyTypeRegistry(t *testing.T) *driver.DeviceTypeRegistry {
	t.Helper()
	drivers := driver.NewRegistry()
	manifest := driver.Manifest{
		DriverKey: "fixture.unified", Version: "1.0.0", DisplayName: "Fixture unified driver",
		Protocols: []string{"fixture"},
		Capabilities: []driver.CapabilityDefinition{{
			Code: "state.read", Kind: driver.CapabilityRead, Risk: driver.RiskLow,
			InputSchema: json.RawMessage(`{}`), OutputSchema: json.RawMessage(`{}`),
		}},
	}
	if err := drivers.Register(driver.Runtime{
		Manifest: manifest,
		ProtocolHandlers: map[string]driver.ProtocolHandler{
			"fixture": func(context.Context, driver.AdapterConfig) error { return nil },
		},
		CommandHandlers: map[string]driver.CommandHandler{},
		StreamHandlers:  map[string]driver.StreamHandler{},
	}); err != nil {
		t.Fatal(err)
	}
	types := driver.NewDeviceTypeRegistry(drivers)
	for _, category := range []Class{ClassDock, ClassAircraft, ClassRobot, ClassCamera, ClassSensor, ClassGateway} {
		definition := driver.DeviceTypeDefinition{
			TypeKey: "fixture." + string(category), Version: 1, DisplayName: "Fixture " + string(category), Category: string(category),
			DriverKey: "fixture.unified", DriverVersionConstraint: "^1.0.0", CapabilityProfile: map[string]json.RawMessage{"state.read": json.RawMessage(`{}`)}, Status: driver.DeviceTypeActive,
		}
		if err := types.Register(definition); err != nil {
			t.Fatal(err)
		}
	}
	return types
}

func deviceFixture(id string, class Class, adapterID int64) Device {
	return Device{ID: id, ProjectID: 17, Type: TypeReference{Key: "fixture." + string(class), Version: 1}, Class: class, DisplayName: id, AdapterID: adapterID}
}

func TestTopologyKeepsEveryHardwareKindAsADevice(t *testing.T) {
	devices := []Device{
		deviceFixture("dock", ClassDock, 8),
		deviceFixture("aircraft", ClassAircraft, 0),
		deviceFixture("robot", ClassRobot, 9),
		deviceFixture("camera", ClassCamera, 0),
		deviceFixture("sensor", ClassSensor, 0),
		deviceFixture("gateway", ClassGateway, 10),
	}
	relationships := []Relationship{
		{ProjectID: 17, FromID: "dock", ToID: "aircraft", Kind: RelationshipDockedAircraft},
		{ProjectID: 17, FromID: "dock", ToID: "aircraft", Kind: RelationshipGatewayFor},
		{ProjectID: 17, FromID: "robot", ToID: "camera", Kind: RelationshipMountedOn},
		{ProjectID: 17, FromID: "robot", ToID: "sensor", Kind: RelationshipContains},
		{ProjectID: 17, FromID: "gateway", ToID: "sensor", Kind: RelationshipGatewayFor},
	}
	topology, err := NewTopology(topologyTypeRegistry(t), devices, relationships)
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Devices()) != len(devices) || len(topology.Relationships()) != len(relationships) {
		t.Fatalf("hardware was lost or represented outside the device graph: devices=%+v relationships=%+v", topology.Devices(), topology.Relationships())
	}
	for _, expected := range devices {
		actual, exists := topology.Device(expected.ID)
		if !exists || actual.Class != expected.Class || actual.Type != expected.Type {
			t.Fatalf("device %q was not retained as a typed device: %+v", expected.ID, actual)
		}
	}
	if adapterID, ok := topology.ResolveAdapter("aircraft"); !ok || adapterID != 8 {
		t.Fatalf("aircraft did not inherit its dock gateway adapter: id=%d ok=%v", adapterID, ok)
	}
	if adapterID, ok := topology.ResolveAdapter("sensor"); !ok || adapterID != 10 {
		t.Fatalf("sensor did not inherit its explicit gateway adapter: id=%d ok=%v", adapterID, ok)
	}
	if _, ok := topology.ResolveAdapter("camera"); ok {
		t.Fatal("mounted-on relationship incorrectly implied connection routing")
	}
}

func TestTopologyRequiresRegisteredVersionedDeviceType(t *testing.T) {
	device := deviceFixture("sensor", ClassSensor, 0)
	device.Type = TypeReference{}
	if _, err := NewTopology(topologyTypeRegistry(t), []Device{device}, nil); err == nil || !strings.Contains(err.Error(), "versioned device type") {
		t.Fatalf("untyped device was accepted: %v", err)
	}
	device.Type = TypeReference{Key: "fixture.missing", Version: 1}
	if _, err := NewTopology(topologyTypeRegistry(t), []Device{device}, nil); err == nil || !strings.Contains(err.Error(), "unknown device type") {
		t.Fatalf("unknown device type was accepted: %v", err)
	}
}

func TestTopologyRejectsCrossProjectAndInvalidDockingRelationships(t *testing.T) {
	types := topologyTypeRegistry(t)
	dock := deviceFixture("dock", ClassDock, 8)
	aircraft := deviceFixture("aircraft", ClassAircraft, 0)
	aircraft.ProjectID = 18
	if _, err := NewTopology(types, []Device{dock, aircraft}, []Relationship{{ProjectID: 17, FromID: "dock", ToID: "aircraft", Kind: RelationshipDockedAircraft}}); err == nil || !strings.Contains(err.Error(), "relationship project") {
		t.Fatalf("cross-project relationship was accepted: %v", err)
	}
	robot := deviceFixture("robot", ClassRobot, 0)
	if _, err := NewTopology(types, []Device{dock, robot}, []Relationship{{ProjectID: 17, FromID: "dock", ToID: "robot", Kind: RelationshipDockedAircraft}}); err == nil || !strings.Contains(err.Error(), "dock to an aircraft") {
		t.Fatalf("invalid docking relationship was accepted: %v", err)
	}
}

func TestTopologyRejectsAmbiguousAndCyclicGateways(t *testing.T) {
	types := topologyTypeRegistry(t)
	first := deviceFixture("first", ClassGateway, 1)
	second := deviceFixture("second", ClassGateway, 2)
	sensor := deviceFixture("sensor", ClassSensor, 0)
	if _, err := NewTopology(types, []Device{first, second, sensor}, []Relationship{
		{ProjectID: 17, FromID: "first", ToID: "sensor", Kind: RelationshipGatewayFor},
		{ProjectID: 17, FromID: "second", ToID: "sensor", Kind: RelationshipGatewayFor},
	}); err == nil || !strings.Contains(err.Error(), "more than one gateway") {
		t.Fatalf("ambiguous gateway routing was accepted: %v", err)
	}
	if _, err := NewTopology(types, []Device{first, second}, []Relationship{
		{ProjectID: 17, FromID: "first", ToID: "second", Kind: RelationshipGatewayFor},
		{ProjectID: 17, FromID: "second", ToID: "first", Kind: RelationshipGatewayFor},
	}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cyclic gateway routing was accepted: %v", err)
	}
}
