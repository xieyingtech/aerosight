import assert from "node:assert/strict";
import test from "node:test";
import { issueFeedbackInputSchema,planIssueFeedback } from "./issue-feedback-core.ts";

const input = { expectedVersion: 3,clientKey: "8f85c7c7-600e-4d16-93aa-5146d16eebfd",detectionId: 9,
  action: "category_correction" as const,correctedLabel: "extension",reason: "人工核验类别" };
const evidence = { issueProjectId: 7,projectId: 7,issueVersion: 3,detectionId: 9,originalLabel: "building",
  algorithmDefinitionVersionId: 12,taskVersionId: 4,taskRunStepId: 21 };

test("feedback pins detection model condition and task version without mutating detection", () => {
  const plan = planIssueFeedback(input,evidence);
  assert.deepEqual(plan.evidenceSnapshot,{ detectionId: 9,originalLabel: "building",algorithmDefinitionVersionId: 12,taskVersionId: 4,taskRunStepId: 21 });
  assert.equal(plan.detectionPatch,null);
});

test("feedback rejects missing correction disposition stale version and cross-project evidence", () => {
  assert.throws(() => issueFeedbackInputSchema.parse({ ...input,correctedLabel: undefined }), /CORRECTED_LABEL_REQUIRED/);
  assert.throws(() => issueFeedbackInputSchema.parse({ ...input,action: "disposition",correctedLabel: undefined }), /DISPOSITION_REQUIRED/);
  assert.throws(() => planIssueFeedback(input,{ ...evidence,issueVersion: 4 }), /VERSION_CONFLICT/);
  assert.throws(() => planIssueFeedback(input,{ ...evidence,issueProjectId: 8 }), /SCOPE_MISMATCH/);
});
