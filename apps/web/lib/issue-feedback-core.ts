import { z } from "zod";

export const issueFeedbackInputSchema = z.object({
  expectedVersion: z.number().int().nonnegative(),
  clientKey: z.string().uuid(),
  detectionId: z.number().int().positive(),
  action: z.enum(["confirm","false_positive","category_correction","disposition"]),
  correctedLabel: z.string().trim().min(1).max(120).optional(),
  disposition: z.enum(["resolved","monitoring","remediated","accepted_risk","not_applicable"]).optional(),
  reason: z.string().trim().min(1).max(2000)
}).strict().superRefine((input, context) => {
  if (input.action === "category_correction" && !input.correctedLabel) context.addIssue({ code: "custom",message: "ISSUE_FEEDBACK_CORRECTED_LABEL_REQUIRED",path: ["correctedLabel"] });
  if (input.action !== "category_correction" && input.correctedLabel) context.addIssue({ code: "custom",message: "ISSUE_FEEDBACK_CORRECTED_LABEL_UNEXPECTED",path: ["correctedLabel"] });
  if (input.action === "disposition" && !input.disposition) context.addIssue({ code: "custom",message: "ISSUE_FEEDBACK_DISPOSITION_REQUIRED",path: ["disposition"] });
});

export function planIssueFeedback(input: z.infer<typeof issueFeedbackInputSchema>, evidence: {
  issueProjectId: number; projectId: number; issueVersion: number; detectionId: number; originalLabel: string;
  algorithmDefinitionVersionId: number; taskVersionId: number | null; taskRunStepId: number | null;
}) {
  const parsed = issueFeedbackInputSchema.parse(input);
  if (evidence.issueProjectId !== evidence.projectId || evidence.detectionId !== parsed.detectionId) throw new Error("ISSUE_FEEDBACK_SCOPE_MISMATCH");
  if (evidence.issueVersion !== parsed.expectedVersion) throw new Error("ISSUE_VERSION_CONFLICT");
  return Object.freeze({
    ...parsed,
    evidenceSnapshot: {
      detectionId: evidence.detectionId,originalLabel: evidence.originalLabel,
      algorithmDefinitionVersionId: evidence.algorithmDefinitionVersionId,
      taskVersionId: evidence.taskVersionId,taskRunStepId: evidence.taskRunStepId
    },
    issuePatch: { stateVersion: evidence.issueVersion + 1 },
    detectionPatch: null
  });
}
