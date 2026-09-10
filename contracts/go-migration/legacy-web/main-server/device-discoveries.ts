import "server-only";

import { correlationId } from "@/lib/observability";
import { withAuditedProjectWrite } from "@/lib/audit";
import { canManageDeviceAdapters } from "@/lib/device-adapter-policy";
import type { DeviceDiscovery, DeviceTypeOption, DiscoveryConnector, DiscoveryStatus } from "@/lib/device-discovery-core";
import { query } from "@/lib/db";
import { requireCurrentProjectPermission } from "@/lib/data";
import { assertFlightHubConnectorEnabled } from "@/lib/dji-flighthub-lifecycle-core";

export async function readProjectDiscoveries(projectId: number) {
  const { access } = await requireCurrentProjectPermission(projectId, "project:view");
  const [discoveries, deviceTypes, connectors] = await Promise.all([
    query<DeviceDiscovery>(`
      select identity.id::text,identity.adapter_id::text as "connectorId",adapter.name as "connectorName",
             definition.connector_key as "connectorKey",identity.external_device_id as "externalDeviceId",
             identity.external_device_type as "externalDeviceType",
             nullif(identity.identity_json->>'parentExternalId','') as "parentExternalId",
             identity.discovery_status as status,type.type_key as "suggestedTypeKey",
             type.display_name as "suggestedTypeName",identity.match_confidence::float8 as "matchConfidence",
             identity.device_id as "deviceId",identity.last_seen_at::text as "lastSeenAt"
        from device_external_identities identity
        join device_adapters adapter on adapter.id=identity.adapter_id and adapter.project_id=identity.project_id
        join connector_definitions definition on definition.id=adapter.connector_definition_id
        left join device_types type on type.id=identity.suggested_device_type_id
       where identity.project_id=$1
       order by case identity.discovery_status when 'conflicted' then 0 when 'discovered' then 1 when 'missing' then 2 when 'managed' then 3 else 4 end,
                identity.last_seen_at desc,identity.id`, [projectId]),
    query<DeviceTypeOption>(`select id::text,type_key as "typeKey",display_name as "displayName",category
      from device_types where status='active' order by display_name,version desc`, []),
    query<DiscoveryConnector>(`select adapter.id::text,adapter.name,definition.connector_key as "connectorKey",adapter.status,
      (definition.connector_key='dji.flighthub2' and definition.status='active' and adapter.status in('connecting','connected','degraded')) as "canScan"
      from device_adapters adapter join connector_definitions definition on definition.id=adapter.connector_definition_id
      where adapter.project_id=$1 order by adapter.name`, [projectId]),
  ]);
  return { discoveries: discoveries.rows, deviceTypes: deviceTypes.rows, connectors: connectors.rows, canManage: canManageDeviceAdapters(access.role) };
}

export async function updateDiscoveryStatus(
  projectId: number,
  identityId: number,
  action: "ignore" | "review" | "rematch",
  requestId?: string | null
) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "device:configure");
  if (!canManageDeviceAdapters(access.role)) throw new Error("PROJECT_ACCESS_DENIED");
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, actorUserId: user.id, requestId: correlationId(requestId),
    action: `device_identity.${action}`, resourceType: "device_external_identity", resourceId: String(identityId),
    input: { action }, policyResult: { permission: "device:configure", role: access.role },
  }, async (client) => {
    const identity = (await client.query<{ status: DiscoveryStatus; deviceId: number | null }>(
      `select discovery_status as status,device_id as "deviceId" from device_external_identities
        where project_id=$1 and id=$2 for update`, [projectId, identityId]
    )).rows[0];
    if (!identity) throw new Error("DEVICE_IDENTITY_NOT_FOUND");
    if (identity.deviceId || identity.status === "managed") throw new Error("MANAGED_IDENTITY_IMMUTABLE");
    if (action === "ignore") {
      await client.query(`update device_external_identities set discovery_status='ignored'
        where project_id=$1 and id=$2`, [projectId, identityId]);
      return { id: identityId, status: "ignored" as const };
    }
    const matched = (await client.query<{ id: string; typeKey: string }>(`select type.id::text,type.type_key as "typeKey"
      from device_external_identities identity join device_types type on type.type_key=identity.external_device_type
      where identity.project_id=$1 and identity.id=$2 and type.status='active'
      order by type.version desc limit 1`, [projectId, identityId])).rows[0];
    const nextStatus: DiscoveryStatus = identity.status === "conflicted" ? "conflicted" : "discovered";
    await client.query(`update device_external_identities
      set suggested_device_type_id=$3,match_confidence=case when $3::bigint is null then null else 1 end,discovery_status=$4
      where project_id=$1 and id=$2`, [projectId, identityId, matched?.id ?? null, nextStatus]);
    return { id: identityId, status: nextStatus, typeKey: matched?.typeKey ?? null, confidence: matched ? 1 : null };
  });
}

