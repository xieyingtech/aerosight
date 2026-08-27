package dji

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aerosight/worker/internal/device"
	"aerosight/worker/internal/driver"
	"aerosight/worker/internal/simulator"
)

type simulatorMQTTPublication struct {
	topic   string
	payload []byte
}

type simulatorMQTTTransport struct{ publications []simulatorMQTTPublication }

func (transport *simulatorMQTTTransport) Publish(_ context.Context, topic string, payload []byte) error {
	transport.publications = append(transport.publications, simulatorMQTTPublication{topic: topic, payload: append([]byte(nil), payload...)})
	return nil
}

func TestDock2SimulatorTraversesProductionDeviceAndCapabilityAPIs(t *testing.T) {
	now := time.UnixMilli(1_787_821_200_000).UTC()
	config, err := simulator.Dock2Scenario(simulator.DJIProductDock2M3TD, "DOCK2-DEMO-001", "M3TD-DEMO-001", now)
	if err != nil {
		t.Fatal(err)
	}
	config.Now = func() time.Time { return now.Add(time.Second) }
	transport := &simulatorMQTTTransport{}
	protocol, err := simulator.NewDJIProtocol(config, transport)
	if err != nil {
		t.Fatal(err)
	}
	if err := protocol.PublishTopology(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := protocol.PublishTelemetry(context.Background()); err != nil {
		t.Fatal(err)
	}
	command := []byte(`{"tid":"demo-return-home","bid":"demo-operation","timestamp":1787821202000,"method":"return_home","data":{}}`)
	if err := protocol.HandleMessage(context.Background(), protocol.ServiceTopic(), command); err != nil {
		t.Fatal(err)
	}

	scope := RouteContext{ProjectID: 17, AdapterID: 8, AllowedGatewaySNs: map[string]bool{"DOCK2-DEMO-001": true}}
	routed := make([]RoutedMessage, 0, len(transport.publications))
	for _, publication := range transport.publications {
		message, err := RouteMQTTMessage(scope, MQTTMessage{
			Topic: publication.topic, Payload: publication.payload, QoS: 1, ReceivedAt: now.Add(2 * time.Second),
		})
		if err != nil {
			t.Fatalf("simulator publication was rejected by production ingress: topic=%s err=%v", publication.topic, err)
		}
		routed = append(routed, message)
	}
	if len(routed) != 4 || routed[0].Kind != RouteTopology || routed[1].Kind != RouteTelemetry ||
		routed[2].Kind != RouteTelemetry || routed[3].Kind != RouteServiceReply {
		t.Fatalf("simulator did not traverse topology, realtime, and command routes: %+v", routed)
	}

	var topologyEnvelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(config.Topology, &topologyEnvelope); err != nil {
		t.Fatal(err)
	}
	nodes, err := ExpandDock2Topology(config.GatewaySN, topologyEnvelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	types := dock2Registry(t)
	observed := map[string]device.EffectiveCapabilities{}
	for _, node := range nodes {
		resolved := types.Resolve(node.TypeKey, 1)
		capabilities, err := device.CalculateEffectiveCapabilities(resolved, device.CapabilityReport{}, device.CapabilityReport{})
		if err != nil {
			t.Fatal(err)
		}
		observed[node.TypeKey] = capabilities
	}
	assertCapabilityKind(t, observed["dji.dock2"], "dock.debug.control", driver.CapabilityCommand)
	assertCapabilityKind(t, observed["dji.matrice3td"], "flight.return_home", driver.CapabilityCommand)
	assertStream(t, observed["dji.matrice3td.camera"], "video.primary", driver.StreamVideo)
	assertStream(t, observed["dji.dock2.environment-sensor"], "sensor.primary", driver.StreamSensor)
}

func TestDock3SimulatorFaultsDegradeAndIsolateCapabilities(t *testing.T) {
	now := time.UnixMilli(1_787_821_200_000).UTC()
	config, err := simulator.Dock3Scenario(simulator.DJIProductDock3M4TD, "DOCK3-DEMO-001", "M4TD-DEMO-001", now)
	if err != nil {
		t.Fatal(err)
	}
	config, err = simulator.InjectUnknownFirmware(config, "99.99.99.99")
	if err != nil {
		t.Fatal(err)
	}
	config.Now = func() time.Time { return now.Add(time.Second) }
	config.Faults = simulator.DJIFaults{
		NackMethods: map[string]int{"return_home": 314001}, UnknownCapability: "future.autonomy.execute",
	}
	transport := &simulatorMQTTTransport{}
	protocol, err := simulator.NewDJIProtocol(config, transport)
	if err != nil {
		t.Fatal(err)
	}
	if err := protocol.PublishTopology(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := protocol.PublishTelemetry(context.Background()); err != nil {
		t.Fatal(err)
	}
	command := []byte(`{"tid":"fault-nack","bid":"fault-operation","timestamp":1787821202000,"method":"return_home","data":{}}`)
	if err := protocol.HandleMessage(context.Background(), protocol.ServiceTopic(), command); err != nil {
		t.Fatal(err)
	}
	scope := RouteContext{ProjectID: 17, AdapterID: 9, AllowedGatewaySNs: map[string]bool{"DOCK3-DEMO-001": true}}
	for _, publication := range transport.publications {
		message, err := RouteMQTTMessage(scope, MQTTMessage{
			Topic: publication.topic, Payload: publication.payload, QoS: 1, ReceivedAt: now.Add(2 * time.Second),
		})
		if err != nil {
			t.Fatalf("fault publication bypassed or broke production ingress: topic=%s err=%v", publication.topic, err)
		}
		if message.Kind == RouteServiceReply {
			reply, err := DecodeServiceReply(message.Envelope.Payload, message.TransactionID, message.BusinessID, message.Method)
			if err == nil {
				// DecodeServiceReply consumes the routed data, not the canonical envelope.
				t.Fatalf("canonical payload unexpectedly decoded as direct service data: %+v", reply)
			}
			var canonical struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(message.Envelope.Payload, &canonical); err != nil {
				t.Fatal(err)
			}
			reply, err = DecodeServiceReply(canonical.Data, message.TransactionID, message.BusinessID, message.Method)
			if err != nil || reply.Outcome() != ReplyNacked || reply.Result != 314001 {
				t.Fatalf("NACK did not reach command protocol: reply=%+v err=%v", reply, err)
			}
		}
	}

	var topologyEnvelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(config.Topology, &topologyEnvelope); err != nil {
		t.Fatal(err)
	}
	nodes, err := ExpandDock3Topology(config.GatewaySN, topologyEnvelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	var aircraft ProductNode
	readOnlyChildren := 0
	for _, node := range nodes {
		if node.TypeKey == "dji.matrice4td" {
			aircraft = node
		}
		if node.ParentExternalID == "M4TD-DEMO-001" && node.ReadOnly && node.CompatibilityReason == "DJI_FIRMWARE_NOT_VALIDATED" {
			readOnlyChildren++
		}
	}
	if !aircraft.ReadOnly || aircraft.CompatibilityReason != "DJI_FIRMWARE_NOT_VALIDATED" || readOnlyChildren != 2 {
		t.Fatalf("firmware degradation did not propagate through device topology: aircraft=%+v children=%d", aircraft, readOnlyChildren)
	}
	compatibility := CheckProductCompatibility("dock3", aircraft.ProductKey, aircraft.FirmwareVersion)
	if !compatibility.ReadOnly || compatibility.Reason != "DJI_FIRMWARE_NOT_VALIDATED" {
		t.Fatalf("unknown firmware was not degraded: %+v", compatibility)
	}
	resolved := allDJITypes(t).Resolve(aircraft.TypeKey, 1)
	firmware := RestrictCapabilitiesForCompatibility(resolved, compatibility)
	available := true
	runtime := device.CapabilityReport{Capabilities: map[string]device.CapabilityState{
		"future.autonomy.execute": {Available: &available},
	}}
	effective, err := device.CalculateEffectiveCapabilities(resolved, firmware, runtime)
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range effective.Capabilities {
		if capability.Kind == driver.CapabilityCommand && capability.Available {
			t.Fatalf("degraded firmware exposed control: %+v", capability)
		}
	}
	assertStream(t, effective, "telemetry.primary", driver.StreamTelemetry)
	if len(effective.Quarantined) != 1 || effective.Quarantined[0].Code != "future.autonomy.execute" {
		t.Fatalf("unknown runtime capability escaped quarantine: %+v", effective.Quarantined)
	}
}
