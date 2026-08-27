import "server-only";

import { query } from "@/lib/db";
import { requireProjectPermissionForUser } from "@/lib/project-access";
import type { AgentExecutionContext } from "@/lib/agent-execution-context-core";
import {
  formatAgentReadToolResult,
  prepareAgentReadToolCall,
  type AgentReadToolName
} from "@/lib/agent-read-tools-core";

const sql: Record<AgentReadToolName, string> = {
  query_devices: `select device.id,device.name,device.type,device.status,
    device_type.type_key as "typeKey",device_type.version as "typeVersion",
    driver.driver_key as "driverKey",driver.version as "driverVersion",
    device.last_seen_at as "observedAt",
    coalesce(pose.spatial_quality,'unlocated') as quality
    from devices device
    join device_types device_type on device_type.id=device.device_type_id
    join driver_definitions driver on driver.id=device_type.driver_definition_id
    left join lateral(select spatial_quality from poses where project_id=$1 and device_id=device.id order by captured_at desc limit 1) pose on true
    where device.project_id=$1 order by device.updated_at desc limit 101`,
  query_missions: `select run.id,task.name,run.status,run.state_reason as reason,
    coalesce(run.finished_at,run.started_at,run.created_at) as "observedAt",'platform-state' as quality
    from task_runs run join tasks task on task.id=run.task_id and task.project_id=run.project_id
    where run.project_id=$1 order by run.created_at desc limit 101`,
  query_events: `select event.id,event.title,event.severity,event.status,event.occurrence_count as "occurrenceCount",
    event.last_detected_at as "observedAt",group_row.location_quality as quality
    from perception_events event join detection_groups group_row on group_row.id=event.detection_group_id and group_row.project_id=event.project_id
    where event.project_id=$1 order by event.last_detected_at desc limit 101`,
  query_assets: `select asset.id,asset.kind,asset.mime_type as "mimeType",asset.version,
    coalesce(asset.captured_at,asset.created_at) as "observedAt",
    case when asset.status='available' then 'verified-metadata' else asset.status end as quality
    from assets asset where asset.project_id=$1 and asset.status='available' and asset.deleted_at is null
    order by coalesce(asset.captured_at,asset.created_at) desc limit 101`,
  query_tracks: `select pose.device_id as id,pose.device_id as "deviceId",min(pose.captured_at) as "startedAt",
    max(pose.captured_at) as "observedAt",count(*)::int as "pointCount",
    ST_AsGeoJSON(ST_MakeLine(pose.standard_position order by pose.captured_at))::json as geometry,
    min(pose.spatial_quality) as quality from poses pose
    where pose.project_id=$1 and pose.standard_position is not null group by pose.device_id order by max(pose.captured_at) desc limit 101`,
  query_map_context: `select 'current' as id,now() as "observedAt",'aggregated' as quality,
    (select count(*)::int from devices where project_id=$1) as "deviceCount",
    (select count(*)::int from perception_events where project_id=$1 and status in('open','acknowledged','investigating')) as "openEventCount",
    (select count(*)::int from task_runs where project_id=$1 and status in('queued','ready','dispatching','running','paused')) as "activeMissionCount"`
};

export async function executeAgentReadTool(context: AgentExecutionContext, name: AgentReadToolName, rawInput: unknown) {
  const prepared = prepareAgentReadToolCall(context, name, rawInput);
  await requireProjectPermissionForUser(context.userId, context.projectId, "project:view");
  const rows = (await query<Record<string, unknown>>(sql[name], [prepared.projectId])).rows;
  return formatAgentReadToolResult(context, name, rows);
}
