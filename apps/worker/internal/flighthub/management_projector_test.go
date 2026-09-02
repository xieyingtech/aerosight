package flighthub

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestManagementProjectionIsCompleteStableAndSecretMinimal(t *testing.T) {
	t.Parallel()
	poll := ManagementCatalogPoll{
		Organization:            Organization{UUID: "ORG_VENDOR_ID_REDACTED", OrganizationID: "ORG_CODE_REDACTED", Name: "脱敏组织", Status: "active", UserRole: "organization-admin"},
		CurrentRole:             CurrentOrganizationRole{UserID: "CURRENT_USER_VENDOR_ID_REDACTED", OrganizationUUID: "ORG_VENDOR_ID_REDACTED", Role: "organization-admin", OrganizationName: "脱敏组织", AuthInfo: json.RawMessage(`{"secret":"MUST_NOT_PROJECT"}`)},
		OrganizationUsers:       []OrganizationUser{{UserID: "ORG_USER_VENDOR_ID_REDACTED", Account: "account@example.invalid", Nickname: "测试用户", Role: "organization-member", Projects: []OrganizationUserProject{{ProjectUUID: "PROJECT_VENDOR_ID_REDACTED", ProjectCallsign: "测试项目"}}}},
		OrganizationRoles:       []OrganizationRole{{RoleID: "ROLE_VENDOR_ID_REDACTED", RoleName: "组织管理员", RoleType: "organization", Permissions: []OrganizationPermission{{PermissionID: "PERMISSION_VENDOR_ID_REDACTED", PermissionName: "项目管理"}}}},
		OrganizationPermissions: []OrganizationPermission{{PermissionID: "PERMISSION_VENDOR_ID_REDACTED", PermissionName: "项目管理", PermissionType: "organization"}},
		RolePermissions:         []OrganizationPermission{{PermissionID: "ROLE_PERMISSION_VENDOR_ID_REDACTED", PermissionName: "角色权限", PermissionType: "organization"}},
		ProjectUsers:            []ProjectUser{{UserID: "PROJECT_USER_VENDOR_ID_REDACTED", Account: "project@example.invalid", Nickname: "项目用户", Role: "project-member", OrganizationRole: "organization-member", RolesPermissions: json.RawMessage(`{"secret":"MUST_NOT_PROJECT"}`)}},
		ProjectMembers:          []ProjectMember{{UserID: "PROJECT_MEMBER_VENDOR_ID_REDACTED", Account: "member@example.invalid", ProjectCallsign: "现场成员", ProjectRole: "project-member", Online: true, Platform: "web", OfflinePosition: &OfflinePosition{Latitude: 31.2, Longitude: 121.4}, ControlDeviceSN: ptrString("AIRCRAFT_SECRET_SN")}},
		ReceivedAt:              time.Unix(0, 0).UTC(),
	}
	first, err := managementRemoteResourceBatches(poll)
	if err != nil {
		t.Fatal(err)
	}
	second, err := managementRemoteResourceBatches(poll)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) || len(first) != 6 {
		t.Fatalf("management projection is not stable or complete: %s", firstJSON)
	}
	serialized := string(firstJSON)
	for _, forbidden := range []string{
		"ORG_VENDOR_ID_REDACTED", "ORG_CODE_REDACTED", "CURRENT_USER_VENDOR_ID_REDACTED",
		"ORG_USER_VENDOR_ID_REDACTED", "PROJECT_VENDOR_ID_REDACTED", "ROLE_VENDOR_ID_REDACTED",
		"PERMISSION_VENDOR_ID_REDACTED", "PROJECT_USER_VENDOR_ID_REDACTED", "PROJECT_MEMBER_VENDOR_ID_REDACTED",
		"AIRCRAFT_SECRET_SN", "MUST_NOT_PROJECT", "121.4", "31.2",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("management projection leaked %q", forbidden)
		}
	}
	wanted := map[string]int{"organization": 1, "organization-user": 2, "organization-role": 1, "organization-permission": 2, "project-user": 1, "project-member": 1}
	for _, batch := range first {
		if len(batch.Resources) != wanted[batch.Kind] || !batch.CompleteSnapshot {
			t.Fatalf("batch %#v", batch)
		}
		delete(wanted, batch.Kind)
	}
	if len(wanted) != 0 {
		t.Fatalf("missing management kinds %#v", wanted)
	}
}

func TestManagementProjectionRejectsDuplicateVendorIdentities(t *testing.T) {
	t.Parallel()
	poll := ManagementCatalogPoll{
		Organization: Organization{UUID: "ORG_REDACTED", Name: "组织"},
		CurrentRole:  CurrentOrganizationRole{UserID: "CURRENT_REDACTED"},
		ProjectUsers: []ProjectUser{{UserID: "DUPLICATE", Role: "member"}, {UserID: "DUPLICATE", Role: "member"}},
	}
	if _, err := managementRemoteResourceBatches(poll); err == nil {
		t.Fatal("expected duplicate identity to fail closed")
	}
	poll.ProjectUsers = []ProjectUser{{UserID: "UNIQUE", Account: strings.Repeat("x", 1025), Role: "member"}}
	if _, err := managementRemoteResourceBatches(poll); err == nil {
		t.Fatal("expected oversized management summary to fail closed")
	}
}

func ptrString(value string) *string { return &value }
