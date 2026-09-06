-- name: ReadDeviceTree :many
select to_jsonb(result_row) as item from (
select device.id,device.device_type_id::text as "deviceTypeId",device.name,device_type.category,device.status,device.data_freshness as "dataFreshness",
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
        where device.project_id=$1 order by device.id
) result_row;
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
