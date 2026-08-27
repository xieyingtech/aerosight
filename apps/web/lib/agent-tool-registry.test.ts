import assert from "node:assert/strict";
import test from "node:test";
import { agentToolRegistrySnapshot, parseAgentToolInput } from "./agent-tool-registry.ts";

test("agent tool whitelist snapshot declares risk permission and confirmation", () => {
  assert.deepEqual(agentToolRegistrySnapshot(), [
    { name: "query_devices", risk: "read-only", permission: "project:view", confirmation: "never" },
    { name: "query_missions", risk: "read-only", permission: "project:view", confirmation: "never" },
    { name: "query_events", risk: "read-only", permission: "project:view", confirmation: "never" },
    { name: "query_assets", risk: "read-only", permission: "project:view", confirmation: "never" },
    { name: "query_tracks", risk: "read-only", permission: "project:view", confirmation: "never" },
    { name: "query_map_context", risk: "read-only", permission: "project:view", confirmation: "never" },
    { name: "draft_inspection_task", risk: "draft", permission: "mission:operate", confirmation: "never" },
    { name: "draft_report", risk: "draft", permission: "agent:use", confirmation: "never" },
    { name: "draft_issue", risk: "draft", permission: "event:handle", confirmation: "never" },
    { name: "request_mission_start", risk: "protected", permission: "mission:operate", confirmation: "required" }
  ]);
});

test("whitelist has no direct device adapter tool", () => {
  const names = agentToolRegistrySnapshot().map((tool) => tool.name);
  assert.equal(names.some((name) => /adapter|command|mqtt|ros|webrtc/i.test(name)), false);
});

test("tool schemas reject undeclared scope and invalid protected requests", () => {
  assert.throws(() => parseAgentToolInput("query_devices", { projectId: 999 }));
  assert.throws(() => parseAgentToolInput("request_mission_start", { taskVersionId: 3, approvalRequestId: "missing", idempotencyKey: "not-a-uuid" }));
  assert.deepEqual(parseAgentToolInput("query_devices", { deviceIds: [3] }), { deviceIds: [3] });
});
