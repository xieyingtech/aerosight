import assert from "node:assert/strict";
import test from "node:test";

import { availableMissionActions, requiredMissionPermission } from "./mission-workbench-core.ts";
import type { ProjectPermission } from "./project-permission-policy.ts";

test("read-only member sees no mission controls", () => {
  assert.deepEqual(availableMissionActions("running", new Set<ProjectPermission>(["project:view"])), []);
});

test("operator and approver controls remain independently permissioned", () => {
  assert.deepEqual(availableMissionActions("paused", new Set<ProjectPermission>(["project:view", "mission:operate"])), ["resume", "cancel", "emergency_stop"]);
  assert.deepEqual(availableMissionActions("ready", new Set<ProjectPermission>(["project:view", "mission:approve"])), ["approve"]);
  assert.equal(requiredMissionPermission("emergency_stop"), "mission:operate");
  assert.equal(requiredMissionPermission("approve"), "mission:approve");
});
