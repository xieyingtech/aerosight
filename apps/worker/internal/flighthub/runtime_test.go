package flighthub

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
)

const runtimeProjectUUID = "00000000-0000-4000-8000-000000000001"

type directoryClientFixture struct {
	topologies  []Topology
	err         error
	token       string
	projectUUID string
}

func (fixture *directoryClientFixture) ListDevices(_ context.Context, token, projectUUID string) ([]Topology, error) {
	fixture.token = token
	fixture.projectUUID = projectUUID
	return fixture.topologies, fixture.err
}

type tokenResolverFixture struct {
	token string
	err   error
}

func (fixture tokenResolverFixture) ResolveToken(context.Context, connector.Instance) (string, error) {
	return fixture.token, fixture.err
}

func runtimeInstance() connector.Instance {
	return connector.Instance{
		ID: 7, ProjectID: 3, ConnectorKey: ConnectorKey, Version: ConnectorVersion,
		DiscoveryScope: json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"测试项目"}`),
	}
}

func TestDiscoveryScopeAcceptsLegacyProjectAndValidatesOptionalOrganization(t *testing.T) {
	legacy, err := parseScope(json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"测试项目"}`))
	if err != nil || legacy.OrganizationUUID != "" {
		t.Fatalf("legacy scope=%#v err=%v", legacy, err)
	}
	current, err := parseScope(json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"测试项目","organizationUuid":"00000000-0000-4000-8000-000000000010"}`))
	if err != nil || current.OrganizationUUID != "00000000-0000-4000-8000-000000000010" {
		t.Fatalf("current scope=%#v err=%v", current, err)
	}
	if _, err := parseScope(json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"测试项目","organizationUuid":"not-a-uuid"}`)); err == nil {
		t.Fatal("invalid organization scope was accepted")
	}
}

func directoryTopology() Topology {
	return Topology{
		Gateway: &Device{
			SN: "DOCK-001", Callsign: "机场一", Online: true,
			Model: DeviceModel{Key: "3-2-0", Domain: "3", Type: "2", Subtype: "0", Name: "DJI Dock 2", Class: "airport"},
		},
		Drone: &Device{
			SN: "AIRCRAFT-001", Online: false,
			Model: DeviceModel{Key: "0-91-1", Domain: "0", Type: "91", Subtype: "1", Name: "M3TD", Class: "drone"},
		},
	}
}

func TestEncryptedTokenResolverBindsCredentialToInstanceAndProject(t *testing.T) {
	secret := "runtime-auth-secret"
	envelope, err := credentials.EncryptJSON(map[string]string{"token": "flight-token"}, secret,
		credentials.AAD("device-adapter", 7, 3))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	resolver := EncryptedTokenResolver{AuthSecret: secret}
	instance := runtimeInstance()
	instance.CredentialEnvelope = raw
	token, err := resolver.ResolveToken(context.Background(), instance)
	if err != nil || token != "flight-token" {
		t.Fatalf("unexpected resolved credential %q: %v", token, err)
	}
	instance.ProjectID = 4
	if token, err := resolver.ResolveToken(context.Background(), instance); err == nil || token != "" || strings.Contains(err.Error(), "flight-token") {
		t.Fatalf("credential escaped its AAD scope: token=%q error=%v", token, err)
	}
}

func TestEncryptedTokenResolverRejectsUnknownCredentialFields(t *testing.T) {
	secret := "runtime-auth-secret"
	envelope, err := credentials.EncryptJSON(map[string]string{"token": "flight-token", "extra": "forbidden"}, secret,
		credentials.AAD("device-adapter", 7, 3))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	instance := runtimeInstance()
	instance.CredentialEnvelope = raw
	if _, err := (EncryptedTokenResolver{AuthSecret: secret}).ResolveToken(context.Background(), instance); err == nil {
		t.Fatal("credential parser accepted an unknown field")
	}
}

