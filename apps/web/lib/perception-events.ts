import "server-only";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { buildPerceptionEventEvidence } from "@/lib/perception-event-view-core";
import { availablePerceptionEventActions } from "@/lib/perception-event-actions-core";
import { withAuditedProjectWrite } from "@/lib/audit";
import { correlationId } from "@/lib/observability";
import { planPerceptionEventAction,type PerceptionEventAction } from "@/lib/perception-event-actions-core";

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
  const {access}=await requireCurrentProjectPermission(projectId,"project:view");
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
  return {...buildPerceptionEventEvidence({event,detections,feedback}),actions:availablePerceptionEventActions(String(event.status),access.permissions)};
}

export async function handlePerceptionEvent(input:{projectId:number;eventId:string;action:PerceptionEventAction;expectedVersion:number;reason:string;category?:string;issueId?:number;requestId?:string|null}){
  const {user,access}=await requireCurrentProjectPermission(input.projectId,"event:handle");
  if(!input.reason.trim())throw new Error("EVENT_REASON_REQUIRED");
  return withAuditedProjectWrite({projectId:input.projectId,teamId:access.teamId,actorUserId:user.id,requestId:correlationId(input.requestId),
    action:`perception_event.${input.action}`,resourceType:"perception_event",resourceId:input.eventId,
    input:{action:input.action,expectedVersion:input.expectedVersion,reason:input.reason,category:input.category,issueId:input.issueId},
    policyResult:{permission:"event:handle",optimisticConcurrency:true,algorithmResultImmutable:true}},async client=>{
      const current=(await client.query<{status:string;stateVersion:number}>(`select status,state_version as "stateVersion" from perception_events where project_id=$1 and id=$2 for update`,[input.projectId,input.eventId])).rows[0];
      if(!current)throw new Error("PERCEPTION_EVENT_NOT_FOUND");
      const plan=planPerceptionEventAction({action:input.action,currentStatus:current.status,actualVersion:current.stateVersion,expectedVersion:input.expectedVersion,permissions:access.permissions,actorUserId:user.id,category:input.category});
      const updated=(await client.query<{stateVersion:number;status:string}>(`update perception_events set status=$3,state_version=$4,
        assigned_user_id=coalesce($5,assigned_user_id),updated_at=now(),resolved_at=case when $3 in('resolved','dismissed') then now() else resolved_at end
        where project_id=$1 and id=$2 and state_version=$6 returning state_version as "stateVersion",status`,[input.projectId,input.eventId,plan.status,plan.stateVersion,plan.assignedUserId??null,input.expectedVersion])).rows[0];
      if(!updated)throw new Error("PERCEPTION_EVENT_VERSION_CONFLICT");
      await client.query(`insert into event_feedback(project_id,team_id,perception_event_id,action,value_json,reason,actor_user_id) values($1,$2,$3,$4,$5,$6,$7)`,
        [input.projectId,access.teamId,input.eventId,plan.feedbackAction,plan.feedbackValue,input.reason.trim(),user.id]);
      if(input.issueId){
        const issue=(await client.query(`select id from issues where project_id=$1 and id=$2`,[input.projectId,input.issueId])).rows[0];if(!issue)throw new Error("ISSUE_NOT_FOUND");
        await client.query(`insert into issue_links(project_id,issue_id,link_type,target_id,created_by_user_id) values($1,$2,'perception_event',$3,$4) on conflict(issue_id,link_type,target_id) do nothing`,[input.projectId,input.issueId,input.eventId,user.id]);
      }
      return updated;
    });
}
