import { test } from "node:test";
import assert from "node:assert/strict";
import { missionLiveHref } from "./mission-live-core.ts";

test("flight navigation waits for a remote task acknowledgement and excludes finished runs", () => {
  assert.equal(missionLiveHref(7, { id: 42, status: "running", deviceId: 12 }), null);
  assert.equal(missionLiveHref(7, { id: 42, status: "running", realtimeFlight: { deviceId: 12, taskUuid: "remote-task" } }), "/projects/7/realtime/devices/12/?autoLive=1&runId=42");
  for (const status of ["failed", "succeeded", "canceled", "blocked"]) {
    assert.equal(missionLiveHref(7, { id: 42, status, realtimeFlight: { deviceId: 12, taskUuid: "remote-task" } }), null);
  }
});
