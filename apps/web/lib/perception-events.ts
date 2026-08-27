import "server-only";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { buildPerceptionEventEvidence } from "@/lib/perception-event-view-core";

export async function listPerceptionEvents(projectId: number) {
  await requireCurrentProjectPermission(projectId, "project:view");
  return (await query<Record<string, unknown>>(`select event.id,'疑似违建' as title,event.severity,event.status,
    event.occurrence_count as "occurrenceCount",event.state_version as "stateVersion",
    event.first_detected_at as "firstDetectedAt",event.last_detected_at as "lastDetectedAt",
    group_row.location_quality as "locationQuality",ST_AsGeoJSON(group_row.geographic_geometry)::json as geometry,
    version.version as "ruleVersion"
    from perception_events event
    join detection_groups group_row on group_row.id=event.detection_group_id and group_row.project_id=event.project_id
    join event_rule_versions version on version.id=event.event_rule_version_id and version.project_id=event.project_id
    where event.project_id=$1 order by event.last_detected_at desc limit 500`,[projectId])).rows;
}

export async function readPerceptionEvent(projectId:number,eventId:string){
  await requireCurrentProjectPermission(projectId,"project:view");
  const event=(await query<Record<string,unknown>>(`select event.id,'疑似违建' as title,event.severity,event.status,
    event.occurrence_count as "occurrenceCount",event.state_version as "stateVersion",event.assigned_user_id as "assignedUserId",
    event.first_detected_at as "firstDetectedAt",event.last_detected_at as "lastDetectedAt",
    event.detection_group_id as "detectionGroupId",version.version as "ruleVersion",rule.name as "ruleName"
    from perception_events event join event_rule_versions version on version.id=event.event_rule_version_id and version.project_id=event.project_id
    join event_rules rule on rule.id=version.event_rule_id and rule.project_id=event.project_id
    where event.project_id=$1 and event.id=$2`,[projectId,eventId])).rows[0];
  if(!event)throw new Error("PERCEPTION_EVENT_NOT_FOUND");
  const detections=(await query<Record<string,unknown>>(`select detection.id,detection.label,detection.confidence,
    detection.location_quality as "locationQuality",ST_AsGeoJSON(detection.geographic_geometry)::json as "geographicGeometry",
    detection.horizontal_error_meters as "horizontalErrorMeters",detection.projection_method as "projectionMethod",
    detection.pixel_geometry_json as "pixelGeometry",version.model_or_process as "modelOrProcess",version.version as "modelVersion",
    coalesce(version.protocol_config_json->>'mappingVersion','suspected-construction/v1') as "mappingVersion",
    asset.id as "inputAssetId",asset.version as "assetVersion",asset.checksum_sha256 as "assetChecksumSha256",
    asset.mime_type as "mimeType",detection.captured_at as "capturedAt"
    from detection_group_members member join detections detection on detection.id=member.detection_id and detection.project_id=member.project_id
    join algorithm_runs run on run.id=detection.algorithm_run_id and run.project_id=detection.project_id
    join algorithm_definition_versions version on version.id=run.algorithm_definition_version_id and version.project_id=run.project_id
    join assets asset on asset.id=detection.input_asset_id and asset.project_id=detection.project_id
    where member.project_id=$1 and member.detection_group_id=$2 order by detection.captured_at`,[projectId,event.detectionGroupId])).rows;
  const feedback=(await query<Record<string,unknown>>(`select feedback.id,feedback.action,feedback.value_json as value,feedback.reason,
    actor.name as "actorName",feedback.created_at as "createdAt" from event_feedback feedback join users actor on actor.id=feedback.actor_user_id
    where feedback.project_id=$1 and feedback.perception_event_id=$2 order by feedback.created_at`,[projectId,eventId])).rows;
  return buildPerceptionEventEvidence({event,detections,feedback});
}
