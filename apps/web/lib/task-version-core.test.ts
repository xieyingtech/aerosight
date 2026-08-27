import assert from "node:assert/strict";
import test from "node:test";

import { assertDraftPublishable } from "./task-version-core.ts";

const draft = {
  status: "draft" as const,
  definition: {
    name: "河道巡检",
    objective: "采集重点河段的可核验证据",
    spatialScope: {
      type: "route", coordinates: [[121.47, 31.23, 80], [121.48, 31.24, 90]],
      maxAltitudeMeters: 120, maxSpeedMetersPerSecond: 12
    },
    requiredCapabilities: [{ code: "flight.navigate", constraints: {} }],
    trigger: { type: "manual" },
    concurrencyLimit: 1,
    reportTemplate: { templateKey: "inspection-default-v1" }
  },
  script: "inspect",
  steps: [{
    position: 1, stepKey: "capture", name: "定点拍摄", capabilityCode: "camera.capture",
    action: "camera.capture", parameters: {},
    failurePolicy: { onFailure: "pause", maxRetries: 2, retryBackoffSeconds: 5, idempotency: "safe" },
    mediaRequirements: { required: true, modes: ["photo"], minimumCount: 1 }
  }]
};

test("complete draft is publishable", () => assert.doesNotThrow(() => assertDraftPublishable(draft)));
test("published or step-less version cannot be republished", () => {
  assert.throws(() => assertDraftPublishable({ ...draft, status: "published" }), /NOT_DRAFT/);
  assert.throws(() => assertDraftPublishable({ ...draft, steps: [] }), /STEPS_REQUIRED/);
});
test("duplicate step positions or keys are rejected", () => {
  assert.throws(() => assertDraftPublishable({ ...draft, steps: [draft.steps[0], { ...draft.steps[0] }] }), /POSITION_INVALID|STEP_INVALID/);
});
