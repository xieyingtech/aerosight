-- name: ListLegacyPerceptionEvents :many
SELECT to_jsonb(r) FROM (
select event.id,event.title,event.severity,event.status,
    event.occurrence_count as "occurrenceCount",event.state_version as "stateVersion",
    event.first_detected_at as "firstDetectedAt",event.last_detected_at as "lastDetectedAt",
    group_row.location_quality as "locationQuality",ST_AsGeoJSON(group_row.geographic_geometry)::json as geometry,
    version.version as "ruleVersion"
    from perception_events event
    join detection_groups group_row on group_row.id=event.detection_group_id and group_row.project_id=event.project_id
    join event_rule_versions version on version.id=event.event_rule_version_id and version.project_id=event.project_id
    where event.project_id=$1 order by event.last_detected_at desc limit 500
) r;

-- name: GetLegacyPerceptionEvent :one
SELECT to_jsonb(r) FROM (
select event.id,event.title,event.severity,event.status,
    event.occurrence_count as "occurrenceCount",event.state_version as "stateVersion",event.assigned_user_id as "assignedUserId",
    event.first_detected_at as "firstDetectedAt",event.last_detected_at as "lastDetectedAt",
    event.detection_group_id::text as "detectionGroupId",version.version as "ruleVersion",rule.name as "ruleName"
    from perception_events event join event_rule_versions version on version.id=event.event_rule_version_id and version.project_id=event.project_id
    join event_rules rule on rule.id=version.event_rule_id and rule.project_id=event.project_id
    where event.project_id=$1 and event.id=$2
) r;

-- name: GetLegacyPerceptionDetections :many
SELECT to_jsonb(r) FROM (
select detection.id::text,detection.label,detection.confidence,
    detection.location_quality as "locationQuality",ST_AsGeoJSON(detection.geographic_geometry)::json as "geographicGeometry",
    detection.horizontal_error_meters as "horizontalErrorMeters",detection.projection_method as "projectionMethod",
    detection.pixel_geometry_json as "pixelGeometry",version.model_or_process as "modelOrProcess",version.version as "modelVersion",
    nullif(version.protocol_config_json->>'mappingVersion','') as "mappingVersion",
    asset.id as "inputAssetId",asset.version as "assetVersion",asset.checksum_sha256 as "assetChecksumSha256",
    asset.mime_type as "mimeType",detection.captured_at as "capturedAt"
    from detections detection
    join algorithm_runs run on run.id=detection.algorithm_run_id and run.project_id=detection.project_id
    join algorithm_definition_versions version on version.id=run.algorithm_definition_version_id and version.project_id=run.project_id
    join assets asset on asset.id=detection.input_asset_id and asset.project_id=detection.project_id
    where detection.project_id=$1 and detection.group_id=$2 order by detection.captured_at
) r;

-- name: GetLegacyPerceptionFeedback :many
SELECT to_jsonb(r) FROM (
select feedback.item->>'id' as id,feedback.item->>'action' as action,feedback.item->'value_json' as value,
    feedback.item->>'reason' as reason,coalesce(actor.name,feedback.item->>'actor_name') as "actorName",
    (feedback.item->>'created_at')::timestamptz as "createdAt"
    from perception_events event cross join lateral jsonb_array_elements(event.legacy_feedback_json) feedback(item)
    left join users actor on actor.id=(feedback.item->>'actor_user_id')::integer
    where event.project_id=$1 and event.id=$2 order by (feedback.item->>'created_at')::timestamptz,(feedback.item->>'id')::bigint
) r;
