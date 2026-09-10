-- name: ReadFlightHubMediaChannels :many
SELECT to_jsonb(r) FROM (
select channel.id::text,channel.device_id as "deviceId",device.name as "deviceName",
        channel.channel_key as "channelKey",channel.display_name as "displayName",channel.availability
      from device_stream_channels channel
      join devices device on device.id=channel.device_id and device.project_id=channel.project_id
      join device_adapters adapter on adapter.id=device.adapter_id and adapter.project_id=device.project_id
      join connector_definitions definition on definition.id=adapter.connector_definition_id and definition.connector_key='dji.flighthub2'
      where channel.project_id=$1 and channel.data_type='video' order by device.name,channel.channel_key
) r;

-- name: ReadFlightHubMediaSessions :many
SELECT to_jsonb(r) FROM (
select stream.id::text,stream.device_id as "deviceId",device.name as "deviceName",stream.stream_key as "streamKey",
        stream.status,stream.supplier,stream.supplier_protocol as "protocol",stream.status_reason as "statusReason",
        stream.last_active_at as "lastActiveAt",stream.supplier_credential_expires_at as "credentialExpiresAt"
      from live_streams stream join devices device on device.id=stream.device_id and device.project_id=stream.project_id
      where stream.project_id=$1 and stream.source_type='dji_flighthub' order by stream.created_at desc limit 100
) r;

-- name: ReadFlightHubMediaResources :many
SELECT to_jsonb(r) FROM (
select resource.id::text,resource.resource_kind as kind,resource.status,
        resource.summary_json as summary,resource.updated_at as "updatedAt",resource.connector_instance_id::text as "connectorId"
      from connector_remote_resources resource
      join device_adapters adapter on adapter.id=resource.connector_instance_id and adapter.project_id=resource.project_id
      join connector_definitions definition on definition.id=adapter.connector_definition_id and definition.connector_key='dji.flighthub2'
      where resource.project_id=$1 and resource.resource_kind in('recording','live-share','stream-converter')
      order by resource.resource_kind,resource.updated_at desc limit 300
) r;
