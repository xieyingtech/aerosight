import assert from "node:assert/strict";
import test from "node:test";
import { assertLiveControlRequest, ReplayControlForbiddenError, requestOperationMode } from "./replay-policy.ts";

test("replay mode is detected from immutable request context", () => {
  assert.equal(requestOperationMode(new Request("https://example.test/control?mode=replay")), "replay");
  assert.equal(requestOperationMode(new Request("https://example.test/control", { headers: { "X-AeroSight-Mode": "replay" } })), "replay");
});

test("control endpoint fails closed when invoked from replay", () => {
  assert.throws(
    () => assertLiveControlRequest(new Request("https://example.test/control?mode=replay")),
    (error) => error instanceof ReplayControlForbiddenError && error.code === "REPLAY_CONTROL_FORBIDDEN"
  );
  assert.doesNotThrow(() => assertLiveControlRequest(new Request("https://example.test/control")));
});
