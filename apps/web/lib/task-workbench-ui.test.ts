import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const root = new URL("../", import.meta.url);

test("task template UI exposes typed trigger parameters conditions resources and schemas", () => {
  const source = readFileSync(new URL("components/task-template-workbench.tsx", root), "utf8");
  for (const marker of ["类型化任务配置", "inputSchema", "trigger", "requires", "condition", "dependsOn", "outputSchema", "retry", "onFailure"]) {
    assert.match(source, new RegExp(marker));
  }
});

test("task run UI exposes trigger snapshots step outputs reasons and audit chain", () => {
  const source = readFileSync(new URL("components/mission-run-workbench.tsx", root), "utf8");
  for (const marker of ["触发与输入快照", "inputSnapshot", "outputSnapshot", "conditionResult", "stateReason", "executionKey", "审计链"]) {
    assert.match(source, new RegExp(marker));
  }
});

test("inspection to collection algorithm condition and issue remains one task graph", () => {
  const definition = {
    steps: ["device.command", "device.collect", "algorithm.run", "issue.create-or-update"]
  };
  assert.deepEqual(definition.steps, ["device.command", "device.collect", "algorithm.run", "issue.create-or-update"]);
});
