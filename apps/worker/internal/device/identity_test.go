package device

import (
	"errors"
	"testing"
)

func identityRegistryFixture(t *testing.T) *IdentityRegistry {
	t.Helper()
	registry := NewIdentityRegistry(topologyTypeRegistry(t))
	for _, adapter := range []AdapterRegistration{
		{ID: 8, ProjectID: 17, DriverKey: "fixture.unified", DriverVersion: "1.0.0", Enabled: true},
		{ID: 9, ProjectID: 17, DriverKey: "fixture.unified", DriverVersion: "1.1.0", Enabled: true},
		{ID: 10, ProjectID: 18, DriverKey: "fixture.unified", DriverVersion: "1.0.0", Enabled: true},
	} {
		if err := registry.RegisterAdapter(adapter); err != nil {
			t.Fatal(err)
		}
	}
	return registry
}

func identityClaim(adapterID int64, externalID string, class Class) IdentityClaim {
	return IdentityClaim{
		ProjectID: 17, AdapterID: adapterID, ExternalID: externalID,
		Type: TypeReference{Key: "fixture." + string(class), Version: 1}, Class: class, DisplayName: externalID,
	}
}

func TestIdentityClaimAndCompatibleRebindPreserveDeviceID(t *testing.T) {
	registry := identityRegistryFixture(t)
	first, replayed, err := registry.Claim(identityClaim(8, "VENDOR-SN-1", ClassAircraft))
	if err != nil || replayed {
		t.Fatalf("claim identity: %+v replayed=%v err=%v", first, replayed, err)
	}
	rebound, err := registry.Rebind(17, first.Device.ID, 9)
	if err != nil {
		t.Fatal(err)
	}
	if rebound.Device.ID != first.Device.ID || len(rebound.AdapterSeen) != 2 {
		t.Fatalf("adapter rebind changed identity or lost connection history: first=%+v rebound=%+v", first, rebound)
	}
	replayedClaim, replayed, err := registry.Claim(identityClaim(9, "VENDOR-SN-1", ClassAircraft))
	if err != nil || !replayed || replayedClaim.Device.ID != first.Device.ID {
		t.Fatalf("rediscovery created a new device: %+v replayed=%v err=%v", replayedClaim, replayed, err)
	}
	if err := registry.RegisterAdapter(AdapterRegistration{ID: 9, ProjectID: 17, DriverKey: "fixture.unified", DriverVersion: "1.5.0", Enabled: true}); err != nil {
		t.Fatalf("compatible driver upgrade rejected: %v", err)
	}
	route, err := registry.ResolveRoute(17, first.Device.ID)
	if err != nil || route.AdapterID != 9 || route.DeviceID != first.Device.ID {
		t.Fatalf("route did not retain stable device identity after upgrade: %+v err=%v", route, err)
	}
}

func TestIdentityClaimDetectsConflictsAndProjectLeaks(t *testing.T) {
	registry := identityRegistryFixture(t)
	first, _, err := registry.Claim(identityClaim(8, "VENDOR-SN-1", ClassAircraft))
	if err != nil {
		t.Fatal(err)
	}
	conflict := identityClaim(8, "VENDOR-SN-1", ClassAircraft)
	conflict.RequestedDevice = "different-device"
	if _, _, err := registry.Claim(conflict); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("external identity conflict was accepted: %v", err)
	}
	secondIdentity := identityClaim(8, "VENDOR-SN-2", ClassAircraft)
	secondIdentity.RequestedDevice = first.Device.ID
	if _, _, err := registry.Claim(secondIdentity); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("device was bound to two external identities: %v", err)
	}
	crossProject := identityClaim(10, "VENDOR-SN-3", ClassAircraft)
	if _, _, err := registry.Claim(crossProject); !errors.Is(err, ErrAdapterUnavailable) {
		t.Fatalf("cross-project adapter claim was accepted: %v", err)
	}
}

func TestGatewayRouteFollowsGatewayRebindWithoutChangingChild(t *testing.T) {
	registry := identityRegistryFixture(t)
	gateway, _, err := registry.Claim(identityClaim(8, "DOCK-SN", ClassDock))
	if err != nil {
		t.Fatal(err)
	}
	childClaim := identityClaim(8, "AIRCRAFT-SN", ClassAircraft)
	childClaim.GatewayDeviceID = gateway.Device.ID
	child, _, err := registry.Claim(childClaim)
	if err != nil {
		t.Fatal(err)
	}
	if child.Device.AdapterID != 0 {
		t.Fatalf("gateway child was given a direct adapter binding: %+v", child)
	}
	if _, err := registry.Rebind(17, gateway.Device.ID, 9); err != nil {
		t.Fatal(err)
	}
	route, err := registry.ResolveRoute(17, child.Device.ID)
	if err != nil || route.AdapterID != 9 || route.ExternalID != "AIRCRAFT-SN" || route.DeviceID != child.Device.ID {
		t.Fatalf("child route did not follow gateway rebind: %+v err=%v", route, err)
	}
}

func TestIncompatibleAdapterUpgradeAndRebindFailClosed(t *testing.T) {
	registry := identityRegistryFixture(t)
	device, _, err := registry.Claim(identityClaim(8, "SENSOR-SN", ClassSensor))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterAdapter(AdapterRegistration{ID: 8, ProjectID: 17, DriverKey: "fixture.unified", DriverVersion: "2.0.0", Enabled: true}); !errors.Is(err, ErrAdapterUnavailable) {
		t.Fatalf("incompatible in-place driver upgrade was accepted: %v", err)
	}
	if _, err := registry.Rebind(17, device.Device.ID, 10); !errors.Is(err, ErrAdapterUnavailable) {
		t.Fatalf("cross-project rebind was accepted: %v", err)
	}
}
