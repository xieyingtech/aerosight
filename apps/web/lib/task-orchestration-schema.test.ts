import assert from "node:assert/strict";
import test from "node:test";
import { validInspectionMission } from "./inspection-mission-schema.test.ts";
import { assertTaskResourcesCompatible, migrateLegacyInspectionTask, taskOrchestrationDefinitionSchema } from "./task-orchestration-schema.ts";

const definition = {
  name: "自动巡检并建案",
  description: "采集、识别并按条件创建案件",
  inputSchema: { type: "object", properties: { minimumConfidence: { type: "number" } }, required: ["minimumConfidence"], additionalProperties: false },
  trigger: { type: "schedule", cron: "0 */2 * * *", timezone: "Asia/Shanghai", enabled: true },
  concurrencyLimit: 1,
  steps: [
    { key: "collect", name: "无人机采集", uses: "device.collect", requires: ["camera.capture"], with: {},
      inputSchema: { type: "object", properties: {}, required: [], additionalProperties: false }, outputSchema: { type: "object", properties: {}, required: [], additionalProperties: true },
      dependsOn: [], timeoutSeconds: 600, retry: { maxAttempts: 2, backoffSeconds: 10 }, onFailure: "abort" },
    { key: "detect", name: "运行识别算法", uses: "algorithm.run", requires: [], with: {
      assetId: "steps.collect.outputs.assetId", definitionVersionId: 1, parameters: {}
    },
      inputSchema: { type: "object", properties: {}, required: [], additionalProperties: true }, outputSchema: { type: "object", properties: {}, required: [], additionalProperties: true },
      dependsOn: ["collect"], timeoutSeconds: 300, retry: { maxAttempts: 2, backoffSeconds: 5 }, onFailure: "abort" },
    { key: "createIssue", name: "创建案件", uses: "issue.create-or-update", requires: [], with: {},
      inputSchema: { type: "object", properties: {}, required: [], additionalProperties: true }, outputSchema: { type: "object", properties: {}, required: [], additionalProperties: true },
      condition: { op: "gte", left: { ref: "steps.detect.outputs.maxConfidence" }, right: { ref: "inputs.minimumConfidence" } },
      dependsOn: ["detect"], timeoutSeconds: 30, retry: { maxAttempts: 1, backoffSeconds: 0 }, onFailure: "pause" }
  ]
};

test("accepts a typed end-to-end task template", () => {
  assert.equal(taskOrchestrationDefinitionSchema.parse(definition).steps[2].uses, "issue.create-or-update");
});

test("device steps reject resources without required capabilities", () => {
  const parsed = taskOrchestrationDefinitionSchema.parse(definition);
  assert.doesNotThrow(() => assertTaskResourcesCompatible(parsed, { collect: ["camera.capture"] }));
  assert.throws(() => assertTaskResourcesCompatible(parsed, { collect: ["flight.navigate"] }), /TASK_RESOURCE_INCOMPATIBLE:collect:camera.capture/);
});

test("legacy inspection tasks migrate to stable typed steps", () => {
  const migrated = migrateLegacyInspectionTask(validInspectionMission);
  assert.equal(migrated.trigger.type, "manual");
  assert.deepEqual(migrated.steps.map((step) => ({ key: step.key, uses: step.uses, requires: step.requires })), [
    { key: "capture", uses: "device.collect", requires: ["camera.capture"] }
  ]);
  assert.equal(taskOrchestrationDefinitionSchema.safeParse(migrated).success, true);
});
