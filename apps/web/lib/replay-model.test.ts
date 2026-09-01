import assert from "node:assert/strict";
import test from "node:test";
import { applyReplayToSnapshot } from "./replay-model.ts";
import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";

const snapshot = { project: { id: 7, name: "P", teamId: 1 }, generatedAt: "2026-08-24T11:00:00Z", consistency: "repeatable-read", devices: [], tracks: [], activeTasks: [], taskSteps: [], algorithmRuns: [], liveStreams: [], mediaPoints: [], suspectedConstruction: [], openIssues: [], openAlerts: [], regions: [], freshness: { latestCapturedAt: null, isRealtime: true }, availability: {} } satisfies ProjectSituationSnapshot;

test("replay poses become historical device positions and tracks", () => {
  const replay = applyReplayToSnapshot(snapshot, { projectId: 7, mode: "replay", window: { from: "2026-08-24T10:00:00Z", to: "2026-08-24T11:00:00Z" }, filters: { deviceTypes: [], bbox: null }, poses: [
    { deviceId: 1, deviceName: "Drone", deviceType: "drone", longitude: 120, latitude: 30, capturedAt: "2026-08-24T10:01:00Z" },
    { deviceId: 1, deviceName: "Drone", deviceType: "drone", longitude: 121, latitude: 31, capturedAt: "2026-08-24T10:02:00Z" }
  ], media: [], events: [], truncated: false });
  assert.equal(replay.devices.length, 1);
  assert.equal(replay.tracks.length, 1);
  assert.equal(replay.freshness.isRealtime, false);
  assert.throws(() => applyReplayToSnapshot(snapshot, { projectId: 8 } as never), /SCOPE_MISMATCH/);
});
