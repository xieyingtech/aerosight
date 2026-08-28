package dji

import (
	"context"
	"encoding/json"
	"testing"

	"aerosight/worker/internal/credentials"
)

type secretFixture struct{ credentials MQTTCredentials }

func (fixture secretFixture) ResolveMQTT(context.Context, AdapterLease) (MQTTCredentials, error) {
	return fixture.credentials, nil
}

func TestBuildMQTTConfigUsesEncryptedCredentialResolverAndStableAdapterIdentity(t *testing.T) {
	config, err := BuildMQTTConfig(context.Background(), AdapterLease{
		AdapterID: 7, ProjectID: 3, BrokerURL: "mqtt://broker.example.test:1883",
		ConfigJSON: json.RawMessage(`{"topics":["dji/project-3/GW001/#"],"gatewaySerials":["GW001"]}`),
	}, secretFixture{credentials: MQTTCredentials{Username: "adapter-7", Password: "not-inline"}})
	if err != nil {
		t.Fatal(err)
	}
	if config.ClientID != "aerosight-3-7" || config.Username != "adapter-7" || string(config.Password) != "not-inline" {
		t.Fatalf("unexpected MQTT config: %+v", config)
	}
}

func TestEncryptedCredentialResolverBindsCredentialToAdapterAndProject(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	envelope, err := credentials.EncryptJSON(map[string]string{"mqttUsername": "dock", "mqttPassword": "password"}, secret,
		credentials.AAD("device-adapter", 7, 3))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	resolver := EncryptedCredentialResolver{AuthSecret: secret}
	value, err := resolver.ResolveMQTT(context.Background(), AdapterLease{AdapterID: 7, ProjectID: 3, CredentialEnvelope: raw})
	if err != nil || value.Username != "dock" || value.Password != "password" {
		t.Fatalf("unexpected credential: %#v %v", value, err)
	}
	if _, err := resolver.ResolveMQTT(context.Background(), AdapterLease{AdapterID: 7, ProjectID: 4, CredentialEnvelope: raw}); err == nil {
		t.Fatal("credential decrypted in the wrong project scope")
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
