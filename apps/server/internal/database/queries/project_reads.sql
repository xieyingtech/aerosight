-- name: ListProjectAssets :many
select to_jsonb(result_row) as item from (
  select id, kind, mime_type as "mimeType", captured_at as "capturedAt", created_at as "createdAt"
  from assets where project_id=$1 and status='available' order by created_at desc
) result_row;

-- name: ReadDeviceTree :many
SELECT to_jsonb(r) FROM (
select device.id,device.device_type_id::text as "deviceTypeId",device.name,device_type.category,device.status,device.data_freshness as "dataFreshness",
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
          select adapter.id,adapter.status,adapter.discovery_scope_json,definition.connector_key,definition.version,
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
        where device.project_id=$1 order by device.id
) r;
-- name: ReadDeviceRelations :many
select to_jsonb(result_row) as item from (
select from_device_id as "fromDeviceId",to_device_id as "toDeviceId",relation_type as "relationType"
         from device_relationships where project_id=$1 and valid_until is null order by valid_from
) result_row;
-- name: ListProjectDevices :many
select to_jsonb(result_row) as item from (
select device.id, device.name, device.type, device.status,
                   device.last_seen_at as "lastSeenAt", device.updated_at as "updatedAt",
                   device_type.id::text as "deviceTypeId", device_type.type_key as "typeKey",
                   device_type.version as "typeVersion", device_type.display_name as "typeName",
                   device_type.category, device_type.vendor, device_type.model,
                   driver.driver_key as "driverKey", driver.version as "driverVersion",
                   driver.status as "driverStatus"
              from devices device
              join device_types device_type on device_type.id = device.device_type_id
              join driver_definitions driver on driver.id = device_type.driver_definition_id
             where device.project_id = $1 order by device.updated_at desc
) result_row;
-- name: GetProjectDevice :one
select to_jsonb(result_row) as item from (
select device.id, device.project_id as "projectId", device.name, device.type, device.status,
                   device.last_seen_at as "lastSeenAt", device.updated_at as "updatedAt",
                   device_type.id::text as "deviceTypeId", device_type.type_key as "typeKey",
                   device_type.version as "typeVersion", device_type.display_name as "typeName",
                   device_type.category, device_type.vendor, device_type.model,
                   driver.driver_key as "driverKey", driver.version as "driverVersion",
                   driver.status as "driverStatus"
              from devices device
              join device_types device_type on device_type.id = device.device_type_id
              join driver_definitions driver on driver.id = device_type.driver_definition_id
             where device.project_id = $1 and device.id = $2
) result_row;
-- name: ReplayPoses :many
select to_jsonb(result_row) as item from (
select pose.observation_id::text as id, pose.device_id as "deviceId", device.name as "deviceName",
              device.type as "deviceType", device_type.type_key as "deviceTypeKey",
              device_type.version as "deviceTypeVersion", driver.driver_key as "driverKey",
              pose.captured_at as "capturedAt", pose.spatial_quality as "spatialQuality",
              ST_X(pose.standard_position) as longitude, ST_Y(pose.standard_position) as latitude,
              ST_Z(pose.standard_position) as "altitudeMeters"
       from poses pose join devices device on device.id = pose.device_id and device.project_id = pose.project_id
       join device_types device_type on device_type.id = device.device_type_id
       join driver_definitions driver on driver.id = device_type.driver_definition_id
       where pose.project_id = $1 and pose.captured_at >= $2 and pose.captured_at <= $3
         and pose.standard_position is not null
         and (cardinality($4::text[]) = 0 or device.type = any($4::text[]) or device_type.type_key = any($4::text[]))
         and ($5::double precision[] is null or ST_Intersects(
           pose.standard_position,
           ST_MakeEnvelope(($5::double precision[])[1], ($5::double precision[])[2], ($5::double precision[])[3], ($5::double precision[])[4], 4326)
         ))
       order by pose.captured_at limit 5001
) result_row;
-- name: ReplayMedia :many
select to_jsonb(result_row) as item from (
select id, device_id as "deviceId", kind, mime_type as "mimeType",
              captured_at as "capturedAt", metadata_json as metadata
       from assets where project_id = $1 and status = 'available' and captured_at >= $2 and captured_at <= $3
       order by captured_at limit 1001
) result_row;
-- name: ReplayEvents :many
select to_jsonb(result_row) as item from (
select cursor::text, event_type as "eventType", payload_json as payload,
              occurred_at as "occurredAt"
       from project_events where project_id = $1 and occurred_at >= $2 and occurred_at <= $3
       order by project_events.cursor limit 2001
) result_row;
