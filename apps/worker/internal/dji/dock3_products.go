package dji

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aerosight/worker/internal/driver"
)

var dock3Products = []ProductDescriptor{
	{Key: ProductKey{Domain: 3, Type: 3, Subtype: 0}, Name: "DJI Dock 3", TypeKey: "dji.dock3", Category: "dock", ValidatedFirmware: "14.03.00.03", Capabilities: []string{"state.read", "mission.execute", "mission.cancel", "dock.debug.control"}, CapabilityConfig: map[string]json.RawMessage{"dock.debug.control": json.RawMessage(`{"enabled":true,"productFamily":"dock3"}`)}},
	{Key: ProductKey{Domain: 0, Type: 100, Subtype: 0}, Name: "DJI Matrice 4D", TypeKey: "dji.matrice4d", Category: "aircraft", ValidatedFirmware: "14.03.00.03", Capabilities: []string{"state.read", "mission.execute", "mission.cancel", "flight.return_home", "stream.telemetry.read"}},
	{Key: ProductKey{Domain: 0, Type: 100, Subtype: 1}, Name: "DJI Matrice 4TD", TypeKey: "dji.matrice4td", Category: "aircraft", ValidatedFirmware: "14.03.00.03", Capabilities: []string{"state.read", "mission.execute", "mission.cancel", "flight.return_home", "stream.telemetry.read"}},
	{Key: ProductKey{Domain: 1, Type: 98, Subtype: 0}, Name: "Matrice 4D Camera", TypeKey: "dji.matrice4d.camera", Category: "camera", Capabilities: []string{"state.read", "stream.video.read", "stream.video.control"}},
	{Key: ProductKey{Domain: 1, Type: 99, Subtype: 0}, Name: "Matrice 4TD Camera", TypeKey: "dji.matrice4td.camera", Category: "camera", Capabilities: []string{"state.read", "stream.video.read", "stream.video.control", "stream.sensor.read"}},
	{Key: ProductKey{Domain: 1, Type: 176, Subtype: 0}, Name: "Matrice 4 Vision Assist", TypeKey: "dji.matrice4.vision-assist", Category: "camera", Capabilities: []string{"state.read", "stream.video.read"}},
	{Key: ProductKey{Domain: 1, Type: 165, Subtype: 0}, Name: "DJI Dock 3 Camera", TypeKey: "dji.dock3.camera", Category: "camera", Capabilities: []string{"state.read", "stream.video.read", "stream.video.control"}},
	{Name: "DJI Dock 3 Environment Sensor", TypeKey: "dji.dock3.environment-sensor", Category: "sensor", Capabilities: []string{"state.read", "stream.sensor.read"}},
}

func Dock3ProductMatrix() []ProductDescriptor {
	result := make([]ProductDescriptor, len(dock3Products))
	copy(result, dock3Products)
	return result
}

func ResolveDock3Product(key ProductKey) (ProductDescriptor, bool) {
	for _, product := range dock3Products {
		if product.Key.Type != 0 && product.Key == key {
			return product, true
		}
	}
	return ProductDescriptor{}, false
}

func RegisterDock3DeviceTypes(registry *driver.DeviceTypeRegistry) error {
	return registerProductDeviceTypes(registry, dock3Products)
}

func ExpandDock3Topology(gatewaySN string, payload json.RawMessage) ([]ProductNode, error) {
	if strings.TrimSpace(gatewaySN) == "" {
		return nil, errors.New("DJI_TOPOLOGY_GATEWAY_REQUIRED")
	}
	var topology productTopology
	if err := json.Unmarshal(payload, &topology); err != nil {
		return nil, fmt.Errorf("DJI_TOPOLOGY_INVALID: %w", err)
	}
	gatewayKey := ProductKey{Domain: int(topology.Domain), Type: int(topology.Type), Subtype: int(topology.Subtype)}
	dock, exists := ResolveDock3Product(gatewayKey)
	if !exists {
		if _, belongsToDock2 := ResolveDock2Product(gatewayKey); belongsToDock2 {
			return nil, errors.New("DJI_DOCK3_PRODUCT_UNSUPPORTED")
		}
		return []ProductNode{unknownProductNode(gatewaySN, topology.ThingVersion, "", "", gatewayKey)}, nil
	}
	if dock.TypeKey != "dji.dock3" {
		return nil, errors.New("DJI_DOCK3_PRODUCT_UNSUPPORTED")
	}
	nodes := []ProductNode{productNode(gatewaySN, dock, topology.ThingVersion, "", "")}
	nodes = append(nodes,
		productNode(gatewaySN+":camera:0", descriptorByTypeKey(dock3Products, "dji.dock3.camera"), topology.ThingVersion, gatewaySN, "contains"),
		productNode(gatewaySN+":environment", descriptorByTypeKey(dock3Products, "dji.dock3.environment-sensor"), topology.ThingVersion, gatewaySN, "contains"),
	)
	for _, child := range topology.SubDevices {
		if child.SN == "" {
			return nil, errors.New("DJI_TOPOLOGY_DEVICE_SN_REQUIRED")
		}
		childKey := ProductKey{Domain: int(child.Domain), Type: int(child.Type), Subtype: int(child.Subtype)}
		product, supported := ResolveDock3Product(childKey)
		if !supported {
			if _, belongsToDock2 := ResolveDock2Product(childKey); belongsToDock2 {
				return nil, errors.New("DJI_DOCK3_AIRCRAFT_UNSUPPORTED")
			}
			nodes = append(nodes, unknownProductNode(child.SN, child.ThingVersion, gatewaySN, "gateway-for", childKey))
			continue
		}
		if product.Category != "aircraft" {
			return nil, errors.New("DJI_DOCK3_AIRCRAFT_UNSUPPORTED")
		}
		nodes = append(nodes, productNode(child.SN, product, child.ThingVersion, gatewaySN, "docked-aircraft"))
		cameraType := "dji.matrice4d.camera"
		if product.TypeKey == "dji.matrice4td" {
			cameraType = "dji.matrice4td.camera"
		}
		nodes = append(nodes,
			productNode(child.SN+":camera:0", descriptorByTypeKey(dock3Products, cameraType), child.ThingVersion, child.SN, "mounted-on"),
			productNode(child.SN+":vision-assist", descriptorByTypeKey(dock3Products, "dji.matrice4.vision-assist"), child.ThingVersion, child.SN, "mounted-on"),
		)
	}
	return nodes, nil
}
