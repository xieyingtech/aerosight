package device

import (
	"testing"
	"time"
)

func authorizationFixture(action string) (AuthorizationSubject, CapabilityTarget) {
	return AuthorizationSubject{UserID: 7, ProjectID: 17, MembershipAlive: true, Role: "member"}, CapabilityTarget{
		ProjectID: 17, DeviceID: "sensor-1", DeviceType: TypeReference{Key: "environment.sensor", Version: 1}, Action: action, Available: true,
	}
}

func TestReadOnlySensorGrantDoesNotExpandToConfiguration(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	subject, target := authorizationFixture("stream.sensor.read")
	grants := []CapabilityGrant{{ProjectID: 17, UserID: 7, Scope: GrantScopeDeviceType, DeviceType: target.DeviceType, Action: "stream.sensor.read", Effect: GrantAllow}}
	if !AuthorizeCapability(subject, target, grants, now).Allowed {
		t.Fatal("sensor read grant was denied")
	}
	target.Action = "sensor.configure"
	if decision := AuthorizeCapability(subject, target, grants, now); decision.Allowed {
		t.Fatalf("read-only grant expanded to configuration: %+v", decision)
	}
}

func TestDeviceGrantControlsOnlyTheNamedAircraft(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	subject, target := authorizationFixture("mission.execute")
	target.DeviceID = "aircraft-1"
	target.DeviceType = TypeReference{Key: "dji.m3d", Version: 1}
	grants := []CapabilityGrant{{ProjectID: 17, UserID: 7, Scope: GrantScopeDevice, DeviceID: "aircraft-1", Action: "mission.execute", Effect: GrantAllow}}
	if !AuthorizeCapability(subject, target, grants, now).Allowed {
		t.Fatal("explicitly granted aircraft was denied")
	}
	target.DeviceID = "aircraft-2"
	if AuthorizeCapability(subject, target, grants, now).Allowed {
		t.Fatal("device-scoped control leaked to another aircraft")
	}
}

func TestRevocationExpiryAndDenyOverrideRoleDefaults(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	subject, target := authorizationFixture("stream.video.read")
	expires := now.Add(time.Minute)
	grant := CapabilityGrant{ProjectID: 17, UserID: 7, Scope: GrantScopeProject, Action: "stream.*", Effect: GrantAllow, ExpiresAt: &expires}
	if !AuthorizeCapability(subject, target, []CapabilityGrant{grant}, now).Allowed {
		t.Fatal("active project stream grant was denied")
	}
	if AuthorizeCapability(subject, target, nil, now).Allowed {
		t.Fatal("revoked grant remained effective")
	}
	if AuthorizeCapability(subject, target, []CapabilityGrant{grant}, expires).Allowed {
		t.Fatal("expired grant remained effective")
	}
	subject.Role = "admin"
	deny := CapabilityGrant{ProjectID: 17, UserID: 7, Scope: GrantScopeDevice, DeviceID: target.DeviceID, Action: "stream.video.read", Effect: GrantDeny}
	if decision := AuthorizeCapability(subject, target, []CapabilityGrant{deny}, now); decision.Allowed || decision.Reason != "CAPABILITY_EXPLICITLY_DENIED" {
		t.Fatalf("explicit deny did not override role default: %+v", decision)
	}
}

func TestDeviceTypeGrantDoesNotSurviveUnreviewedTypeMigration(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	subject, target := authorizationFixture("mission.execute")
	target.DeviceType = TypeReference{Key: "dji.m4d", Version: 1}
	grant := CapabilityGrant{ProjectID: 17, UserID: 7, Scope: GrantScopeDeviceType, DeviceType: target.DeviceType, Action: "mission.execute", Effect: GrantAllow}
	if !AuthorizeCapability(subject, target, []CapabilityGrant{grant}, now).Allowed {
		t.Fatal("matching device type grant was denied")
	}
	target.DeviceType.Version = 2
	if AuthorizeCapability(subject, target, []CapabilityGrant{grant}, now).Allowed {
		t.Fatal("device type migration silently expanded an old grant")
	}
}

func TestAuthorizationFailsClosedForScopeAndUnavailableCapability(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	subject, target := authorizationFixture("state.read")
	if !AuthorizeCapability(subject, target, nil, now).Allowed {
		t.Fatal("member role did not receive state.read default")
	}
	target.Available = false
	if AuthorizeCapability(subject, target, nil, now).Allowed {
		t.Fatal("unavailable capability was authorized")
	}
	target.Available = true
	target.ProjectID = 18
	if AuthorizeCapability(subject, target, nil, now).Allowed {
		t.Fatal("cross-project capability was authorized")
	}
}
