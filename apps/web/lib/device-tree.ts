import "server-only";

import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { buildDeviceTree, type DeviceTreeItem, type DeviceTreeRelation } from "@/lib/device-tree-core";
import { actionPatternMatches, authorizeCapabilityAction } from "@/lib/device-command-core";
import { actionsForCapability } from "@/lib/device-capability-actions";

export async function readProjectDeviceTree(projectId: number) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "project:view");
  const [devices, relations, grants] = await Promise.all([
    query<DeviceTreeItem>(
      `select device.id,device.device_type_id::text as "deviceTypeId",device.name,device_type.category,device.status,device.data_freshness as "dataFreshness",
              device.status_reason as "statusReason",device_type.display_name as "typeName",
              device_type.type_key as "typeKey",driver.driver_key as "driverKey",driver.version as "driverVersion",
              device_type.vendor,device_type.model,
              coalesce((select jsonb_agg(jsonb_build_object(
                'code',capability.capability_code,'availability',capability.availability,
                'reason',capability.availability_reason,'risk',capability.risk_level
              ) order by capability.capability_code) from device_capabilities capability
                where capability.project_id=device.project_id and capability.device_id=device.id),'[]') as capabilities,
              coalesce((select jsonb_agg(jsonb_build_object(
                'stableChannelId',channel.stable_channel_id,'name',channel.display_name,
                'dataType',channel.data_type,'availability',channel.availability
              ) order by channel.channel_key) from device_stream_channels channel
                where channel.project_id=device.project_id and channel.device_id=device.id),'[]') as channels
         from devices device
         join device_types device_type on device_type.id=device.device_type_id
         join driver_definitions driver on driver.id=device_type.driver_definition_id
        where device.project_id=$1 order by device.id`, [projectId]
    ),
    query<DeviceTreeRelation>(
      `select from_device_id as "fromDeviceId",to_device_id as "toDeviceId",relation_type as "relationType"
         from device_relationships where project_id=$1 and valid_until is null order by valid_from`, [projectId]
    ),
    query<{ scopeType: "project" | "device_type" | "device"; deviceTypeId: string | null; deviceId: number | null; actionPattern: string; effect: "allow" | "deny" }>(
      `select scope_type as "scopeType",device_type_id::text as "deviceTypeId",device_id as "deviceId",
              action_pattern as "actionPattern",effect
         from device_capability_grants
        where project_id=$1 and team_id=$2 and user_id=$3 and (expires_at is null or expires_at>now())`,
      [projectId, access.teamId, user.id]
    )
  ]);
  const authorizedDevices = devices.rows.map((device) => ({ ...device, capabilities: device.capabilities.map((capability) => {
    const matching = grants.rows.filter((grant) => actionPatternMatches(grant.actionPattern, capability.code)
      && (grant.scopeType === "project" || (grant.scopeType === "device_type" && grant.deviceTypeId === device.deviceTypeId)
        || (grant.scopeType === "device" && grant.deviceId === device.id)));
    let authorized = false;
    try {
      authorized = authorizeCapabilityAction({ role: access.role, action: capability.code, grants: matching });
    } catch {}
    const actions = capability.availability === "available" && authorized
      ? actionsForCapability(capability.code, capability.risk) : [];
    return { ...capability, authorized, actions };
  }) }));
  return buildDeviceTree(authorizedDevices, relations.rows);
}
