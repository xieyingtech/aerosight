import "server-only";

import { notFound } from "next/navigation";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";

export type IssueListItem = {
  id: number; number: number; title: string; status: string; priority: string;
  occurrenceCount: number; labels: string[]; hasMapLocation: boolean;
  firstSeenAt: string | Date; lastSeenAt: string | Date; updatedAt: string | Date;
};

export async function listIssues(projectId: number) {
  await requireCurrentProjectPermission(projectId, "project:view");
  return (await query<IssueListItem>(`select issue.id,issue.number,issue.title,issue.status,issue.priority,
    issue.occurrence_count as "occurrenceCount",issue.labels_json as labels,
    issue.state_version as "stateVersion",
    issue.first_seen_at as "firstSeenAt",issue.last_seen_at as "lastSeenAt",issue.updated_at as "updatedAt",
    exists(select 1 from issue_links link join detections detection
      on detection.project_id=link.project_id and detection.id=case when link.target_id~'^[0-9]+$' then link.target_id::bigint end
      where link.project_id=issue.project_id and link.issue_id=issue.id and link.link_type='detection'
        and detection.geographic_geometry is not null) as "hasMapLocation"
    from issues issue where issue.project_id=$1 order by issue.last_seen_at desc,issue.number desc`, [projectId])).rows;
}

export async function readIssue(projectId: number, issueId: number) {
  const { access } = await requireCurrentProjectPermission(projectId, "project:view");
  const issue = (await query<Record<string, unknown>>(`select issue.id,issue.number,issue.title,issue.description,
    issue.status,issue.priority,issue.source_type as "sourceType",issue.source_id as "sourceId",
    issue.task_run_id as "taskRunId",issue.task_version_id as "taskVersionId",
    issue.condition_scope_key as "conditionScopeKey",issue.business_object_key as "businessObjectKey",
    issue.occurrence_count as "occurrenceCount",issue.labels_json as labels,issue.state_version as "stateVersion",
    issue.first_seen_at as "firstSeenAt",issue.last_seen_at as "lastSeenAt",issue.created_at as "createdAt",
    task.name as "taskName",version.version as "taskVersion"
    from issues issue left join task_runs run on run.id=issue.task_run_id and run.project_id=issue.project_id
    left join tasks task on task.id=run.task_id and task.project_id=run.project_id
    left join task_versions version on version.id=issue.task_version_id and version.project_id=issue.project_id
    where issue.project_id=$1 and issue.id=$2`, [projectId, issueId])).rows[0];
  if (!issue) notFound();
  const [events, links, detections, assets, assignees, members, agents] = await Promise.all([
    query<Record<string, unknown>>(`select event.id,event.event_type as "eventType",event.body,event.metadata_json as metadata,
      event.created_at as "createdAt",coalesce(actor.name,agent.name,'系统') as "actorName"
      from issue_events event left join users actor on actor.id=event.actor_user_id
      left join agents agent on agent.id=event.actor_agent_id and agent.project_id=event.project_id
      where event.project_id=$1 and event.issue_id=$2 order by event.created_at,event.id`, [projectId, issueId]),
    query<Record<string, unknown>>(`select link.link_type as "linkType",link.target_id as "targetId",link.created_at as "createdAt"
      from issue_links link where link.project_id=$1 and link.issue_id=$2 order by link.id`, [projectId, issueId]),
    query<Record<string, unknown>>(`select detection.id,detection.label,detection.confidence,
      detection.input_asset_id as "inputAssetId",detection.pixel_geometry_json as "pixelGeometry",
      detection.location_quality as "locationQuality",detection.projection_method as "projectionMethod",
      detection.horizontal_error_meters as "horizontalErrorMeters",detection.captured_at as "capturedAt",
      ST_AsGeoJSON(detection.geographic_geometry)::json as geometry,
      run.id as "algorithmRunId",definition.name as "algorithmName",version.version as "algorithmVersion",
      version.model_or_process as "modelOrProcess",coalesce(version.protocol_config_json->>'mappingVersion','v1') as "mappingVersion",
      asset.version as "assetVersion",asset.checksum_sha256 as "assetChecksumSha256"
      from issue_links link join detections detection
        on detection.project_id=link.project_id and detection.id=case when link.target_id~'^[0-9]+$' then link.target_id::bigint end
      join algorithm_runs run on run.id=detection.algorithm_run_id and run.project_id=detection.project_id
      join algorithm_definition_versions version on version.id=run.algorithm_definition_version_id and version.project_id=run.project_id
      join algorithm_definitions definition on definition.id=version.algorithm_definition_id and definition.project_id=version.project_id
      join assets asset on asset.id=detection.input_asset_id and asset.project_id=detection.project_id
      where link.project_id=$1 and link.issue_id=$2 and link.link_type='detection' order by detection.captured_at,detection.id`, [projectId, issueId]),
    query<Record<string, unknown>>(`select distinct asset.id,asset.version,asset.kind,asset.mime_type as "mimeType",
      asset.checksum_sha256 as "checksumSha256",asset.captured_at as "capturedAt"
      from assets asset where asset.project_id=$1 and asset.status='available' and (
        exists(select 1 from issue_links link where link.project_id=asset.project_id and link.issue_id=$2
          and link.link_type='asset' and link.target_id=asset.id::text)
        or exists(select 1 from issue_links link join detections detection
          on detection.project_id=link.project_id and detection.id=case when link.target_id~'^[0-9]+$' then link.target_id::bigint end
          where link.project_id=asset.project_id and link.issue_id=$2 and link.link_type='detection' and detection.input_asset_id=asset.id)
      ) order by asset.captured_at,asset.id`, [projectId, issueId]),
    query<Record<string, unknown>>(`select assignee.id,assignee.assignee_type as "assigneeType",
      coalesce(member.id,agent.id) as "assigneeId",coalesce(member.name,agent.name) as name,assignee.created_at as "createdAt"
      from issue_assignees assignee left join users member on member.id=assignee.user_id
      left join agents agent on agent.id=assignee.agent_id and agent.project_id=assignee.project_id
      where assignee.project_id=$1 and assignee.issue_id=$2 and assignee.active order by assignee.created_at`, [projectId, issueId]),
    query<Record<string, unknown>>(`select member_user.id,member_user.name,member.role
      from projects project join team_members member on member.team_id=project.team_id
      join users member_user on member_user.id=member.user_id where project.id=$1 order by member_user.name`, [projectId]),
    query<Record<string, unknown>>(`select id,name,status,config_json->>'kind' as kind from agents
      where project_id=$1 and status='active' order by case when config_json->>'kind'='copilot' then 0 else 1 end,name`, [projectId])
  ]);
  return { issue, events: events.rows, links: links.rows, detections: detections.rows, assets: assets.rows,
    assignees: assignees.rows, members: members.rows, agents: agents.rows,
    canHandle: access.permissions.has("issue:handle"), canAssign: access.permissions.has("issue:assign"),
    canUseAgent: access.permissions.has("agent:use") };
}
