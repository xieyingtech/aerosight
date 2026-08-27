package dji

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"aerosight/worker/internal/driver"
)

type ProductKey struct {
	Domain  int
	Type    int
	Subtype int
}

type ProductDescriptor struct {
	Key               ProductKey
	Name              string
	TypeKey           string
	Category          string
	ValidatedFirmware string
	Capabilities      []string
}

var dock2Products = []ProductDescriptor{
	{Key: ProductKey{Domain: 3, Type: 2, Subtype: 0}, Name: "DJI Dock 2", TypeKey: "dji.dock2", Category: "dock", ValidatedFirmware: "14.03.07.01", Capabilities: []string{"state.read", "mission.execute", "mission.cancel", "dock.debug.control"}},
	{Key: ProductKey{Domain: 0, Type: 91, Subtype: 0}, Name: "DJI Matrice 3D", TypeKey: "dji.matrice3d", Category: "aircraft", ValidatedFirmware: "14.03.07.01", Capabilities: []string{"state.read", "mission.execute", "mission.cancel", "flight.return_home", "stream.telemetry.read"}},
	{Key: ProductKey{Domain: 0, Type: 91, Subtype: 1}, Name: "DJI Matrice 3TD", TypeKey: "dji.matrice3td", Category: "aircraft", ValidatedFirmware: "14.03.07.01", Capabilities: []string{"state.read", "mission.execute", "mission.cancel", "flight.return_home", "stream.telemetry.read"}},
	{Key: ProductKey{1, 80, 0}, Name: "Matrice 3D Camera", TypeKey: "dji.matrice3d.camera", Category: "camera", Capabilities: []string{"state.read", "stream.video.read", "stream.video.control"}},
	{Key: ProductKey{1, 81, 0}, Name: "Matrice 3TD Camera", TypeKey: "dji.matrice3td.camera", Category: "camera", Capabilities: []string{"state.read", "stream.video.read", "stream.video.control", "stream.sensor.read"}},
	{Key: ProductKey{Domain: 1, Type: 176, Subtype: 0}, Name: "Matrice 3 Vision Assist", TypeKey: "dji.matrice3.vision-assist", Category: "camera", Capabilities: []string{"state.read", "stream.video.read"}},
	{Key: ProductKey{Domain: 1, Type: 165, Subtype: 0}, Name: "DJI Dock 2 Camera", TypeKey: "dji.dock2.camera", Category: "camera", Capabilities: []string{"state.read", "stream.video.read", "stream.video.control"}},
	{Name: "DJI Dock 2 Environment Sensor", TypeKey: "dji.dock2.environment-sensor", Category: "sensor", Capabilities: []string{"state.read", "stream.sensor.read"}},
}

func Dock2ProductMatrix() []ProductDescriptor {
	result := make([]ProductDescriptor, len(dock2Products))
	copy(result, dock2Products)
	return result
}

func ResolveDock2Product(key ProductKey) (ProductDescriptor, bool) {
	for _, product := range dock2Products {
		if product.Key.Type != 0 && product.Key == key {
			return product, true
		}
	}
	return ProductDescriptor{}, false
}

func RegisterDock2DeviceTypes(registry *driver.DeviceTypeRegistry) error {
	for _, product := range dock2Products {
		profile := make(map[string]json.RawMessage, len(product.Capabilities))
		for _, capability := range product.Capabilities {
			profile[capability] = json.RawMessage(`{"enabled":true}`)
		}
		if err := registry.Register(driver.DeviceTypeDefinition{
			TypeKey: product.TypeKey, Version: 1, DisplayName: product.Name, Category: product.Category,
			DriverKey: DriverKey, DriverVersionConstraint: "^1.0.0", CapabilityProfile: profile,
			Status: driver.DeviceTypeActive,
		}); err != nil {
			return fmt.Errorf("register %s: %w", product.TypeKey, err)
		}
	}
	return nil
}

type enumValue int

