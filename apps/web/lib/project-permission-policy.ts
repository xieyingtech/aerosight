export const PROJECT_PERMISSIONS = [
  "project:view",
  "event:handle",
  "mission:operate",
  "mission:approve",
  "device:configure",
  "safety:manage",
  "algorithm:manage",
  "agent:use",
  "report:export"
] as const;

export type ProjectPermission = (typeof PROJECT_PERMISSIONS)[number];
export type ProjectTeamRole = "owner" | "admin" | "member";

const allPermissions = new Set<ProjectPermission>(PROJECT_PERMISSIONS);

export function isProjectPermission(value: string): value is ProjectPermission {
  return allPermissions.has(value as ProjectPermission);
}

export function effectiveProjectPermissions(
  role: ProjectTeamRole,
  explicitPermissions: readonly string[] = []
) {
  if (role === "owner" || role === "admin") {
    return new Set<ProjectPermission>(PROJECT_PERMISSIONS);
  }

  const permissions = new Set<ProjectPermission>(["project:view"]);
  for (const permission of explicitPermissions) {
    if (isProjectPermission(permission)) permissions.add(permission);
  }
  return permissions;
}
