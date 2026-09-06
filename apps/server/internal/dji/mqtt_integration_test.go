package dji

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func mqttIntegrationConfig(t *testing.T, clientID string, topics []string) MQTTConfig {
	t.Helper()
	broker := os.Getenv("AEROSIGHT_TEST_MQTT_URL")
	username := os.Getenv("AEROSIGHT_TEST_MQTT_USER")
	password := os.Getenv("AEROSIGHT_TEST_MQTT_PASSWORD")
	if broker == "" || username == "" || password == "" {
		t.Skip("set AEROSIGHT_TEST_MQTT_URL/USER/PASSWORD to run the temporary Broker integration")
	}
	return MQTTConfig{BrokerURL: broker, ClientID: clientID, Username: username, Password: []byte(password), Topics: topics}
}

func waitSessionState(t *testing.T, session *MQTTSession, state string) {
	t.Helper()
	select {
	case event := <-session.Events():
		if event.State != state {
			t.Fatalf("wanted MQTT state %s, got %+v", state, event)
		}
	case <-time.After(8 * time.Second):
		t.Fatalf("timed out waiting for MQTT state %s", state)
	}
}

func waitMessagePayload(t *testing.T, messages <-chan MQTTMessage, expected string) MQTTMessage {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case message := <-messages:
			if string(message.Payload) == expected {
				return message
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for MQTT payload %q", expected)
		}
	}
}

func TestMQTT5AuthenticationReconnectAndSubscriptionRecovery(t *testing.T) {
	nonce := time.Now().UnixNano()
	topic := fmt.Sprintf("dji/demo/integration/%d", nonce)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	messages := make(chan MQTTMessage, 2)
	subscriber, err := StartMQTTSession(ctx, mqttIntegrationConfig(t, fmt.Sprintf("aerosight-sub-%d", nonce), []string{topic}), func(_ context.Context, message MQTTMessage) error {
		messages <- message
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := StartMQTTSession(ctx, mqttIntegrationConfig(t, fmt.Sprintf("aerosight-pub-%d", nonce), nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	waitSessionState(t, subscriber, "connected")
	waitSessionState(t, publisher, "connected")
	if err := publisher.Publish(ctx, topic, []byte("before-reconnect")); err != nil {
		t.Fatal(err)
	}
	if message := waitMessagePayload(t, messages, "before-reconnect"); message.QoS != 1 {
		t.Fatalf("unexpected MQTT 5 message: %+v", message)
	}

	subscriber.terminateConnectionForTest()
	waitSessionState(t, subscriber, "degraded")
	waitSessionState(t, subscriber, "connected")
	if err := publisher.Publish(ctx, topic, []byte("after-reconnect")); err != nil {
		t.Fatal(err)
	}
	waitMessagePayload(t, messages, "after-reconnect")
	cancel()
	select {
	case <-subscriber.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("subscriber did not stop")
	}
}

func TestMQTT5RejectsInvalidAuthentication(t *testing.T) {
	nonce := time.Now().UnixNano()
	config := mqttIntegrationConfig(t, fmt.Sprintf("aerosight-bad-auth-%d", nonce), nil)
	config.Password = []byte("definitely-not-the-configured-password")
	ctx, cancel := context.WithCancel(context.Background())
	session, err := StartMQTTSession(ctx, config, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	waitSessionState(t, session, "degraded")
	cancel()
	select {
	case <-session.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("invalid-auth session did not stop")
	}
}
