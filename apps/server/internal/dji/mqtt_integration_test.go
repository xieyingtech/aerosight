package dji

import (
	"context"
	"encoding/json"
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

func TestMQTTManagerShutdownAndRestart(t *testing.T) {
	config := mqttIntegrationConfig(t, "manager-integration", nil)
	topic := fmt.Sprintf("dji/demo/integration/%d", time.Now().UnixNano())
	raw, err := json.Marshal(map[string]any{"topics": []string{topic}, "gatewaySerials": []string{"GW001"}})
	if err != nil {
		t.Fatal(err)
	}
	repository := &leaseRepositoryFixture{lease: AdapterLease{AdapterID: 1, ProjectID: 2, BrokerURL: config.BrokerURL, ConfigJSON: raw}}
	for _, owner := range []string{"before-shutdown", "after-restart"} {
		ctx, cancel := context.WithCancel(context.Background())
		sessions := make(chan *MQTTSession, 1)
		connector := func(ctx context.Context, cfg MQTTConfig, handler MQTTMessageHandler) (ManagedSession, error) {
			session, err := StartMQTTSession(ctx, cfg, handler)
			if err == nil {
				sessions <- session
			}
			return session, err
		}
		manager := NewAdapterManager(repository, secretFixture{credentials: MQTTCredentials{Username: config.Username, Password: string(config.Password)}}, connector, nil, owner, nil)
		done := make(chan error, 1)
		go func() { done <- manager.Run(ctx) }()
		func() {
			defer cancel()
			var session *MQTTSession
			select {
			case session = <-sessions:
			case <-time.After(8 * time.Second):
				t.Fatal("manager failed to create MQTT session")
			}
			deadline := time.Now().Add(8 * time.Second)
			for {
				repository.mu.Lock()
				connected := len(repository.statuses) > 0 && repository.statuses[len(repository.statuses)-1] == "connected"
				repository.mu.Unlock()
				if connected {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("manager failed to connect to broker")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := manager.Publish(ctx, 1, topic, []byte(owner)); err != nil {
				t.Fatal(err)
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(6 * time.Second):
				t.Fatal("manager shutdown timed out")
			}
			select {
			case <-session.Done():
			default:
				t.Fatal("manager returned before MQTT closed")
			}
			repository.mu.Lock()
			defer repository.mu.Unlock()
			if repository.owner != "" {
				t.Fatal("manager did not release lease after MQTT closed")
			}
			repository.statuses = nil
		}()
	}
}
