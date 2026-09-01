import "server-only";

import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { buildDeviceTree, type DeviceTreeItem, type DeviceTreeRelation } from "@/lib/device-tree-core";
import { projectDeviceCapabilities, type DeviceCapabilityGrant } from "@/lib/device-action-projection";

export async function readProjectDeviceTree(projectId: number) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "project:view");
  const [devices, relations, grants] = await Promise.all([
    query<DeviceTreeItem>(
      `select device.id,device.device_type_id::text as "deviceTypeId",device.name,device_type.category,device.status,device.data_freshness as "dataFreshness",
              device.status_reason as "statusReason",device_type.display_name as "typeName",
              device_type.type_key as "typeKey",driver.driver_key as "driverKey",driver.version as "driverVersion",
              device_type.vendor,device_type.model,
              case
                when telemetry.payload_json#>>'{position,validity}'='invalid'
                  and (pose.observation_id is null or telemetry.captured_at>=pose.captured_at) then 'invalid'
                when pose.observation_id is null then 'missing'
                when pose.standard_position is null then 'unverified'
                else 'available'
              end as "positionStatus",
              case
                when telemetry.payload_json#>>'{position,validity}'='invalid'
                  and (pose.observation_id is null or telemetry.captured_at>=pose.captured_at)
                  then coalesce(telemetry.payload_json#>>'{position,reason}','coordinate_invalid')
                when pose.observation_id is null then 'position_missing'
                when pose.standard_position is null then 'coordinate_reference_unverified'
                else null
              end as "positionReason",
              coalesce(telemetry.quality_json->>'source',driver.driver_key) as "positionSource",
              case when pose.observation_id is null then null else json_build_object(
                'longitude',ST_X(coalesce(pose.standard_position,pose.original_position)),
                'latitude',ST_Y(coalesce(pose.standard_position,pose.original_position)),
                'altitudeMeters',ST_Z(coalesce(pose.standard_position,pose.original_position)),
                'capturedAt',pose.captured_at,
                'calibrationStatus',case when pose.standard_position is null then 'unverified' else 'calibrated' end
              ) end as pose,
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
         left join lateral(
           select observation_id,standard_position,original_position,captured_at
             from poses where project_id=device.project_id and device_id=device.id
            order by captured_at desc limit 1
         ) pose on true
         left join device_latest_telemetry telemetry
           on telemetry.project_id=device.project_id and telemetry.device_id=device.id
          and telemetry.telemetry_type='dji.flighthub.state'
        where device.project_id=$1 order by device.id`, [projectId]
    ),
    query<DeviceTreeRelation>(
      `select from_device_id as "fromDeviceId",to_device_id as "toDeviceId",relation_type as "relationType"
         from device_relationships where project_id=$1 and valid_until is null order by valid_from`, [projectId]
    ),
    query<DeviceCapabilityGrant>(
      `select scope_type as "scopeType",device_type_id::text as "deviceTypeId",device_id as "deviceId",
              action_pattern as "actionPattern",effect
         from device_capability_grants
        where project_id=$1 and team_id=$2 and user_id=$3 and (expires_at is null or expires_at>now())`,
      [projectId, access.teamId, user.id]
    )
  ]);
  const authorizedDevices = devices.rows.map((device) => ({ ...device, capabilities: projectDeviceCapabilities({
    deviceId: device.id, deviceTypeId: device.deviceTypeId, deviceStatus: device.status,
    role: access.role, capabilities: device.capabilities, grants: grants.rows
  }) }));
  return buildDeviceTree(authorizedDevices, relations.rows);
}
