UPDATE devices AS device
SET name = coalesce(
  nullif(nullif(nullif(trim(identity.identity_json#>>'{attributes,callsign}'), ''), identity.external_device_id), split_part(identity.external_device_id, '/', 2)),
  type.display_name || ' ' || right(coalesce(nullif(identity.identity_json#>>'{attributes,serialNumber}', ''), split_part(identity.external_device_id, '/', 2)), 6)
), updated_at = now()
FROM device_external_identities AS identity
JOIN device_adapters AS adapter ON adapter.id = identity.adapter_id
JOIN connector_definitions AS connector ON connector.id = adapter.connector_definition_id
JOIN device_types AS type ON type.id = identity.suggested_device_type_id
WHERE device.id = identity.device_id AND device.project_id = identity.project_id
  AND connector.connector_key = 'dji.flighthub2'
  AND (device.name = identity.external_device_id
    OR device.name = identity.identity_json#>>'{attributes,serialNumber}');
