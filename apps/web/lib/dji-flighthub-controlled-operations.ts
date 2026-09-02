import "server-only";

import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { buildFlightHubControlledOperations, type ControlledOperationJob } from "@/lib/dji-flighthub-controlled-operations-core";

type AccessRow = { role: string; connectorStatus: string; connectorProjectId: number; connectorTeamId: number;
  manifest: { capabilities?: Array<{ code?: unknown; kind?: unknown }> }; featureFlags: Record<string, unknown>;
  managementGranted: boolean };

function safeFeatureFlags(value: unknown) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return Object.fromEntries(Object.entries(value as Record<string, unknown>).flatMap(([key, enabled]) =>
    /^[a-z][a-z0-9.-]{1,127}$/.test(key) && typeof enabled === "boolean" ? [[key, enabled]] : []));
}

export async function readFlightHubControlledOperations(projectId: number, connectorId: string) {
  if (!Number.isSafeInteger(projectId) || projectId <= 0 || !/^\d+$/.test(connectorId)) throw new Error("FLIGHTHUB_CONTROLLED_OPERATIONS_NOT_FOUND");
  const { user, access } = await requireCurrentProjectPermission(projectId, "project:view");
  const row = (await query<AccessRow>(`select membership.role,adapter.status as "connectorStatus",
      adapter.project_id::int as "connectorProjectId",adapter.team_id::int as "connectorTeamId",
      definition.manifest_json as manifest,coalesce(flags.flighthub_action_flags_json,'{}'::jsonb) as "featureFlags",
      (membership.role='owner' or exists(select 1 from project_permissions permission where permission.project_id=project.id
        and permission.team_id=project.team_id and permission.user_id=$1 and permission.permission='organization:manage')) as "managementGranted"
    from projects project join team_members membership on membership.team_id=project.team_id and membership.user_id=$1
    join device_adapters adapter on adapter.project_id=project.id and adapter.team_id=project.team_id
    join connector_definitions definition on definition.id=adapter.connector_definition_id
      and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
    left join project_feature_flags flags on flags.project_id=project.id
    where project.id=$2 and adapter.id=$3`, [user.id,projectId,connectorId])).rows[0];
  if (!row || row.connectorProjectId !== projectId || row.connectorTeamId !== access.teamId) throw new Error("FLIGHTHUB_CONTROLLED_OPERATIONS_NOT_FOUND");
  const capabilities = await query<{ capabilityCode: string }>(`select distinct capability_code as "capabilityCode"
    from connector_capability_snapshots where project_id=$1 and connector_instance_id=$2 and status='supported'
      and evidence_level='field-write' and (expires_at is null or expires_at>now())`, [projectId,connectorId]);
  const jobs = await query<ControlledOperationJob>(`select * from (
      select id::text,action_kind as action,status,last_error_code as "lastErrorCode",completed_at as "completedAt",updated_at as "updatedAt"
        from connector_action_jobs where project_id=$1 and connector_instance_id=$2
      union all select id::text,action_kind,status,last_error_code,completed_at,updated_at
        from connector_device_admin_jobs where project_id=$1 and connector_instance_id=$2
      union all select id::text,action_kind,status,last_error_code,completed_at,updated_at
        from connector_management_write_jobs where project_id=$1 and connector_instance_id=$2
      union all select id::text,action_kind,status,last_error_code,completed_at,updated_at
        from connector_geospatial_action_jobs where project_id=$1 and connector_instance_id=$2
      union all select id::text,action_kind,status,last_error_code,completed_at,updated_at
        from connector_model_delete_jobs where project_id=$1 and connector_instance_id=$2
      union all select id::text,action_kind,status,last_error_code,completed_at,updated_at
        from connector_live_action_jobs where project_id=$1 and connector_instance_id=$2
    ) jobs order by "updatedAt" desc limit 50`, [projectId,connectorId]);
  const manifestCapabilities = new Set((Array.isArray(row.manifest?.capabilities) ? row.manifest.capabilities : [])
    .flatMap((item) => item?.kind === "action" && typeof item.code === "string" ? [item.code] : []));
  return buildFlightHubControlledOperations({ projectId, connectorStatus: row.connectorStatus, role: row.role,
    permissions: access.permissions, managementGranted: row.managementGranted, manifestCapabilities,
    featureFlags: safeFeatureFlags(row.featureFlags), fieldWriteCapabilities: new Set(capabilities.rows.map((item) => item.capabilityCode)),
    jobs: jobs.rows });
}
