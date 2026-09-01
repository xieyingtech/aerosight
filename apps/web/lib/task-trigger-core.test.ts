import assert from "node:assert/strict";
import test from "node:test";
import { assertUserCallableTaskTrigger, planTaskTrigger, taskTriggerInvocationSchema } from "./task-trigger-core.ts";

const authorization = { projectId: 7, taskProjectId: 7, taskVersionStatus: "published", taskStatus: "active", concurrencyLimit: 2, activeRunCount: 0, actor: { type: "user" as const, id: "11" } };
const inputSchema = { type: "object", properties: { areaId: { type: "number" } }, required: ["areaId"], additionalProperties: false };
const base = { idempotencyKey: "delivery-1", occurredAt: "2026-09-01T00:00:00.000Z", inputs: { areaId: 4 } };

test("every trigger type produces a stable namespaced key and actor snapshot", () => {
  const cases = [
    [{ type: "manual" }, { type: "manual", ...base }],
    [{ type: "schedule", cron: "*/5 * * * *", timezone: "UTC", enabled: true }, { type: "schedule", scheduledFor: base.occurredAt, ...base }],
    [{ type: "api", key: "integration-a" }, { type: "api", key: "integration-a", ...base }],
    [{ type: "webhook", source: "vendor-a" }, { type: "webhook", source: "vendor-a", deliveryId: "event-1", ...base }],
    [{ type: "upstream", taskId: 31, statuses: ["succeeded"] }, { type: "upstream", upstreamProjectId: 7, upstreamTaskId: 31, upstreamRunId: 42, status: "succeeded", ...base }],
    [{ type: "copilot", delegation: "chat" }, { type: "copilot", delegation: "chat", agentJobId: "job-1", ...base }]
  ] as const;
  for (const [trigger, invocation] of cases) {
    const plan = planTaskTrigger({ trigger, inputSchema, invocation, authorization });
    assert.equal(plan.triggerKey, `${invocation.type}:delivery-1`);
    assert.deepEqual((plan.inputSnapshot.trigger as { actor: unknown }).actor, authorization.actor);
    assert.deepEqual(plan.inputSnapshot.inputs, { areaId: 4 });
    assert(!JSON.stringify(plan.inputSnapshot).includes("integration-a"));
  }
});

test("disabled schedules, concurrency overflow and cross-project upstream input fail closed", () => {
  assert.throws(() => planTaskTrigger({ trigger: { type: "schedule", cron: "* * * * *", timezone: "UTC", enabled: false }, inputSchema, invocation: { type: "schedule", scheduledFor: base.occurredAt, ...base }, authorization }), /SCHEDULE_DISABLED/);
  assert.throws(() => planTaskTrigger({ trigger: { type: "manual" }, inputSchema, invocation: { type: "manual", ...base }, authorization: { ...authorization, activeRunCount: 2 } }), /CONCURRENCY_LIMIT/);
  assert.throws(() => planTaskTrigger({ trigger: { type: "upstream", taskId: 31, statuses: ["succeeded"] }, inputSchema, invocation: { type: "upstream", upstreamProjectId: 8, upstreamTaskId: 31, upstreamRunId: 42, status: "succeeded", ...base }, authorization }), /UPSTREAM_MISMATCH/);
});

test("input snapshots enforce required and undeclared fields", () => {
  assert.throws(() => planTaskTrigger({ trigger: { type: "manual" }, inputSchema, invocation: { type: "manual", ...base, inputs: {} }, authorization }), /INPUT_REQUIRED/);
  assert.throws(() => planTaskTrigger({ trigger: { type: "manual" }, inputSchema, invocation: { type: "manual", ...base, inputs: { areaId: 4, projectId: 9 } }, authorization }), /INPUT_UNKNOWN/);
});

test("user-facing trigger endpoints cannot spoof schedule, upstream or Copilot sources", () => {
  assert.doesNotThrow(() => assertUserCallableTaskTrigger(taskTriggerInvocationSchema.parse({ type: "manual", ...base })));
  assert.throws(() => assertUserCallableTaskTrigger(taskTriggerInvocationSchema.parse({ type: "upstream", upstreamProjectId: 7, upstreamTaskId: 31, upstreamRunId: 42, status: "succeeded", ...base })), /INTERNAL_SOURCE_REQUIRED/);
});
