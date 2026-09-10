import "server-only";

import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { applyFlightHubDevicePrerequisites, buildDeviceTree, type DeviceTreeItem, type DeviceTreeRelation } from "@/lib/device-tree-core";
import { projectDeviceCapabilities, type DeviceCapabilityGrant } from "@/lib/device-action-projection";

export async function readProjectDeviceTree(projectId: number) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "project:view");
  const [devices, relations, grants] = await Promise.all([
    query<DeviceTreeItem>(
      `select device.id,device.device_type_id::text as "deviceTypeId",device.name,device_type.category,device.status,device.data_freshness as "dataFreshness",
              device.status_reason as "statusReason",device_type.display_name as "typeName",
              device_type.type_key as "typeKey",driver.driver_key as "driverKey",driver.version as "driverVersion",
              device_type.vendor,device_type.model,
              case when flighthub_route.id is null then null else json_build_object(
                'connectorStatus',case when flighthub_route.priority_count>1 then 'route_conflict'
                  when flighthub_route.connector_key<>'dji.flighthub2' or flighthub_route.version<>'1.0.0' then 'not_primary'
                  else flighthub_route.status end,
                'stateFresh',coalesce(telemetry.captured_at>now()-interval '30 seconds' and telemetry.captured_at<=now()+interval '1 second',false),
                'cameraFeatureEnabled',coalesce(flags.flighthub_action_flags_json @> '{"flighthub.camera.change":true}'::jsonb,false),
                'cameraFieldVerified',exists(select 1 from connector_capability_snapshots capability
                  where capability.project_id=device.project_id and capability.connector_instance_id=flighthub_route.id
                    and capability.capability_code='device.camera.change' and capability.status='supported'
					and capability.account_fingerprint=flighthub_route.discovery_scope_json->>'accountFingerprint'
					and capability.region='cn' and capability.deployment='cn-public-cloud'
                    and capability.evidence_level='field-write' and capability.device_model=device.device_model
                    and capability.firmware_version=device.firmware_version and (capability.expires_at is null or capability.expires_at>now())),
                'lensFeatureEnabled',coalesce(flags.flighthub_action_flags_json @> '{"flighthub.lens.change":true}'::jsonb,false),
                'lensFieldVerified',exists(select 1 from connector_capability_snapshots capability
                  where capability.project_id=device.project_id and capability.connector_instance_id=flighthub_route.id
                    and capability.capability_code='device.lens.change' and capability.status='supported'
					and capability.account_fingerprint=flighthub_route.discovery_scope_json->>'accountFingerprint'
					and capability.region='cn' and capability.deployment='cn-public-cloud'
                    and capability.evidence_level='field-write' and capability.device_model=device.device_model
                    and capability.firmware_version=device.firmware_version and (capability.expires_at is null or capability.expires_at>now())),
                'tcaState',coalesce(tca.state,'missing'),'tcaCheckedAt',tca.verified_at,'tcaItemCount',tca.item_count
              ) end as "flightHubControl",
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
        left join lateral(
          select adapter.id,adapter.status,definition.connector_key,definition.version,
            (select count(*)::int from device_connector_bindings peer
              where peer.project_id=binding.project_id and peer.device_id=binding.device_id
                and peer.status='active' and peer.priority=binding.priority) as priority_count
            from device_connector_bindings binding
            join device_adapters adapter on adapter.id=binding.connector_instance_id and adapter.project_id=binding.project_id
            join connector_definitions definition on definition.id=adapter.connector_definition_id
           where binding.project_id=device.project_id and binding.device_id=device.id and binding.status='active'
           order by binding.priority desc,binding.connector_instance_id limit 1
        ) flighthub_route on true
        left join project_feature_flags flags on flags.project_id=device.project_id
        left join lateral(
          select capability.verified_at,case when jsonb_typeof(capability.details_json->'itemCount')='number'
            then greatest(0,least(1000,(capability.details_json->>'itemCount')::int)) else null end as item_count,
            case when capability.expires_at is not null and capability.expires_at<=now() then 'stale'
              when capability.status='supported' then 'available'
              when capability.status='empty' then 'empty' else 'unavailable' end as state
            from connector_capability_snapshots capability
           where capability.project_id=device.project_id and capability.connector_instance_id=flighthub_route.id
             and capability.capability_code='tca.status.read' and capability.evidence_level='live-read'
           order by capability.verified_at desc limit 1
        ) tca on true
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
  const authorizedDevices = devices.rows.map((rawDevice) => {
    const device = applyFlightHubDevicePrerequisites(rawDevice);
    return { ...device, capabilities: projectDeviceCapabilities({
      deviceId: device.id, deviceTypeId: device.deviceTypeId, deviceStatus: device.status,
      role: access.role, capabilities: device.capabilities, grants: grants.rows
    }) };
  });
  return buildDeviceTree(authorizedDevices, relations.rows);
}
