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
