import "server-only";

import { withAuditedProjectWrite } from "@/lib/audit";
import { requireProjectPermissionForUser } from "@/lib/project-access";
import type { AgentExecutionContext } from "@/lib/agent-execution-context-core";
import { planAgentDraft, type AgentDraftToolName } from "@/lib/agent-draft-tools-core";
import { agentToolRegistry } from "@/lib/agent-tool-registry";
import { correlationId } from "@/lib/observability";

export async function executeAgentDraftTool(context: AgentExecutionContext, name: AgentDraftToolName, rawInput: unknown, requestId?: string | null) {
  const permission = agentToolRegistry[name].permission;
  await requireProjectPermissionForUser(context.userId, context.projectId, permission);
  const draft = planAgentDraft(context, name, rawInput);
  return withAuditedProjectWrite({
    projectId: context.projectId,
    teamId: context.teamId,
    actorUserId: context.userId,
    requestId: correlationId(requestId),
    action: `agent.${name}`,
    resourceType: "agent_session",
    resourceId: String(context.sessionId),
    input: draft.payload,
    policyResult: { permission, outcome: "draft-only" }
  }, async (client) => {
    const inserted = (await client.query<{ id: string }>(
      `insert into agent_drafts(project_id,team_id,session_id,created_by_user_id,draft_type,status,title,payload_json)
       values($1,$2,$3,$4,$5,'draft',$6,$7) returning id`,
      [draft.projectId,draft.teamId,draft.sessionId,draft.userId,draft.draftType,draft.title,draft.payload]
    )).rows[0];
    for (const evidence of draft.evidenceRefs) {
      await client.query(
        `insert into agent_draft_evidence(project_id,agent_draft_id,reference_type,reference_id,reference_version,observed_at,quality)
         values($1,$2,$3,$4,$5,$6,$7)`,
        [draft.projectId,inserted.id,evidence.type,evidence.id,evidence.version,evidence.observedAt,evidence.quality]
      );
    }
    return { id: inserted.id, type: draft.draftType, status: "draft" as const, evidenceCount: draft.evidenceRefs.length };
  });
}