func TestRuntimeRegistersPollDiscoveryAndEnforcesScope(t *testing.T) {
	client := &directoryClientFixture{topologies: []Topology{directoryTopology()}}
	registry := connector.NewRegistry()
	if err := RegisterRuntime(registry, client, tokenResolverFixture{token: "runtime-token"}); err != nil {
		t.Fatal(err)
	}
	runtime, err := registry.Resolve(ConnectorKey, ConnectorVersion)
	if err != nil {
		t.Fatal(err)
	}
	instance := runtimeInstance()
	batch, err := runtime.DiscoveryHandlers[connector.DiscoveryPoll](context.Background(), connector.DiscoveryRequest{
		Instance: instance, Mode: connector.DiscoveryPoll,
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.token != "runtime-token" || client.projectUUID != runtimeProjectUUID || len(batch.Devices) != 2 || !batch.CompleteSnapshot || batch.SourceVersion != ContractVersion {
		t.Fatalf("unexpected runtime discovery: client=%#v batch=%#v", client, batch)
	}
	for _, device := range batch.Devices {
		if !runtime.ScopeFilter(instance, device) {
			t.Fatalf("runtime rejected in-scope device %#v", device)
		}
	}
	forged := batch.Devices[0]
	forged.ExternalID = "00000000-0000-4000-8000-000000000002/DOCK-001"
	if runtime.ScopeFilter(instance, forged) {
		t.Fatal("runtime accepted a cross-project external identity")
	}
}

func TestMapDirectoryUsesStableScopedIdentityAndReadOnlyProductTypes(t *testing.T) {
	devices, firstCursor, err := MapDirectory(runtimeProjectUUID, []Topology{directoryTopology()})
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 || devices[0].ExternalType != "dji.matrice3td" || devices[1].ExternalType != "dji.dock2" {
		t.Fatalf("unexpected mapped product types: %#v", devices)
	}
	byID := map[string]connector.ExternalDevice{}
	for _, device := range devices {
		byID[device.ExternalID] = device
		capabilities, ok := device.Attributes["capabilities"].([]string)
		if !ok || len(capabilities) != 1 || capabilities[0] != "state.read" || device.Attributes["readOnly"] != true {
			t.Fatalf("connector exposed non-read-only capability: %#v", device.Attributes)
		}
	}
	drone := byID[runtimeProjectUUID+"/AIRCRAFT-001"]
	if drone.ParentExternalID != runtimeProjectUUID+"/DOCK-001" {
		t.Fatalf("aircraft parent was not preserved: %#v", drone)
	}
	_, secondCursor, err := MapDirectory(runtimeProjectUUID, []Topology{directoryTopology()})
	if err != nil || string(firstCursor) != string(secondCursor) {
		t.Fatalf("snapshot cursor is unstable: %s %s %v", firstCursor, secondCursor, err)
	}
}

func TestMapDirectoryFailsClosedForDuplicateAndUnknownProducts(t *testing.T) {
	duplicate := directoryTopology()
	duplicate.Drone.SN = duplicate.Gateway.SN
	if _, _, err := MapDirectory(runtimeProjectUUID, []Topology{duplicate}); err == nil {
		t.Fatal("duplicate serial reached connector persistence")
	}
	unknown := directoryTopology()
	unknown.Drone.Model = DeviceModel{Key: "0-999-0", Domain: "0", Type: "999", Subtype: "0", Name: "Future aircraft", Class: "drone"}
	devices, _, err := MapDirectory(runtimeProjectUUID, []Topology{unknown})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, device := range devices {
		if strings.HasSuffix(device.ExternalID, "/AIRCRAFT-001") {
			found = device.ExternalType == "dji.unknown" && device.Attributes["knownProduct"] == false && device.Attributes["reviewReason"] != ""
		}
	}
	if !found {
		t.Fatalf("unknown product was not routed to review: %#v", devices)
	}
}

func TestMapDirectoryRecognizesDock3Products(t *testing.T) {
	topology := Topology{
		Gateway: &Device{SN: "DOCK-003", Model: DeviceModel{Key: "3-3-0", Domain: "3", Type: "3", Subtype: "0", Name: "DJI Dock 3", Class: "airport"}},
		Drone:   &Device{SN: "AIRCRAFT-004", Model: DeviceModel{Key: "0-100-1", Domain: "0", Type: "100", Subtype: "1", Name: "M4TD", Class: "drone"}},
	}
	devices, _, err := MapDirectory(runtimeProjectUUID, []Topology{topology})
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]bool{}
	for _, device := range devices {
		types[device.ExternalType] = true
		if device.Attributes["serialNumber"] == "" {
			t.Fatalf("mapped identity omitted cross-source conflict key: %#v", device)
		}
	}
	if !types["dji.dock3"] || !types["dji.matrice4td"] {
		t.Fatalf("Dock 3 topology did not reuse DJI DeviceTypes: %#v", devices)
	}
}

func TestRuntimeDoesNotReturnPartialBatchOnUpstreamFailure(t *testing.T) {
	registry := connector.NewRegistry()
	upstream := errors.New("DJI_FLIGHTHUB_UPSTREAM_UNAVAILABLE")
	if err := RegisterRuntime(registry, &directoryClientFixture{err: upstream}, tokenResolverFixture{token: "runtime-token"}); err != nil {
		t.Fatal(err)
	}
	runtime, _ := registry.Resolve(ConnectorKey, ConnectorVersion)
	batch, err := runtime.DiscoveryHandlers[connector.DiscoveryPoll](context.Background(), connector.DiscoveryRequest{Instance: runtimeInstance()})
	if !errors.Is(err, upstream) || len(batch.Devices) != 0 || batch.CompleteSnapshot {
		t.Fatalf("upstream failure produced a partial batch: %#v %v", batch, err)
	}
}
