import "server-only";

import { randomUUID } from "node:crypto";
import { withAuditedProjectWrite } from "@/lib/audit";
import { requireCurrentProjectPermission } from "@/lib/data";
import { correlationId } from "@/lib/observability";
import { publishProjectEvent } from "@/lib/project-events";
import { issueFeedbackInputSchema,planIssueFeedback } from "@/lib/issue-feedback-core";

export async function recordIssueFeedback(projectId: number,issueId: number,rawInput: unknown,requestId?: string|null) {
  const input = issueFeedbackInputSchema.parse(rawInput);
  const { user,access } = await requireCurrentProjectPermission(projectId,"issue:handle");
  return withAuditedProjectWrite({ projectId,teamId: access.teamId,requestId: correlationId(requestId),
    idempotencyKey: input.clientKey,actorUserId: user.id,action: "issue.feedback.record",resourceType: "issue",resourceId: String(issueId),
    input: { action: input.action,detectionId: input.detectionId,expectedVersion: input.expectedVersion },
    policyResult: { permission: "issue:handle" }
  }, async (client) => {
    const replay = (await client.query<Record<string,unknown>>(`select id,action,created_at as "createdAt" from issue_feedback
      where project_id=$1 and issue_id=$2 and client_key=$3`,[projectId,issueId,input.clientKey])).rows[0];
    if (replay) return { ...replay,replayed: true };
    const evidence = (await client.query<{
      issueProjectId: number; issueVersion: number; detectionId: number; originalLabel: string;
      algorithmDefinitionVersionId: number; taskVersionId: number|null; taskRunStepId: number|null;
    }>(`select issue.project_id as "issueProjectId",issue.state_version as "issueVersion",detection.id::int as "detectionId",
      detection.label as "originalLabel",run.algorithm_definition_version_id::int as "algorithmDefinitionVersionId",
      issue.task_version_id::int as "taskVersionId",(
        select case when link.target_id~'^[0-9]+$' then link.target_id::bigint end from issue_links link
        where link.project_id=issue.project_id and link.issue_id=issue.id and link.link_type='task_step' order by link.id desc limit 1
      )::int as "taskRunStepId"
      from issues issue join issue_links detection_link on detection_link.project_id=issue.project_id
        and detection_link.issue_id=issue.id and detection_link.link_type='detection'
      join detections detection on detection.project_id=detection_link.project_id
        and detection.id=case when detection_link.target_id~'^[0-9]+$' then detection_link.target_id::bigint end
      join algorithm_runs run on run.id=detection.algorithm_run_id and run.project_id=detection.project_id
      where issue.project_id=$1 and issue.id=$2 and detection.id=$3 for update of issue`,[projectId,issueId,input.detectionId])).rows[0];
    if (!evidence) throw new Error("ISSUE_FEEDBACK_EVIDENCE_NOT_FOUND");
    const plan = planIssueFeedback(input,{ ...evidence,projectId });
    const feedback = (await client.query<{ id: number; createdAt: string }>(`insert into issue_feedback(
      project_id,team_id,issue_id,detection_id,algorithm_definition_version_id,task_version_id,task_run_step_id,
      action,corrected_label,disposition,reason,client_key,evidence_snapshot_json,actor_user_id)
      values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) returning id,created_at as "createdAt"`,
      [projectId,access.teamId,issueId,input.detectionId,evidence.algorithmDefinitionVersionId,evidence.taskVersionId,
        evidence.taskRunStepId,input.action,input.correctedLabel ?? null,input.disposition ?? null,input.reason,input.clientKey,
        plan.evidenceSnapshot,user.id])).rows[0];
    const eventType = `feedback.${input.action}`;
    await client.query(`insert into issue_events(project_id,issue_id,event_type,body,metadata_json,actor_user_id,client_key)
      values($1,$2,$3,$4,$5,$6,$7)`,[projectId,issueId,eventType,input.reason,{ feedbackId: feedback.id,...plan.evidenceSnapshot,
        correctedLabel: input.correctedLabel ?? null,disposition: input.disposition ?? null },user.id,`feedback:${input.clientKey}`]);
    await client.query(`update issues set state_version=state_version+1,updated_at=now() where project_id=$1 and id=$2`,[projectId,issueId]);
    await publishProjectEvent(client,{ projectId,teamId: access.teamId,eventId: randomUUID(),eventType: "issue.updated",
      payload: { issueId,feedbackId: feedback.id,feedbackAction: input.action,stateVersion: plan.issuePatch.stateVersion },enqueue: false });
    return { id: feedback.id,action: input.action,createdAt: feedback.createdAt,stateVersion: plan.issuePatch.stateVersion,replayed: false };
  });
}
