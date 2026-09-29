import assert from "node:assert/strict";
import test from "node:test";
import { agentFlightRunId } from "./agent-floating-flight.ts";

test("only successful authorized flight and task runs are watched", () => {
  assert.equal(agentFlightRunId("submit_flight", "succeeded", { taskRunId: 9 }, {}), 9);
  assert.equal(agentFlightRunId("launch_flight", "succeeded", {}, { output: { runId: 12 } }), 12);
  assert.equal(agentFlightRunId("launch_flight", "pending", {}, { output: { runId: 12 } }), null);
  assert.equal(agentFlightRunId("run_task", "succeeded", {}, { output: { taskRunId: 7 } }), 7);
  for (const status of ["pending", "executing", "failed", "rejected"]) assert.equal(agentFlightRunId("submit_flight", status, { taskRunId: 9 }, {}), null);
  assert.equal(agentFlightRunId("run_algorithm", "succeeded", {}, { output: { taskRunId: 7 } }), null);
  assert.equal(agentFlightRunId("submit_flight", "succeeded", { taskRunId: -1 }, {}), null);
  assert.equal(agentFlightRunId("run_task", "succeeded", {}, { output: { runId: 7 } }), null);
});
