package dji

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

type MQTTConfig struct {
	BrokerURL string
	ClientID  string
	Username  string
	Password  []byte
	Topics    []string
	TLSConfig *tls.Config
}

type MQTTMessage struct {
	Topic      string
	Payload    []byte
	QoS        byte
	Retained   bool
	Duplicate  bool
	ReceivedAt time.Time
}

type MQTTMessageHandler func(context.Context, MQTTMessage) error

type SessionEvent struct {
	State string
	Code  string
}

type MQTTSession struct {
	manager *autopaho.ConnectionManager
	events  chan SessionEvent
}

func parseMQTTBroker(raw string) (*url.URL, error) {
	broker, err := url.Parse(raw)
	if err != nil || broker.Hostname() == "" {
		return nil, errors.New("DJI_MQTT_BROKER_INVALID")
	}
	if broker.User != nil {
		return nil, errors.New("DJI_MQTT_INLINE_CREDENTIALS_FORBIDDEN")
	}
	switch broker.Scheme {
	case "mqtt":
	case "mqtts":
		broker.Scheme = "tls"
	default:
		return nil, errors.New("DJI_MQTT_SCHEME_UNSUPPORTED")
	}
	return broker, nil
}

func validateMQTTConfig(config MQTTConfig) error {
	if strings.TrimSpace(config.ClientID) == "" {
		return errors.New("DJI_MQTT_CLIENT_ID_REQUIRED")
	}
	if strings.TrimSpace(config.Username) == "" || len(config.Password) == 0 {
		return errors.New("DJI_MQTT_AUTH_REQUIRED")
	}
	for _, topic := range config.Topics {
		if strings.TrimSpace(topic) == "" {
			return errors.New("DJI_MQTT_TOPIC_INVALID")
		}
	}
	return nil
}

func StartMQTTSession(ctx context.Context, config MQTTConfig, handler MQTTMessageHandler) (*MQTTSession, error) {
	if err := validateMQTTConfig(config); err != nil {
		return nil, err
	}
	broker, err := parseMQTTBroker(config.BrokerURL)
	if err != nil {
		return nil, err
	}
	events := make(chan SessionEvent, 16)
	emit := func(event SessionEvent) {
		select {
		case events <- event:
		default:
		}
	}
	if handler == nil {
		handler = func(context.Context, MQTTMessage) error { return nil }
	}
	clientConfig := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{broker},
		TlsCfg:                        config.TLSConfig,
		KeepAlive:                     20,
		CleanStartOnInitialConnection: false,
		SessionExpiryInterval:         24 * 60 * 60,
		ConnectTimeout:                5 * time.Second,
		ReconnectBackoff: func(attempt int) time.Duration {
			delay := time.Duration(1<<min(attempt, 5)) * 100 * time.Millisecond
			return min(delay, 3*time.Second)
		},
		ConnectUsername: config.Username,
		ConnectPassword: append([]byte(nil), config.Password...),
		OnConnectError:  func(error) { emit(SessionEvent{State: "degraded", Code: "DJI_MQTT_CONNECT_FAILED"}) },
		OnConnectionDown: func() bool {
			emit(SessionEvent{State: "degraded", Code: "DJI_MQTT_CONNECTION_LOST"})
			return true
		},
		ClientConfig: paho.ClientConfig{
			ClientID: config.ClientID,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){func(received paho.PublishReceived) (bool, error) {
				packet := received.Packet
				err := handler(ctx, MQTTMessage{
					Topic: packet.Topic, Payload: append([]byte(nil), packet.Payload...), QoS: packet.QoS,
					Retained: packet.Retain, Duplicate: packet.Duplicate(), ReceivedAt: time.Now().UTC(),
				})
				return err == nil, err
			}},
		},
	}
	clientConfig.OnConnectionUp = func(manager *autopaho.ConnectionManager, _ *paho.Connack) {
		go func() {
			if len(config.Topics) > 0 {
				subscriptions := make([]paho.SubscribeOptions, 0, len(config.Topics))
				for _, topic := range config.Topics {
					subscriptions = append(subscriptions, paho.SubscribeOptions{Topic: topic, QoS: 1})
				}
				subscribeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				if _, err := manager.Subscribe(subscribeCtx, &paho.Subscribe{Subscriptions: subscriptions}); err != nil {
					emit(SessionEvent{State: "degraded", Code: "DJI_MQTT_SUBSCRIBE_FAILED"})
					return
				}
			}
			emit(SessionEvent{State: "connected", Code: "DJI_MQTT_READY"})
		}()
	}
	manager, err := autopaho.NewConnection(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("start DJI MQTT 5 session: %w", err)
	}
	return &MQTTSession{manager: manager, events: events}, nil
}

func (session *MQTTSession) Events() <-chan SessionEvent { return session.events }

func (session *MQTTSession) Done() <-chan struct{} { return session.manager.Done() }

func (session *MQTTSession) Publish(ctx context.Context, topic string, payload []byte) error {
	_, err := session.manager.Publish(ctx, &paho.Publish{Topic: topic, Payload: payload, QoS: 1})
	return err
}

func (session *MQTTSession) terminateConnectionForTest() {
	session.manager.TerminateConnectionForTest()
}
