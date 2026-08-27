import assert from "node:assert/strict";
import test from "node:test";

import { inspectionMissionDefinitionSchema } from "./inspection-mission-schema.ts";

export const validInspectionMission = {
  name: "河道巡检",
  objective: "采集重点河段的可核验证据",
  spatialScope: {
    type: "route" as const,
    coordinates: [[121.47, 31.23, 80], [121.48, 31.24, 90]],
    maxAltitudeMeters: 120,
    maxSpeedMetersPerSecond: 12
  },
  requiredCapabilities: [{ code: "flight.navigate", constraints: { positioning: "rtk" } }],
  trigger: { type: "manual" as const },
  concurrencyLimit: 1,
  reportTemplate: { templateKey: "inspection-default-v1" },
  steps: [{
    position: 1,
    stepKey: "capture",
    name: "定点拍摄",
    capabilityCode: "camera.capture",
    action: "camera.capture",
    parameters: { intervalSeconds: 3 },
    failurePolicy: { onFailure: "pause" as const, maxRetries: 2, retryBackoffSeconds: 5, idempotency: "safe" as const },
    mediaRequirements: { required: true, modes: ["photo" as const], minimumCount: 1 }
  }]
};

test("accepts a complete inspection template", () => {
  assert.equal(inspectionMissionDefinitionSchema.parse(validInspectionMission).name, "河道巡检");
});

const invalidFields: Array<[string, (input: typeof validInspectionMission) => void, string]> = [
  ["objective", (input) => { input.objective = ""; }, "objective"],
  ["route", (input) => { input.spatialScope.coordinates = [[181, 31.23, 80]]; }, "spatialScope"],
  ["capabilities", (input) => { input.requiredCapabilities = []; }, "requiredCapabilities"],
  ["trigger", (input) => { input.trigger = { type: "schedule", cron: "", timezone: "" } as never; }, "trigger"],
  ["step", (input) => { input.steps[0].action = ""; }, "steps"],
  ["failure policy", (input) => { input.steps[0].failurePolicy = { ...input.steps[0].failurePolicy, idempotency: "unsafe", maxRetries: 1 } as never; }, "failurePolicy"],
  ["media requirements", (input) => { input.steps[0].mediaRequirements.minimumCount = 0; }, "mediaRequirements"],
  ["report template", (input) => { input.reportTemplate.templateKey = ""; }, "reportTemplate"]
];

for (const [name, mutate, expectedPath] of invalidFields) {
  test(`rejects an invalid ${name}`, () => {
    const input = structuredClone(validInspectionMission);
    mutate(input);
    const result = inspectionMissionDefinitionSchema.safeParse(input);
    assert.equal(result.success, false);
    assert.ok(result.error.issues.some((issue) => issue.path.join(".").includes(expectedPath)));
  });
}

test("rejects an open area ring", () => {
  const input = structuredClone(validInspectionMission) as Record<string, unknown>;
  input.spatialScope = {
    type: "area", maxAltitudeMeters: 100, maxSpeedMetersPerSecond: 8,
    rings: [[[121.4, 31.2], [121.5, 31.2], [121.5, 31.3], [121.4, 31.3]]]
  };
  assert.equal(inspectionMissionDefinitionSchema.safeParse(input).success, false);
});
