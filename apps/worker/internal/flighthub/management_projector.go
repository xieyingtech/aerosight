package flighthub

import (
	"context"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
)

func (sink *SQLResourceStreamSink) ApplyManagementCatalog(ctx context.Context, instance connector.Instance, poll ManagementCatalogPoll) error {
	if err := sink.resources.AssertWritable(ctx, instance); err != nil {
		return err
	}
	batches, err := managementRemoteResourceBatches(poll)
	if err != nil {
		return err
	}
	for _, batch := range batches {
		if _, err := sink.resources.ApplyRemoteResources(ctx, instance, batch); err != nil {
			return err
		}
	}
	return nil
}

func managementRemoteResourceBatches(poll ManagementCatalogPoll) ([]connector.RemoteResourceBatch, error) {
	organizationSummary := map[string]any{
		"name": poll.Organization.Name, "status": poll.Organization.Status,
		"industryType": poll.Organization.IndustryType.Type, "industrySubtype": poll.Organization.IndustryType.Subtype,
		"measureUnits": poll.Organization.UnitsSystem.Measure, "temperatureUnits": poll.Organization.UnitsSystem.Temperature,
		"mfaEnabled": poll.Organization.MFAEnabled, "currentUserRole": poll.Organization.UserRole,
		"source": "dji-flighthub-openapi",
	}
	organizationVersion, err := managementSummaryVersion(organizationSummary)
	if err != nil {
		return nil, err
	}
	organizationUpdatedAt := managementUnixTime(poll.Organization.UpdatedAt)
	organizations := []connector.RemoteResource{{
		RemoteID: secureRemoteKey(poll.Organization.UUID), RemoteVersion: organizationVersion,
		RemoteUpdatedAt: organizationUpdatedAt, Summary: organizationSummary,
	}}

	organizationUsers := make([]connector.RemoteResource, 0, len(poll.OrganizationUsers)+1)
	currentSummary := map[string]any{
		"scope": "current-identity", "account": poll.CurrentRole.Account, "nickname": poll.CurrentRole.Nickname,
		"role": poll.CurrentRole.Role, "organizationName": poll.CurrentRole.OrganizationName,
		"mfaEnabled": poll.CurrentRole.MFAEnabled, "source": "dji-flighthub-openapi",
	}
	currentVersion, err := managementSummaryVersion(currentSummary)
	if err != nil {
		return nil, err
	}
	organizationUsers = append(organizationUsers, connector.RemoteResource{
		RemoteID: "current:" + secureRemoteKey(poll.CurrentRole.UserID), RemoteVersion: currentVersion, Summary: currentSummary,
	})
	seenOrganizationUsers := make(map[string]struct{}, len(poll.OrganizationUsers))
	for _, user := range poll.OrganizationUsers {
		remoteID := secureRemoteKey(user.UserID)
		if _, duplicate := seenOrganizationUsers[remoteID]; duplicate {
			return nil, schemaError()
		}
		seenOrganizationUsers[remoteID] = struct{}{}
		summary := map[string]any{
			"scope": "organization", "account": user.Account, "accountSecond": user.AccountSecond,
			"nickname": user.Nickname, "role": user.Role, "sourceType": user.SourceType,
			"projectCount": len(user.Projects), "source": "dji-flighthub-openapi",
		}
		version, versionErr := managementSummaryVersion(summary)
		if versionErr != nil {
			return nil, versionErr
		}
		organizationUsers = append(organizationUsers, connector.RemoteResource{
			RemoteID: remoteID, RemoteVersion: version, RemoteUpdatedAt: managementUnixTime(user.CreatedAt), Summary: summary,
		})
	}

	roles := make([]connector.RemoteResource, 0, len(poll.OrganizationRoles))
	roleSeen := make(map[string]struct{}, len(poll.OrganizationRoles))
	for _, role := range poll.OrganizationRoles {
		remoteID := secureRemoteKey(role.RoleID)
		if _, duplicate := roleSeen[remoteID]; duplicate {
			return nil, schemaError()
		}
		roleSeen[remoteID] = struct{}{}
		summary := map[string]any{
			"name": role.RoleName, "description": role.RoleDescription, "roleType": role.RoleType,
			"preset": role.Preset, "addToOrganization": role.AddToOrg, "permissionCount": countPermissions(role.Permissions),
			"source": "dji-flighthub-openapi",
		}
		version, versionErr := managementSummaryVersion(summary)
		if versionErr != nil {
			return nil, versionErr
		}
		roles = append(roles, connector.RemoteResource{RemoteID: remoteID, RemoteVersion: version, RemoteUpdatedAt: managementRFC3339Time(role.UpdatedAt), Summary: summary})
	}

	permissions, err := permissionRemoteResources("catalog", poll.OrganizationPermissions)
	if err != nil {
		return nil, err
	}
	rolePermissions, err := permissionRemoteResources("role-effective", poll.RolePermissions)
	if err != nil {
		return nil, err
	}
	permissions = append(permissions, rolePermissions...)

	projectUsers := make([]connector.RemoteResource, 0, len(poll.ProjectUsers))
	projectUserSeen := make(map[string]struct{}, len(poll.ProjectUsers))
	for _, user := range poll.ProjectUsers {
		remoteID := secureRemoteKey(user.UserID)
		if _, duplicate := projectUserSeen[remoteID]; duplicate {
			return nil, schemaError()
		}
		projectUserSeen[remoteID] = struct{}{}
		summary := map[string]any{
			"account": user.Account, "nickname": user.Nickname, "callsign": user.OrganizationUserCallsign,
			"projectRole": user.Role, "organizationRole": user.OrganizationRole,
			"callsignUpdated": user.ProjectCallsignUpdated, "phoneFilled": user.PhoneFilled, "emailFilled": user.EmailFilled,
			"source": "dji-flighthub-openapi",
		}
		version, versionErr := managementSummaryVersion(summary)
		if versionErr != nil {
			return nil, versionErr
		}
		projectUsers = append(projectUsers, connector.RemoteResource{RemoteID: remoteID, RemoteVersion: version, Summary: summary})
	}

	projectMembers := make([]connector.RemoteResource, 0, len(poll.ProjectMembers))
	projectMemberSeen := make(map[string]struct{}, len(poll.ProjectMembers))
	for _, member := range poll.ProjectMembers {
		remoteID := secureRemoteKey(member.UserID)
		if _, duplicate := projectMemberSeen[remoteID]; duplicate {
			return nil, schemaError()
		}
		projectMemberSeen[remoteID] = struct{}{}
		// Deliberately omit offline position and controlled-device SN from the management projection.
		summary := map[string]any{
			"account": member.Account, "projectCallsign": member.ProjectCallsign, "projectRole": member.ProjectRole,
			"organizationCallsign": member.OrganizationCallsign, "organizationRole": member.OrganizationRole,
			"online": member.Online, "pendingOffline": member.PendingOffline, "platform": member.Platform,
			"source": "dji-flighthub-openapi",
		}
		version, versionErr := managementSummaryVersion(summary)
		if versionErr != nil {
			return nil, versionErr
		}
		projectMembers = append(projectMembers, connector.RemoteResource{RemoteID: remoteID, RemoteVersion: version, Summary: summary})
	}

	return []connector.RemoteResourceBatch{
		{Kind: "organization", Resources: organizations, CompleteSnapshot: true},
		{Kind: "organization-user", Resources: organizationUsers, CompleteSnapshot: true},
		{Kind: "organization-role", Resources: roles, CompleteSnapshot: true},
		{Kind: "organization-permission", Resources: permissions, CompleteSnapshot: true},
		{Kind: "project-user", Resources: projectUsers, CompleteSnapshot: true},
		{Kind: "project-member", Resources: projectMembers, CompleteSnapshot: true},
	}, nil
}

