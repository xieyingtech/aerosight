import assert from "node:assert/strict";
import test from "node:test";
import { createAgentExecutionContext } from "./agent-execution-context-core.ts";
import {
  AGENT_TOOL_RESULT_LIMIT,
  formatAgentReadToolResult,
  prepareAgentReadToolCall
} from "./agent-read-tools-core.ts";

const context = createAgentExecutionContext({ userId: 7, teamId: 11, projectId: 17, sessionId: 23 });

test("read tool scope always comes from execution context", () => {
  const call = prepareAgentReadToolCall(context, "query_devices", { deviceIds: [3] });
  assert.equal(call.projectId, 17);
  assert.throws(() => prepareAgentReadToolCall(context, "query_devices", { projectId: 999 }));
  assert.throws(() => prepareAgentReadToolCall(context, "query_events", { window: { userId: 999 } }));
});

test("large read results are truncated and retain stable references and freshness", () => {
  const rows = Array.from({ length: AGENT_TOOL_RESULT_LIMIT + 5 }, (_, index) => ({
    id: index + 1,
    name: `device-${index + 1}`,
    observedAt: "2026-08-27T00:00:00.000Z",
    quality: "usable"
  }));
  const result = formatAgentReadToolResult(context, "query_devices", rows, new Date("2026-08-27T00:01:30.000Z"));
  assert.equal(result.items.length, AGENT_TOOL_RESULT_LIMIT);
  assert.equal(result.truncated, true);
  assert.equal(result.freshnessSeconds, 90);
  assert.deepEqual(result.items[0].reference, { type: "device", id: "1", href: "/projects/17/devices?selected=1" });
});

test("oversized payloads stop before entering model context", () => {
  const rows = [{ id: 1, content: "x".repeat(70 * 1024), observedAt: "2026-08-27T00:00:00.000Z" }];
  const result = formatAgentReadToolResult(context, "query_assets", rows);
  assert.equal(result.items.length, 0);
  assert.equal(result.truncated, true);
});
