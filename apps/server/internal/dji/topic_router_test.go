package dji

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func routeFixture(topic, tid, gateway, method string, timestamp int64) MQTTMessage {
	payload := fmt.Sprintf(`{"tid":%q,"bid":"bid-%s","timestamp":%d,"gateway":%q,"method":%q,"data":{"seq":%d,"value":1}}`,
		tid, tid, timestamp, gateway, method, timestamp)
	return MQTTMessage{Topic: topic, Payload: []byte(payload), QoS: 1, ReceivedAt: time.UnixMilli(timestamp + 10).UTC()}
}

func TestDJITopicRouterCoversTopologyStateEventsRequestsAndReplies(t *testing.T) {
	scope := RouteContext{ProjectID: 9, AdapterID: 8, AllowedGatewaySNs: map[string]bool{"GW001": true}}
	tests := []struct {
		topic, gateway, method string
		kind                   RouteKind
		eventType, deviceSN    string
	}{
		{"sys/product/GW001/status", "", "update_topo", RouteTopology, "device.topology", "GW001"},
		{"thing/product/AIR001/state", "GW001", "", RouteState, "device.state", "AIR001"},
		{"thing/product/AIR001/osd", "GW001", "", RouteTelemetry, "device.telemetry", "AIR001"},
		{"thing/product/GW001/events", "GW001", "flighttask_progress", RouteEvent, "device.event", "GW001"},
		{"thing/product/GW001/requests", "GW001", "storage_config_get", RouteRequest, "device.request", "GW001"},
		{"thing/product/GW001/services_reply", "GW001", "return_home", RouteServiceReply, "command.reply", "GW001"},
	}
	for index, fixture := range tests {
		routed, err := RouteMQTTMessage(scope, routeFixture(fixture.topic, fmt.Sprintf("tid-%d", index), fixture.gateway, fixture.method, 1_700_000_000_000+int64(index)))
		if err != nil {
			t.Fatalf("route %s: %v", fixture.topic, err)
		}
		if routed.Kind != fixture.kind || routed.Envelope.EventType != fixture.eventType || routed.DeviceSN != fixture.deviceSN {
			t.Fatalf("unexpected route for %s: %+v", fixture.topic, routed)
		}
		if err := routed.Envelope.ValidateForScope(9, 8); err != nil {
			t.Fatalf("invalid unified envelope for %s: %v", fixture.topic, err)
		}
	}
}

func TestDJITopicRouterRejectsUnclaimedAndMismatchedGatewayIdentity(t *testing.T) {
	scope := RouteContext{ProjectID: 9, AdapterID: 8, AllowedGatewaySNs: map[string]bool{"GW001": true}}
	for _, message := range []MQTTMessage{
		routeFixture("thing/product/AIR001/state", "tid-a", "GW999", "", 1_700_000_000_000),
		routeFixture("thing/product/GW001/events", "tid-b", "GW999", "event", 1_700_000_000_001),
		routeFixture("thing/product/GW999/services_reply", "tid-c", "", "reply", 1_700_000_000_002),
	} {
		if _, err := RouteMQTTMessage(scope, message); err == nil {
			t.Fatalf("identity mismatch was accepted: %+v", message)
		}
	}
}

type ingressStoreFixture struct {
	mu           sync.Mutex
	seen         map[string]bool
	cursors      map[string]int64
	dispositions []IngressDisposition
	calls        int
}

func (store *ingressStoreFixture) Accept(_ context.Context, message RoutedMessage) (IngressDisposition, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.calls++
	key := message.Topic + "\x00" + message.TransactionID
	if store.seen[key] {
		store.dispositions = append(store.dispositions, IngressDuplicate)
		return IngressDuplicate, nil
	}
	store.seen[key] = true
	if message.Ordered() && message.TimestampMS < store.cursors[message.RouteKey()] {
		store.dispositions = append(store.dispositions, IngressOutOfOrder)
		return IngressOutOfOrder, nil
	}
	store.cursors[message.RouteKey()] = message.TimestampMS
	store.dispositions = append(store.dispositions, IngressAccepted)
	return IngressAccepted, nil
}

func TestMessageIngestorIsolatesDuplicateOutOfOrderAndIdentityMismatch(t *testing.T) {
	store := &ingressStoreFixture{seen: map[string]bool{}, cursors: map[string]int64{}}
	ingestor := NewMessageIngestor(store)
	scope := RouteContext{ProjectID: 9, AdapterID: 8, AllowedGatewaySNs: map[string]bool{"GW001": true}}
	handle := ingestor.Handle(scope)
	latest := routeFixture("thing/product/AIR001/state", "tid-latest", "GW001", "", 1_700_000_000_200)
	if err := handle(context.Background(), latest); err != nil {
		t.Fatal(err)
	}
	if err := handle(context.Background(), latest); err != nil {
		t.Fatal(err)
	}
	if err := handle(context.Background(), routeFixture("thing/product/AIR001/state", "tid-old", "GW001", "", 1_700_000_000_100)); err != nil {
		t.Fatal(err)
	}
	if err := handle(context.Background(), routeFixture("thing/product/AIR001/state", "tid-forged", "GW999", "", 1_700_000_000_300)); err == nil {
		t.Fatal("identity mismatch reached the ingress store")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.calls != 3 {
		t.Fatalf("identity mismatch was not isolated before persistence: %d calls", store.calls)
	}
	want := []IngressDisposition{IngressAccepted, IngressDuplicate, IngressOutOfOrder}
	for index := range want {
		if store.dispositions[index] != want[index] {
			t.Fatalf("unexpected disposition sequence: %+v", store.dispositions)
		}
	}
}
