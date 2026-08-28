import "server-only";

import { randomUUID } from "node:crypto";
import { z } from "zod";
import { withAuditedProjectWrite } from "@/lib/audit";
import { shouldQueueCopilotMention } from "@/lib/copilot-mention-core";
import { requireCurrentProjectPermission } from "@/lib/data";
import { planIssueMutation, type IssueMutation } from "@/lib/issue-collaboration-core";
import { correlationId } from "@/lib/observability";
import { publishProjectEvent } from "@/lib/project-events";

export const issueMutationInputSchema = z.object({
  expectedVersion: z.number().int().nonnegative(),
  clientKey: z.string().uuid(),
  mutation: z.discriminatedUnion("action", [
    z.object({ action: z.literal("comment"), body: z.string() }).strict(),
    z.object({ action: z.literal("status"), status: z.enum(["open", "closed"]) }).strict(),
    z.object({ action: z.literal("labels"), labels: z.array(z.string()) }).strict(),
    z.object({ action: z.enum(["assign", "unassign"]), assigneeType: z.enum(["user", "agent"]), assigneeId: z.number().int().positive() }).strict()
  ])
}).strict();

export async function mutateIssue(projectId: number, issueId: number, input: {
  expectedVersion: number; clientKey: string; mutation: IssueMutation;
}, requestId?: string | null) {
  const required = input.mutation.action === "assign" || input.mutation.action === "unassign" ? "issue:assign" : "issue:handle";
  const { user, access } = await requireCurrentProjectPermission(projectId, required);
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, actorUserId: user.id, requestId: correlationId(requestId),
    idempotencyKey: input.clientKey, action: `issue.${input.mutation.action}`,
    resourceType: "issue", resourceId: String(issueId), input, policyResult: { permission: required, optimisticConcurrency: true }
  }, async (client) => {
    const issue = (await client.query<{ stateVersion: number }>(
      `select state_version as "stateVersion" from issues where project_id=$1 and id=$2 for update`, [projectId, issueId]
    )).rows[0];
    if (!issue) throw new Error("ISSUE_NOT_FOUND");
    const replay = (await client.query<{ id: number }>(
      `select id from issue_events where project_id=$1 and issue_id=$2 and client_key=$3`, [projectId, issueId, input.clientKey]
    )).rows[0];
    if (replay) return { issueId, stateVersion: issue.stateVersion, replayed: true };
    const plan = planIssueMutation({ mutation: input.mutation, permissions: access.permissions, actualVersion: issue.stateVersion, expectedVersion: input.expectedVersion });

    if (plan.assignment) {
      const { assigneeType, assigneeId, action } = plan.assignment;
      if (assigneeType === "user") {
        const scoped = (await client.query(`select 1 from team_members where team_id=$1 and user_id=$2`, [access.teamId, assigneeId])).rowCount;
        if (!scoped) throw new Error("ISSUE_ASSIGNEE_SCOPE_INVALID");
      } else {
        const scoped = (await client.query(`select 1 from agents where project_id=$1 and id=$2`, [projectId, assigneeId])).rowCount;
        if (!scoped) throw new Error("ISSUE_ASSIGNEE_SCOPE_INVALID");
      }
      const subjectColumn = assigneeType === "user" ? "user_id" : "agent_id";
      if (action === "assign") {
        const existing = (await client.query(`select 1 from issue_assignees where project_id=$1 and issue_id=$2 and ${subjectColumn}=$3 and active`, [projectId, issueId, assigneeId])).rowCount;
        if (!existing) await client.query(`insert into issue_assignees(project_id,team_id,issue_id,assignee_type,${subjectColumn},assigned_by_user_id) values($1,$2,$3,$4,$5,$6)`, [projectId, access.teamId, issueId, assigneeType, assigneeId, user.id]);
      } else {
        await client.query(`update issue_assignees set active=false,removed_at=now() where project_id=$1 and issue_id=$2 and ${subjectColumn}=$3 and active`, [projectId, issueId, assigneeId]);
      }
    }
    const updated = (await client.query<{ stateVersion: number }>(`update issues set
      status=coalesce($3,status),labels_json=coalesce($4,labels_json),state_version=$5,updated_at=now(),
      closed_at=case when $3='closed' then now() when $3='open' then null else closed_at end
      where project_id=$1 and id=$2 and state_version=$6 returning state_version as "stateVersion"`,
    [projectId, issueId, "status" in plan ? plan.status : null, "labels" in plan ? plan.labels : null, plan.nextVersion, issue.stateVersion])).rows[0];
    if (!updated) throw new Error("ISSUE_VERSION_CONFLICT");
    const activity = (await client.query<{ id: number }>(`insert into issue_events(project_id,issue_id,event_type,body,metadata_json,actor_user_id,client_key)
      values($1,$2,$3,$4,$5,$6,$7) returning id`, [projectId, issueId, plan.eventType, plan.body, plan.metadata, user.id, input.clientKey])).rows[0];
    let copilotJobId: string | null = null;
    if (input.mutation.action === "comment" && shouldQueueCopilotMention(plan.body ?? "", access.permissions)) {
      const session = (await client.query<{ id: number }>(`insert into agent_sessions(project_id,issue_id,started_by_user_id,summary)
        values($1,$2,$3,$4) returning id`, [projectId, issueId, user.id, `Copilot · 案件 #${issueId}`])).rows[0];
      const idempotencyKey = `issue-comment:${activity.id}:copilot`;
      const job = (await client.query<{ id: string }>(`insert into agent_tool_jobs(
        project_id,team_id,session_id,requested_by_user_id,issue_id,trigger_issue_event_id,trigger_type,idempotency_key,
        tool_name,required_permission,args_json,context_expires_at)
        values($1,$2,$3,$4,$5,$6,'issue_mention',$7,'issue_copilot','agent:use',
          jsonb_build_object('issueId',$5::int,'commentEventId',$6::int),now()+interval '24 hours')
        on conflict(project_id,idempotency_key) where idempotency_key is not null do update set idempotency_key=excluded.idempotency_key
        returning id`, [projectId, access.teamId, session.id, user.id, issueId, activity.id, idempotencyKey])).rows[0];
      copilotJobId = job.id;
      await client.query(`insert into issue_events(project_id,issue_id,event_type,metadata_json,actor_user_id)
        values($1,$2,'copilot.requested',jsonb_build_object('jobId',$3::text,'sessionId',$4::int,'triggerEventId',$5::int),$6)`,
      [projectId, issueId, job.id, session.id, activity.id, user.id]);
    }
    await publishProjectEvent(client, { projectId, teamId: access.teamId, eventId: randomUUID(), eventType: "issue.updated", payload: { issueId, stateVersion: updated.stateVersion, action: input.mutation.action }, enqueue: false });
    return { issueId, stateVersion: updated.stateVersion, replayed: false, copilotJobId };
  });
}
