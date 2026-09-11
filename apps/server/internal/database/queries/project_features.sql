-- name: ReadProjectFeatures :one
SELECT (coalesce(f.flighthub_action_flags_json,'{}'::jsonb) || jsonb_build_object(
 'operations.overview',coalesce(f.operations_overview_enabled,false),
 'devices.commands',coalesce(f.device_commands_enabled,false),
 'storage.objects',coalesce(f.object_storage_enabled,false),
 'algorithms.external',coalesce(f.external_algorithms_enabled,false)
))::jsonb AS values
FROM projects p LEFT JOIN project_feature_flags f ON f.project_id=p.id WHERE p.id=$1;

-- name: EnsureProjectFeatures :exec
INSERT INTO project_feature_flags(project_id) VALUES($1) ON CONFLICT(project_id) DO NOTHING;

-- name: LockProjectFeatures :one
SELECT * FROM project_feature_flags WHERE project_id=$1 FOR UPDATE;

-- name: SaveProjectFeatures :exec
UPDATE project_feature_flags SET
 operations_overview_enabled=$2,device_commands_enabled=$3,
 object_storage_enabled=$4,external_algorithms_enabled=$5,
 flighthub_action_flags_json=flighthub_action_flags_json || sqlc.arg(action_changes)::jsonb,
 updated_by_user_id=$6,updated_at=now()
WHERE project_id=$1;
