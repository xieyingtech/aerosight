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
	CapabilityConfig  map[string]json.RawMessage
}

var dock2Products = []ProductDescriptor{
	{Key: ProductKey{Domain: 3, Type: 2, Subtype: 0}, Name: "DJI Dock 2", TypeKey: "dji.dock2", Category: "dock", ValidatedFirmware: "14.03.07.01", Capabilities: []string{"state.read", "mission.execute", "mission.cancel", "dock.debug.control"}, CapabilityConfig: map[string]json.RawMessage{"dock.debug.control": json.RawMessage(`{"enabled":true,"productFamily":"dock2"}`)}},
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
	return registerProductDeviceTypes(registry, dock2Products)
}

func registerProductDeviceTypes(registry *driver.DeviceTypeRegistry, products []ProductDescriptor) error {
	for _, product := range products {
		profile := make(map[string]json.RawMessage, len(product.Capabilities))
		for _, capability := range product.Capabilities {
			configured := product.CapabilityConfig[capability]
			if len(configured) == 0 {
				configured = json.RawMessage(`{"enabled":true}`)
			}
			profile[capability] = configured
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
	Domain          enumValue `json:"domain"`
	Type            enumValue `json:"type"`
	Subtype         enumValue `json:"sub_type"`
	ThingVersion    string    `json:"thing_version"`
	FirmwareVersion string    `json:"firmware_version"`
	SubDevices      []struct {
		SN              string    `json:"sn"`
		Domain          enumValue `json:"domain"`
		Type            enumValue `json:"type"`
		Subtype         enumValue `json:"sub_type"`
		ThingVersion    string    `json:"thing_version"`
		FirmwareVersion string    `json:"firmware_version"`
	} `json:"sub_devices"`
}

type ProductNode struct {
	ExternalID          string
	TypeKey             string
	Name                string
	Category            string
	ProtocolVersion     string
	FirmwareVersion     string
	ParentExternalID    string
	Relation            string
	ProductKey          ProductKey
	ReadOnly            bool
	CompatibilityReason string
}

func descriptorByTypeKey(products []ProductDescriptor, typeKey string) ProductDescriptor {
	for _, product := range products {
		if product.TypeKey == typeKey {
			return product
		}
	}
	panic("unknown built-in DJI type key: " + typeKey)
}

func productNode(externalID string, product ProductDescriptor, firmware, parent, relation string) ProductNode {
	return ProductNode{ExternalID: externalID, TypeKey: product.TypeKey, Name: product.Name, Category: product.Category, ProtocolVersion: firmware, ParentExternalID: parent, Relation: relation, ProductKey: product.Key}
}

func applyProductCompatibility(node ProductNode, family string, product ProductDescriptor, firmware string) ProductNode {
	node.FirmwareVersion = strings.TrimSpace(firmware)
	if node.FirmwareVersion == "" {
		return node
	}
	compatibility := CheckProductCompatibility(family, product.Key, node.FirmwareVersion)
	if compatibility.ReadOnly {
		node.ReadOnly = true
		node.CompatibilityReason = compatibility.Reason
	}
	return node
}

func inheritProductCompatibility(node, parent ProductNode) ProductNode {
	node.FirmwareVersion = parent.FirmwareVersion
	if parent.ReadOnly {
		node.ReadOnly = true
		node.CompatibilityReason = parent.CompatibilityReason
	}
	return node
}

func unknownProductNode(externalID, protocolVersion, parent, relation string, key ProductKey) ProductNode {
	return ProductNode{
		ExternalID: externalID, TypeKey: UnknownDeviceTypeKey, Name: "Unknown DJI device", Category: "unknown",
		ProtocolVersion: protocolVersion, ParentExternalID: parent, Relation: relation, ProductKey: key,
		ReadOnly: true, CompatibilityReason: "DJI_PRODUCT_ENUM_UNKNOWN",
	}
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
	gatewayKey := ProductKey{Domain: gatewayDomain, Type: int(topology.Type), Subtype: int(topology.Subtype)}
	dock, exists := ResolveDock2Product(gatewayKey)
	if !exists {
		if _, belongsToDock3 := ResolveDock3Product(gatewayKey); belongsToDock3 {
			return nil, errors.New("DJI_DOCK2_PRODUCT_UNSUPPORTED")
		}
		return []ProductNode{unknownProductNode(gatewaySN, topology.ThingVersion, "", "", gatewayKey)}, nil
	}
	if dock.TypeKey != "dji.dock2" {
		return nil, errors.New("DJI_DOCK2_PRODUCT_UNSUPPORTED")
	}
	dockNode := applyProductCompatibility(productNode(gatewaySN, dock, topology.ThingVersion, "", ""), "dock2", dock, topology.FirmwareVersion)
	nodes := []ProductNode{dockNode}
	nodes = append(nodes,
		inheritProductCompatibility(productNode(gatewaySN+":camera:0", descriptorByTypeKey(dock2Products, "dji.dock2.camera"), topology.ThingVersion, gatewaySN, "contains"), dockNode),
		inheritProductCompatibility(productNode(gatewaySN+":environment", descriptorByTypeKey(dock2Products, "dji.dock2.environment-sensor"), topology.ThingVersion, gatewaySN, "contains"), dockNode),
	)
	for _, child := range topology.SubDevices {
		if child.SN == "" {
			return nil, errors.New("DJI_TOPOLOGY_DEVICE_SN_REQUIRED")
		}
		domain := int(child.Domain)
		childKey := ProductKey{Domain: domain, Type: int(child.Type), Subtype: int(child.Subtype)}
		product, supported := ResolveDock2Product(childKey)
		if !supported {
			if _, belongsToDock3 := ResolveDock3Product(childKey); belongsToDock3 {
				return nil, errors.New("DJI_DOCK2_AIRCRAFT_UNSUPPORTED")
			}
			nodes = append(nodes, unknownProductNode(child.SN, child.ThingVersion, gatewaySN, "gateway-for", childKey))
			continue
		}
		if product.Category != "aircraft" {
			return nil, errors.New("DJI_DOCK2_AIRCRAFT_UNSUPPORTED")
		}
		aircraftNode := applyProductCompatibility(productNode(child.SN, product, child.ThingVersion, gatewaySN, "docked-aircraft"), "dock2", product, child.FirmwareVersion)
		nodes = append(nodes, aircraftNode)
		cameraType := "dji.matrice3d.camera"
		if product.TypeKey == "dji.matrice3td" {
			cameraType = "dji.matrice3td.camera"
		}
		nodes = append(nodes,
			inheritProductCompatibility(productNode(child.SN+":camera:0", descriptorByTypeKey(dock2Products, cameraType), child.ThingVersion, child.SN, "mounted-on"), aircraftNode),
			inheritProductCompatibility(productNode(child.SN+":vision-assist", descriptorByTypeKey(dock2Products, "dji.matrice3.vision-assist"), child.ThingVersion, child.SN, "mounted-on"), aircraftNode),
		)
	}
	return nodes, nil
}
