import "server-only";

import { correlationId } from "@/lib/observability";
import { withAuditedProjectWrite } from "@/lib/audit";
import { query } from "@/lib/db";
import { requireCurrentProjectPermission } from "@/lib/data";
import { buildMissionAuditTrace, planEmergencyStopDrill } from "@/lib/mission-audit-trace-core";

export async function getMissionAuditTrace(projectId: number, taskRunId: number) {
  await requireCurrentProjectPermission(projectId, "safety:manage");
  const run = (await query<{
    id: number; triggerSource: string; taskVersionId: string | null; safetyPolicyVersionId: string | null;
    approvalRequestId: string | null; preflight: Record<string, unknown>;
  }>(`select id,trigger_source as "triggerSource",task_version_id as "taskVersionId",
      safety_policy_version_id as "safetyPolicyVersionId",approval_request_id as "approvalRequestId",
      preflight_snapshot_json as preflight from task_runs where project_id=$1 and id=$2`, [projectId, taskRunId])).rows[0];
  if (!run) throw new Error("TASK_RUN_NOT_FOUND");
  const request = (await query<{
    requestId: string; action: string; actorUserId: number | null; actorAgentId: number | null; createdAt: Date;
  }>(`select request_id as "requestId",action,actor_user_id as "actorUserId",actor_agent_id as "actorAgentId",created_at as "createdAt"
      from audit_events where project_id=$1 and ((resource_type='task_run' and resource_id=$2)
        or (resource_type='task_version' and resource_id=$3))
      order by case when action in('agent.request_mission_start','task_run.transition','task_run.emergency_stop') then 0 else 1 end,
        created_at limit 1`,
    [projectId, String(taskRunId), run.taskVersionId])).rows[0];
  const approval = run.approvalRequestId ? (await query<{
    id: string; status: string; requiredApprovals: number; receivedApprovals: number;
  }>(`select request.id,request.status,request.required_approvals as "requiredApprovals",
      count(decision.id)::int as "receivedApprovals" from approval_requests request
      left join approvals decision on decision.approval_request_id=request.id and decision.project_id=request.project_id
      where request.project_id=$1 and request.id=$2 group by request.id`, [projectId, run.approvalRequestId])).rows[0] : null;
  const commands = (await query<{
    id: string; action: string; capabilityCode: string; status: string; priority: number;
    attempt: number | null; attemptStatus: string | null; errorCode: string | null;
  }>(`select command.id::text,command.command_key as action,command.capability_code as "capabilityCode",command.status,command.priority,
      attempt.attempt,attempt.status as "attemptStatus",attempt.error_code as "errorCode" from device_commands command
      left join lateral(select item.attempt,item.status,item.error_code from command_attempts item
        where item.command_id=command.id and item.project_id=command.project_id order by item.attempt desc limit 1) attempt on true
      where command.project_id=$1 and command.task_run_id=$2 order by command.created_at`, [projectId, taskRunId])).rows;
  return buildMissionAuditTrace({
    projectId, taskRunId, triggerSource: run.triggerSource,
    request: request ? {
      requestId: request.requestId, action: request.action,
      actorType: request.actorAgentId || run.triggerSource === "agent" ? "agent" : "user",
      actorId: request.actorAgentId ?? request.actorUserId ?? 0,
      createdAt: request.createdAt.toISOString()
    } : null,
    preflight: {
      policyVersionId: run.safetyPolicyVersionId,
      allowed: run.preflight.allowed === true,
      checks: Array.isArray(run.preflight.checks) ? run.preflight.checks : []
    },
    approval,
    commands
  });
}

export async function runEmergencyStopDrill(input: {
  projectId: number;
  taskRunId: number;
  dryRun: boolean;
  outcome: "ack" | "nack" | "timeout" | "disconnected";
  requestId?: string | null;
}) {
  if (!input.dryRun) throw new Error("EMERGENCY_STOP_DRILL_MUST_BE_DRY_RUN");
  if (!["ack", "nack", "timeout", "disconnected"].includes(input.outcome)) {
    throw new Error("EMERGENCY_STOP_DRILL_OUTCOME_INVALID");
  }
  const { user, access } = await requireCurrentProjectPermission(input.projectId, "safety:manage");
  const requestId = correlationId(input.requestId);
  return withAuditedProjectWrite({
    projectId: input.projectId, teamId: access.teamId, requestId, actorUserId: user.id,
    action: "safety.emergency_stop_drill", resourceType: "task_run", resourceId: String(input.taskRunId),
    input: { dryRun: true, outcome: input.outcome }, policyResult: { permission: "safety:manage", effect: "none" }
  }, async (client) => {
    const run = (await client.query<{ connected: boolean; capabilityDeclared: boolean }>(
      `select device.status in('online','degraded') as connected,
        exists(select 1 from device_capabilities capability where capability.device_id=device.id
          and capability.project_id=device.project_id and capability.capability_code in('safety.emergency_stop','flight.return_home','command.rth')) as "capabilityDeclared"
        from task_runs run join devices device on device.id=run.selected_device_id and device.project_id=run.project_id
        where run.project_id=$1 and run.id=$2`, [input.projectId, input.taskRunId]
    )).rows[0];
    if (!run) throw new Error("TASK_RUN_NOT_FOUND");
    return planEmergencyStopDrill({
      projectId: input.projectId, taskRunId: input.taskRunId, requestId, actorUserId: user.id,
      deviceConnected: run.connected, capabilityDeclared: run.capabilityDeclared, outcome: input.outcome
    });
  });
}
