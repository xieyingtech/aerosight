-- name: ListDeviceTypePresentation :many
SELECT to_jsonb(r) FROM (
  SELECT t.id::text, t.type_key AS "typeKey", t.version, t.display_name AS "displayName", t.icon, t.status,
    t.category, t.vendor, t.model, d.driver_key AS "driverKey", d.version AS "driverVersion",
    t.driver_version_constraint AS "driverVersionConstraint", t.capability_profile_json AS "capabilityProfile"
  FROM device_types t JOIN driver_definitions d ON d.id=t.driver_definition_id
  ORDER BY t.display_name, t.version DESC, t.id
) r;

-- name: ListDeviceCatalogDrivers :many
SELECT to_jsonb(r) FROM (
 SELECT id::text, driver_key AS "driverKey", version, display_name AS "displayName", status, manifest_json AS manifest
 FROM driver_definitions ORDER BY driver_key,version,id
) r;

-- name: UpdateDeviceTypeIcon :one
UPDATE device_types SET icon = sqlc.arg(icon), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id::text;
