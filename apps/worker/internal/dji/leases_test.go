package dji

import (
	"context"
	"encoding/json"
	"testing"
)

type secretFixture struct{ credentials MQTTCredentials }

func (fixture secretFixture) ResolveMQTT(context.Context, string) (MQTTCredentials, error) {
	return fixture.credentials, nil
}

func TestBuildMQTTConfigUsesSecretReferenceAndStableAdapterIdentity(t *testing.T) {
	config, err := BuildMQTTConfig(context.Background(), AdapterLease{
		AdapterID: 7, ProjectID: 3, BrokerURL: "mqtt://broker.example.test:1883", SecretRef: "secret://adapter/7",
		ConfigJSON: json.RawMessage(`{"topics":["dji/project-3/GW001/#"],"gatewaySerials":["GW001"]}`),
	}, secretFixture{credentials: MQTTCredentials{Username: "adapter-7", Password: "not-inline"}})
	if err != nil {
		t.Fatal(err)
	}
	if config.ClientID != "aerosight-3-7" || config.Username != "adapter-7" || string(config.Password) != "not-inline" {
		t.Fatalf("unexpected MQTT config: %+v", config)
	}
}

func TestBuildMQTTConfigRequiresScopedTopics(t *testing.T) {
	_, err := BuildMQTTConfig(context.Background(), AdapterLease{
		AdapterID: 7, ProjectID: 3, BrokerURL: "mqtt://broker.example.test:1883",
		ConfigJSON: json.RawMessage(`{}`),
	}, secretFixture{})
	if err == nil || err.Error() != "DJI_ADAPTER_TOPICS_REQUIRED" {
		t.Fatalf("missing topics were accepted: %v", err)
	}
}