func (value *enumValue) UnmarshalJSON(raw []byte) error {
	var number int
	if json.Unmarshal(raw, &number) == nil {
		*value = enumValue(number)
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return err
	}
	parsed, err := strconv.Atoi(text)
	if err != nil {
		return err
	}
	*value = enumValue(parsed)
	return nil
}

type productTopology struct {
	Domain       enumValue `json:"domain"`
	Type         enumValue `json:"type"`
	Subtype      enumValue `json:"sub_type"`
	ThingVersion string    `json:"thing_version"`
	SubDevices   []struct {
		SN           string    `json:"sn"`
		Domain       enumValue `json:"domain"`
		Type         enumValue `json:"type"`
		Subtype      enumValue `json:"sub_type"`
		ThingVersion string    `json:"thing_version"`
	} `json:"sub_devices"`
}

type ProductNode struct {
	ExternalID       string
	TypeKey          string
	Name             string
	Category         string
	Firmware         string
	ParentExternalID string
	Relation         string
}

func descriptorByTypeKey(typeKey string) ProductDescriptor {
	for _, product := range dock2Products {
		if product.TypeKey == typeKey {
			return product
		}
	}
	panic("unknown built-in DJI type key: " + typeKey)
}

func productNode(externalID string, product ProductDescriptor, firmware, parent, relation string) ProductNode {
	return ProductNode{ExternalID: externalID, TypeKey: product.TypeKey, Name: product.Name, Category: product.Category, Firmware: firmware, ParentExternalID: parent, Relation: relation}
}

func ExpandDock2Topology(gatewaySN string, payload json.RawMessage) ([]ProductNode, error) {
	if strings.TrimSpace(gatewaySN) == "" {
		return nil, errors.New("DJI_TOPOLOGY_GATEWAY_REQUIRED")
	}
	var topology productTopology
	if err := json.Unmarshal(payload, &topology); err != nil {
		return nil, fmt.Errorf("DJI_TOPOLOGY_INVALID: %w", err)
	}
	gatewayDomain := int(topology.Domain)
	if gatewayDomain == 0 {
		gatewayDomain = 3 // Dock 2 messages before domain was added imply the dock namespace.
	}
	dock, exists := ResolveDock2Product(ProductKey{Domain: gatewayDomain, Type: int(topology.Type), Subtype: int(topology.Subtype)})
	if !exists || dock.TypeKey != "dji.dock2" {
		return nil, errors.New("DJI_DOCK2_PRODUCT_UNSUPPORTED")
	}
	nodes := []ProductNode{productNode(gatewaySN, dock, topology.ThingVersion, "", "")}
	nodes = append(nodes,
		productNode(gatewaySN+":camera:0", descriptorByTypeKey("dji.dock2.camera"), topology.ThingVersion, gatewaySN, "contains"),
		productNode(gatewaySN+":environment", descriptorByTypeKey("dji.dock2.environment-sensor"), topology.ThingVersion, gatewaySN, "contains"),
	)
	for _, child := range topology.SubDevices {
		if child.SN == "" {
			return nil, errors.New("DJI_TOPOLOGY_DEVICE_SN_REQUIRED")
		}
		domain := int(child.Domain)
		product, supported := ResolveDock2Product(ProductKey{Domain: domain, Type: int(child.Type), Subtype: int(child.Subtype)})
		if !supported || product.Category != "aircraft" {
			return nil, errors.New("DJI_DOCK2_AIRCRAFT_UNSUPPORTED")
		}
		nodes = append(nodes, productNode(child.SN, product, child.ThingVersion, gatewaySN, "docked-aircraft"))
		cameraType := "dji.matrice3d.camera"
		if product.TypeKey == "dji.matrice3td" {
			cameraType = "dji.matrice3td.camera"
		}
		nodes = append(nodes,
			productNode(child.SN+":camera:0", descriptorByTypeKey(cameraType), child.ThingVersion, child.SN, "mounted-on"),
			productNode(child.SN+":vision-assist", descriptorByTypeKey("dji.matrice3.vision-assist"), child.ThingVersion, child.SN, "mounted-on"),
		)
	}
	return nodes, nil
}
