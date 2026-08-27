import "server-only";

import { randomUUID } from "node:crypto";
import { auditHash } from "@/lib/audit-boundary";
import type { AgentExecutionContext } from "@/lib/agent-execution-context-core";
import { authorizeAgentMissionStart, agentMissionStartInputSchema } from "@/lib/agent-mission-start-core";
import { withIdempotentProjectOperation } from "@/lib/idempotency";
import { correlationId } from "@/lib/observability";
import { requireProjectPermissionForUser, resolveProjectAccess } from "@/lib/project-access";
import { publishProjectEvent } from "@/lib/project-events";

type AuthorizationRow = {
  taskProjectId: number;
  taskVersionStatus: string;
  taskId: number;
  approvalStatus: string | null;
  approvalProjectId: number | null;
  approvalResourceType: string | null;
  approvalResourceId: string | null;
  approvalAction: string | null;
  preflightAllowed: boolean;
  deviceCommandsEnabled: boolean;
  selectedDeviceId: number | null;
  safetyPolicyVersionId: number | null;
  preflight: Record<string, unknown>;
};

export async function executeAgentMissionStart(context: AgentExecutionContext, rawInput: unknown, requestId?: string | null) {
  const input = agentMissionStartInputSchema.parse(rawInput);
  const access = await resolveProjectAccess(context.userId, context.projectId);
  const hasPermission = Boolean(access?.permissions.has("mission:operate"));
  if (hasPermission) await requireProjectPermissionForUser(context.userId, context.projectId, "mission:operate");
  const { query } = await import("@/lib/db");
  const row = (await query<AuthorizationRow>(
    `select version.project_id as "taskProjectId",version.status as "taskVersionStatus",version.task_id as "taskId",
      approval.status as "approvalStatus",approval.project_id as "approvalProjectId",approval.resource_type as "approvalResourceType",
      approval.resource_id as "approvalResourceId",approval.action as "approvalAction",
      coalesce((approval.context_json#>>'{preflight,allowed}')::boolean,false) as "preflightAllowed",
      coalesce(flags.device_commands_enabled,false) as "deviceCommandsEnabled",
      nullif(approval.context_json->>'selectedDeviceId','')::int as "selectedDeviceId",
      nullif(approval.context_json->>'safetyPolicyVersionId','')::bigint as "safetyPolicyVersionId",
      coalesce(approval.context_json->'preflight','{}'::jsonb) as preflight
      from task_versions version
      left join approval_requests approval on approval.id=$3 and approval.project_id=version.project_id
      left join project_feature_flags flags on flags.project_id=version.project_id
      where version.project_id=$1 and version.id=$2`,
    [context.projectId,input.taskVersionId,input.approvalRequestId]
  )).rows[0];
  if (!row) throw new Error("AGENT_MISSION_VERSION_NOT_FOUND");
  const plan = authorizeAgentMissionStart(context, input, { ...row, hasPermission });

  return withIdempotentProjectOperation({
    projectId: context.projectId,
    teamId: context.teamId,
    actorKey: `user:${context.userId}`,
    operation: "agent.mission.start",
    idempotencyKey: plan.idempotencyKey,
    request: { taskVersionId: plan.taskVersionId, approvalRequestId: plan.approvalRequestId }
  }, async (client) => {
    const auditId = (await client.query<{ id: string }>(
      `insert into audit_events(project_id,team_id,request_id,idempotency_key,actor_user_id,action,resource_type,resource_id,input_hash,policy_result_json)
       values($1,$2,$3,$4,$5,'agent.request_mission_start','task_version',$6,$7,$8) returning id`,
      [context.projectId,context.teamId,correlationId(requestId),plan.idempotencyKey,context.userId,String(plan.taskVersionId),
        auditHash(input),{ permission: "mission:operate", approvalRequestId: plan.approvalRequestId, preflight: "passed" }]
    )).rows[0];
    const run = (await client.query<{ id: number }>(
      `insert into task_runs(project_id,team_id,task_id,task_version_id,selected_device_id,safety_policy_version_id,
        approval_request_id,trigger_source,status,input_snapshot_json,preflight_snapshot_json,created_by_user_id,state_reason)
       values($1,$2,$3,$4,$5,$6,$7,'agent','dispatching',$8,$9,$10,'agent-approved-start') returning id`,
      [context.projectId,context.teamId,row.taskId,plan.taskVersionId,plan.selectedDeviceId,plan.safetyPolicyVersionId,
        plan.approvalRequestId,{ agentSessionId: context.sessionId },row.preflight,context.userId]
    )).rows[0];
    await client.query(
      `insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status)
       select project_id,team_id,$3,id,position,'pending' from task_steps where project_id=$1 and task_version_id=$2 order by position`,
      [context.projectId,plan.taskVersionId,run.id]
    );
    await publishProjectEvent(client, {
      projectId: context.projectId,teamId: context.teamId,eventId: randomUUID(),eventType: "task_run.transitioned",
      payload: { taskRunId: run.id, to: "dispatching", source: "agent", sessionId: context.sessionId },enqueue: true
    });
    const result = { taskRunId: run.id, status: "dispatching", completion: plan.completion };
    await client.query(`update audit_events set status='completed',result_hash=$2,completed_at=now() where id=$1 and project_id=$3`,
      [auditId.id,auditHash(result),context.projectId]);
    return result;
  });
}