export async function bindDiscoveredDeviceRecord(
  projectId: number,
  identityId: number,
  rawInput: { name?: unknown; deviceTypeKey?: unknown; targetDeviceId?: unknown },
  requestId?: string | null
) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "device:configure");
  if (!canManageDeviceAdapters(access.role)) throw new Error("PROJECT_ACCESS_DENIED");
  const name = typeof rawInput.name === "string" ? rawInput.name.trim() : "";
  const requestedTypeKey = typeof rawInput.deviceTypeKey === "string" ? rawInput.deviceTypeKey.trim() : "";
  const targetDeviceId = Number(rawInput.targetDeviceId || 0);
  if (!name || name.length > 100 || !requestedTypeKey) throw new Error("INVALID_DEVICE_BINDING");
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, actorUserId: user.id, requestId: correlationId(requestId),
    action: targetDeviceId > 0 ? "device_identity.migrate" : "device_identity.bind",
    resourceType: "device_external_identity", resourceId: String(identityId),
    input: { name, deviceTypeKey: requestedTypeKey, targetDeviceId: targetDeviceId || null },
    policyResult: { permission: "device:configure", role: access.role },
  }, async (client) => {
    const identity = (await client.query<{
		id: string; adapterId: string; deviceId: number | null; status: DiscoveryStatus;
		parentExternalId: string | null; teamId: number; connectorStatus: string;
	}>(`select id::text,adapter_id::text as "adapterId",device_id as "deviceId",discovery_status as status,
		nullif(identity.identity_json->>'parentExternalId','') as "parentExternalId",identity.team_id as "teamId",
		adapter.status as "connectorStatus"
		from device_external_identities identity
		join device_adapters adapter on adapter.id=identity.adapter_id and adapter.project_id=identity.project_id
		where identity.project_id=$1 and identity.id=$2 for update of identity,adapter`, [projectId, identityId])).rows[0];
	if (!identity) throw new Error("DEVICE_IDENTITY_NOT_FOUND");
	try { assertFlightHubConnectorEnabled(identity.connectorStatus); }
	catch { throw new Error("CONNECTOR_DISABLED"); }
    if (identity.deviceId) return { deviceId: identity.deviceId, replayed: true };
    if (identity.status !== "discovered") throw new Error("DEVICE_IDENTITY_NOT_CONFIRMABLE");
    const type = (await client.query<{ id: string; category: string }>(`select id::text,category from device_types
      where type_key=$1 and status='active' order by version desc limit 1`, [requestedTypeKey])).rows[0];
    if (!type) throw new Error("DEVICE_TYPE_NOT_AVAILABLE");
    let deviceId = targetDeviceId;
    if (deviceId > 0) {
      const target = await client.query(`select id from devices where project_id=$1 and id=$2 for update`, [projectId, deviceId]);
      if (!target.rows[0]) throw new Error("TARGET_DEVICE_NOT_FOUND");
      await client.query(`update device_connector_bindings set status='standby'
        where project_id=$1 and device_id=$2 and status='active'`, [projectId, deviceId]);
    } else {
      const created = await client.query<{ id: number }>(`insert into devices(
        project_id,adapter_id,device_type_id,name,type,status,metadata_json
      ) values($1,$2,$3,$4,$5,'unknown',$6) returning id`,
      [projectId, identity.adapterId, type.id, name, type.category, { identityId, onboarding: "review" }]);
      deviceId = created.rows[0].id;
    }
    await client.query(`update devices set adapter_id=$3,updated_at=now() where project_id=$1 and id=$2`,
      [projectId, deviceId, identity.adapterId]);
    await client.query(`update device_external_identities set device_id=$3,suggested_device_type_id=$4,
      match_confidence=1,discovery_status='managed',bound_at=now()
      where project_id=$1 and id=$2 and device_id is null`, [projectId, identityId, deviceId, type.id]);
    const role = identity.parentExternalId ? "inherited" : type.category === "dock" || type.category === "gateway" ? "gateway" : "direct";
    await client.query(`insert into device_connector_bindings(
      project_id,team_id,device_id,connector_instance_id,external_identity_id,route_role,priority,status,metadata_json
    ) values($1,$2,$3,$4,$5,$6,
      coalesce((select max(priority)+10 from device_connector_bindings where project_id=$1 and device_id=$3),100),
      'active',$7)
    on conflict(device_id,connector_instance_id) do update set external_identity_id=excluded.external_identity_id,
      route_role=excluded.route_role,priority=excluded.priority,status='active',unbound_at=null,metadata_json=excluded.metadata_json`,
    [projectId, identity.teamId, deviceId, identity.adapterId, identityId, role, { source: "review-onboarding" }]);
    await client.query(`insert into device_capabilities(
      device_id,project_id,capability_code,version,declared_by_adapter_id,params_schema_json,
      input_schema_json,output_schema_json,risk_level,source_json
    ) select $1,$2,capability->>'code',driver.version,$3,
      coalesce(capability->'inputSchema','{}'),coalesce(capability->'inputSchema','{}'),
      coalesce(capability->'outputSchema','{}'),coalesce(capability->>'risk','low'),
      jsonb_build_object('driver',driver.driver_key,'typeKey',type.type_key)
      from device_types type join driver_definitions driver on driver.id=type.driver_definition_id
      cross join lateral jsonb_array_elements(case when jsonb_typeof(driver.manifest_json->'capabilities')='array'
        then driver.manifest_json->'capabilities' else '[]'::jsonb end) capability
      where type.id=$4 and type.capability_profile_json ? (capability->>'code')
      on conflict(device_id,capability_code) do update set declared_by_adapter_id=excluded.declared_by_adapter_id,
        availability='available',availability_reason=null,updated_at=now()`, [deviceId, projectId, identity.adapterId, type.id]);
    if (identity.parentExternalId) await client.query(`insert into device_relationships(
      project_id,team_id,from_device_id,to_device_id,relation_type,source_type,metadata_json
    ) select $1,$2,parent.device_id,$3,'contains','discovery',$6
      from device_external_identities parent where parent.project_id=$1 and parent.adapter_id=$4
        and parent.external_device_id=$5 and parent.device_id is not null
        and not exists(select 1 from device_relationships relation where relation.project_id=$1
          and relation.from_device_id=parent.device_id and relation.to_device_id=$3 and relation.valid_until is null)`,
    [projectId, identity.teamId, deviceId, identity.adapterId, identity.parentExternalId, { source: "review-onboarding" }]);
    return { deviceId, replayed: false, migrated: targetDeviceId > 0 };
  });
}
