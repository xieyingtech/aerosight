-- name: ListDeviceTypePresentation :many
SELECT to_jsonb(r) FROM (
  SELECT id::text, type_key AS "typeKey", version, display_name AS "displayName", icon, status
  FROM device_types ORDER BY display_name, version DESC, id
) r;

-- name: UpdateDeviceTypeIcon :one
UPDATE device_types SET icon = sqlc.arg(icon), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id::text;
