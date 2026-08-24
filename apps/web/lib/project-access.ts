import "server-only";

import { query } from "@/lib/db";
import {
  effectiveProjectPermissions,
  type ProjectPermission,
  type ProjectTeamRole
} from "@/lib/project-permission-policy";

export type ProjectAccess = {
  projectId: number;
  teamId: number;
  role: ProjectTeamRole;
  permissions: Set<ProjectPermission>;
};

type ProjectAccessRow = {
  projectId: number;
  teamId: number;
  role: ProjectTeamRole;
  explicitPermissions: string[];
};

export async function resolveProjectAccess(userId: number, projectId: number): Promise<ProjectAccess | null> {
  const result = await query<ProjectAccessRow>(
    `select project.id as "projectId", project.team_id as "teamId", membership.role,
            coalesce(array_agg(permission.permission) filter (where permission.permission is not null), '{}')
              as "explicitPermissions"
       from projects project
       join team_members membership
         on membership.team_id = project.team_id and membership.user_id = $1
       left join project_permissions permission
         on permission.project_id = project.id
        and permission.team_id = project.team_id
        and permission.user_id = membership.user_id
      where project.id = $2
      group by project.id, project.team_id, membership.role`,
    [userId, projectId]
  );
  const row = result.rows[0];
  if (!row) return null;
  return {
    projectId: row.projectId,
    teamId: row.teamId,
    role: row.role,
    permissions: effectiveProjectPermissions(row.role, row.explicitPermissions)
  };
}

export async function requireProjectPermissionForUser(
  userId: number,
  projectId: number,
  permission: ProjectPermission
) {
  const access = await resolveProjectAccess(userId, projectId);
  if (!access?.permissions.has(permission)) throw new Error("PROJECT_ACCESS_DENIED");
  return access;
}
