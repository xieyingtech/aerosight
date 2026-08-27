import assert from "node:assert/strict";
import test from "node:test";

import { applyCommandAck, transitionTaskRun } from "./task-run-core.ts";

test("legal transitions increment the optimistic version", () => {
  const ready = transitionTaskRun({ status: "queued", stateVersion: 4 }, 4, "ready", "preflight passed");
  const dispatching = transitionTaskRun(ready, 5, "dispatching", "approval complete");
  assert.deepEqual(dispatching, { status: "dispatching", stateVersion: 6, reason: "approval complete" });
});

test("illegal and stale transitions preserve deterministic failure", () => {
  assert.throws(() => transitionTaskRun({ status: "queued", stateVersion: 2 }, 1, "ready", "stale"), /VERSION_CONFLICT/);
  assert.throws(() => transitionTaskRun({ status: "succeeded", stateVersion: 2 }, 2, "running", "again"), /TRANSITION_INVALID/);
  assert.throws(() => transitionTaskRun({ status: "running", stateVersion: 2 }, 2, "succeeded", ""), /REASON_REQUIRED/);
});

test("known ACK advances only its command ledger entry", () => {
  const result = applyCommandAck([
    { commandId: "command-1", status: "sent" }, { commandId: "command-2", status: "sent" }
  ], { commandId: "command-1", outcome: "ack", result: { vendorSequence: 9 } });
  assert.equal(result.matched, true);
  assert.equal(result.entries[0].status, "acknowledged");
  assert.equal(result.entries[1].status, "sent");
});

test("unknown ACK is diagnosed without changing command or run state", () => {
  const entries = [{ commandId: "command-1", status: "sent" as const }];
  const result = applyCommandAck(entries, { commandId: "unknown", outcome: "ack" });
  assert.equal(result.matched, false);
  assert.equal(result.diagnostic, "UNKNOWN_COMMAND_ACK");
  assert.strictEqual(result.entries, entries);
});
