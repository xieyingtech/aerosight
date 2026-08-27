import "server-only";

import { withAuditedProjectWrite } from "@/lib/audit";
import { requireProjectPermissionForUser } from "@/lib/project-access";
import type { AgentExecutionContext } from "@/lib/agent-execution-context-core";
import { planAgentDraft, type AgentDraftToolName } from "@/lib/agent-draft-tools-core";
import { agentToolRegistry } from "@/lib/agent-tool-registry";
import { correlationId } from "@/lib/observability";
import { evidenceVersionHash } from "@/lib/generated-alert-draft-core";

export type AgentDraftGenerationMetadata={modelId:string;promptTemplateVersion:string;toolCalls:unknown[];generatedAt:Date};

export async function executeAgentDraftTool(context: AgentExecutionContext, name: AgentDraftToolName, rawInput: unknown, requestId?: string | null, generation?:AgentDraftGenerationMetadata) {
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
      `insert into agent_drafts(project_id,team_id,session_id,created_by_user_id,draft_type,status,title,payload_json,
        model_id,prompt_template_version,generation_tool_calls_json,evidence_version_hash,generated_at)
       values($1,$2,$3,$4,$5,'draft',$6,$7,$8,$9,$10,$11,$12) returning id`,
      [draft.projectId,draft.teamId,draft.sessionId,draft.userId,draft.draftType,draft.title,draft.payload,
        generation?.modelId??null,generation?.promptTemplateVersion??null,generation?.toolCalls??[],
        generation?evidenceVersionHash(draft.evidenceRefs):null,generation?.generatedAt??null]
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
