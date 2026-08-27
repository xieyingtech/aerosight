import { z } from "zod";
import { inspectionMissionDefinitionSchema } from "./inspection-mission-schema.ts";
import { assertAgentToolArgsDoNotContainScope, type AgentExecutionContext } from "./agent-execution-context-core.ts";

export const agentEvidenceReferenceSchema = z.object({
  type: z.enum(["asset", "event", "detection", "track", "task_run"]),
  id: z.string().trim().min(1).max(200),
  version: z.string().trim().min(1).max(100),
  observedAt: z.string().datetime(),
  quality: z.string().trim().min(1).max(100)
}).strict();

export const inspectionTaskDraftInputSchema = z.object({
  definition: inspectionMissionDefinitionSchema,
  evidenceRefs: z.array(agentEvidenceReferenceSchema).max(100).default([])
}).strict();

export const reportDraftInputSchema = z.object({
  title: z.string().trim().min(1).max(200),
  sections: z.array(z.object({ heading: z.string().trim().min(1), body: z.string().trim().min(1) }).strict()).min(1).max(50),
  evidenceRefs: z.array(agentEvidenceReferenceSchema).min(1).max(100)
}).strict();

export const issueDraftInputSchema = z.object({
  title: z.string().trim().min(1).max(200),
  description: z.string().trim().min(1).max(10_000),
  priority: z.enum(["low", "medium", "high", "critical"]),
  evidenceRefs: z.array(agentEvidenceReferenceSchema).min(1).max(100)
}).strict();

export type AgentDraftToolName = "draft_inspection_task" | "draft_report" | "draft_issue";

export function planAgentDraft(context: AgentExecutionContext, name: AgentDraftToolName, rawInput: unknown) {
  assertAgentToolArgsDoNotContainScope(rawInput);
  const input = name === "draft_inspection_task"
    ? inspectionTaskDraftInputSchema.parse(rawInput)
    : name === "draft_report"
      ? reportDraftInputSchema.parse(rawInput)
      : issueDraftInputSchema.parse(rawInput);
  const draftType = name === "draft_inspection_task" ? "inspection_task" : name === "draft_report" ? "report" : "issue";
  const title = "definition" in input ? input.definition.name : input.title;
  return Object.freeze({
    projectId: context.projectId,
    teamId: context.teamId,
    sessionId: context.sessionId,
    userId: context.userId,
    draftType,
    status: "draft" as const,
    title,
    payload: input,
    evidenceRefs: input.evidenceRefs
  });
}
