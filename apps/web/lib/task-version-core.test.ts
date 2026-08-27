import assert from "node:assert/strict";
import test from "node:test";

import { assertDraftPublishable } from "./task-version-core.ts";

const draft = {
  status: "draft" as const,
  definition: { name: "河道巡检" },
  script: "inspect",
  steps: [{ position: 1, stepKey: "capture", action: "camera.capture" }]
};

test("complete draft is publishable", () => assert.doesNotThrow(() => assertDraftPublishable(draft)));
test("published or step-less version cannot be republished", () => {
  assert.throws(() => assertDraftPublishable({ ...draft, status: "published" }), /NOT_DRAFT/);
  assert.throws(() => assertDraftPublishable({ ...draft, steps: [] }), /STEPS_REQUIRED/);
});
test("duplicate step positions or keys are rejected", () => {
  assert.throws(() => assertDraftPublishable({ ...draft, steps: [draft.steps[0], { ...draft.steps[0] }] }), /POSITION_INVALID|STEP_INVALID/);
});