func permissionRemoteResources(source string, items []OrganizationPermission) ([]connector.RemoteResource, error) {
	resources := make([]connector.RemoteResource, 0)
	seen := make(map[string]struct{})
	var walk func([]OrganizationPermission, string, int) error
	walk = func(children []OrganizationPermission, parent string, depth int) error {
		if depth > 16 || len(resources)+len(children) > 10000 {
			return schemaError()
		}
		for _, permission := range children {
			key := strings.TrimSpace(permission.PermissionID)
			if key == "" {
				return schemaError()
			}
			remoteID := source + ":" + secureRemoteKey(key)
			if _, duplicate := seen[remoteID]; duplicate {
				return schemaError()
			}
			seen[remoteID] = struct{}{}
			summary := map[string]any{
				"sourceScope": source, "name": permission.PermissionName, "description": permission.PermissionDescription,
				"permissionType": permission.PermissionType, "level": permission.Level, "visible": permission.Visible,
				"basic": permission.Basic, "childCount": len(permission.Children), "source": "dji-flighthub-openapi",
			}
			if parent != "" {
				summary["parentReference"] = secureRemoteKey(parent)
			}
			version, err := managementSummaryVersion(summary)
			if err != nil {
				return err
			}
			resources = append(resources, connector.RemoteResource{RemoteID: remoteID, RemoteVersion: version, RemoteUpdatedAt: managementRFC3339Time(permission.UpdatedAt), Summary: summary})
			if err := walk(permission.Children, key, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(items, "", 0); err != nil {
		return nil, err
	}
	return resources, nil
}

func countPermissions(items []OrganizationPermission) int {
	count := 0
	var walk func([]OrganizationPermission)
	walk = func(children []OrganizationPermission) {
		for _, child := range children {
			count++
			walk(child.Children)
		}
	}
	walk(items)
	return count
}

func managementSummaryVersion(summary map[string]any) (string, error) {
	if len(summary) > 32 {
		return "", schemaError()
	}
	for _, value := range summary {
		if text, ok := value.(string); ok && len(text) > 1024 {
			return "", schemaError()
		}
	}
	return modelVersion(summary)
}

func managementUnixTime(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	var parsed time.Time
	if value < 10_000_000_000 {
		parsed = time.Unix(value, 0).UTC()
	} else {
		parsed = time.UnixMilli(value).UTC()
	}
	return &parsed
}

func managementRFC3339Time(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}
