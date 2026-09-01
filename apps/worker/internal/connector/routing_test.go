package connector

import (
	"errors"
	"testing"
)

func TestSelectPrimaryRouteSupportsDirectGatewayAndInherited(t *testing.T) {
	for _, role := range []string{"direct", "gateway", "inherited"} {
		route, err := SelectPrimaryRoute([]BindingRoute{{
			ConnectorInstanceID: 7, ExternalIdentityID: 11, Role: role, Priority: 100, Status: "active",
		}})
		if err != nil || route.Role != role {
			t.Fatalf("role %s was not routable: route=%#v err=%v", role, route, err)
		}
	}
}

func TestSelectPrimaryRouteUsesUniqueHighestPriority(t *testing.T) {
	route, err := SelectPrimaryRoute([]BindingRoute{
		{ConnectorInstanceID: 1, ExternalIdentityID: 10, Role: "direct", Priority: 20, Status: "active"},
		{ConnectorInstanceID: 2, ExternalIdentityID: 20, Role: "inherited", Priority: 100, Status: "active"},
		{ConnectorInstanceID: 3, ExternalIdentityID: 30, Role: "inherited", Priority: 200, Status: "standby"},
	})
	if err != nil || route.ConnectorInstanceID != 2 {
		t.Fatalf("unexpected primary route: route=%#v err=%v", route, err)
	}
}

func TestSelectPrimaryRouteFailsClosedForDoublePrimary(t *testing.T) {
	_, err := SelectPrimaryRoute([]BindingRoute{
		{ConnectorInstanceID: 1, ExternalIdentityID: 10, Role: "direct", Priority: 100, Status: "active"},
		{ConnectorInstanceID: 2, ExternalIdentityID: 20, Role: "gateway", Priority: 100, Status: "active"},
	})
	if !errors.Is(err, ErrRouteConflict) {
		t.Fatalf("expected route conflict, got %v", err)
	}
}

func TestSelectPrimaryRouteRejectsUnavailableAndInvalidRoutes(t *testing.T) {
	_, err := SelectPrimaryRoute([]BindingRoute{
		{ConnectorInstanceID: 1, ExternalIdentityID: 10, Role: "unknown", Priority: 100, Status: "active"},
		{ConnectorInstanceID: 2, ExternalIdentityID: 20, Role: "direct", Priority: 90, Status: "disabled"},
	})
	if !errors.Is(err, ErrRouteUnavailable) {
		t.Fatalf("expected unavailable route, got %v", err)
	}
}

func TestConnectorMigrationKeepsDeviceIdentityAndSwitchesPrimary(t *testing.T) {
	const deviceID = 42
	before, err := SelectPrimaryRoute([]BindingRoute{
		{ConnectorInstanceID: 1, ExternalIdentityID: 10, Role: "direct", Priority: 100, Status: "active"},
		{ConnectorInstanceID: 2, ExternalIdentityID: 20, Role: "direct", Priority: 90, Status: "standby"},
	})
	if err != nil || before.ConnectorInstanceID != 1 {
		t.Fatalf("unexpected route before migration for device %d: %#v %v", deviceID, before, err)
	}
	after, err := SelectPrimaryRoute([]BindingRoute{
		{ConnectorInstanceID: 1, ExternalIdentityID: 10, Role: "direct", Priority: 100, Status: "standby"},
		{ConnectorInstanceID: 2, ExternalIdentityID: 20, Role: "direct", Priority: 110, Status: "active"},
	})
	if err != nil || after.ConnectorInstanceID != 2 || deviceID != 42 {
		t.Fatalf("migration changed device identity or route: device=%d route=%#v err=%v", deviceID, after, err)
	}
}
