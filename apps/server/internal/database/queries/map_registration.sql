-- name: RegisterMapRegion :one
INSERT INTO project_map_regions(project_id,team_id,registration_key,name,geometry)
SELECT sqlc.arg(project_id),sqlc.arg(team_id),sqlc.arg(registration_key),sqlc.arg(name),shape.geom
FROM (SELECT ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON(sqlc.arg(geometry_json)::text),4326)) AS geom) shape
WHERE ST_IsValid(shape.geom) AND NOT ST_IsEmpty(shape.geom)
ON CONFLICT(project_id,registration_key) DO UPDATE SET registration_key=EXCLUDED.registration_key
RETURNING id;

-- name: SnapshotMapRegions :many
SELECT jsonb_build_object('id',id::text,'projectId',project_id,'name',name,'geometry',ST_AsGeoJSON(geometry)::jsonb)
FROM project_map_regions WHERE project_id=$1 ORDER BY id;

-- name: RegisterMapDevice :one
INSERT INTO devices(project_id,registration_key,name,type,device_type_id,config_json,status,status_reason,data_freshness)
SELECT sqlc.arg(project_id),sqlc.arg(registration_key),sqlc.arg(name),
 CASE category WHEN 'camera' THEN 'fixed_sensor' WHEN 'sensor' THEN 'fixed_sensor' WHEN 'ground_robot' THEN 'ground_robot' ELSE 'other' END,
 id,sqlc.arg(config_json)::jsonb,'offline','尚未接入设备','unknown'
FROM device_types WHERE type_key=sqlc.arg(type_key) AND status='active' AND type_key LIKE 'registered.%'
ORDER BY version DESC LIMIT 1
ON CONFLICT(project_id,registration_key) WHERE registration_key IS NOT NULL DO UPDATE SET registration_key=EXCLUDED.registration_key
RETURNING id;
