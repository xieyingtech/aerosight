import assert from "node:assert/strict";
import test from "node:test";

import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";
import { activeProjectStreams, hasTransitionalLiveStream, isLiveStreamPlayable, liveStreamPollDecision, resolveWorkbenchSelection, workbenchQuery } from "./realtime-workbench-core.ts";

const snapshot = {
  project: { id: 7, name: "North", teamId: 3 }, generatedAt: "2026-08-28T00:00:00Z", consistency: "repeatable-read",
  devices: [{ id: 11, name: "Dock" }, { id: 12, name: "Drone" }], tracks: [], activeTasks: [],
  liveStreams: [{ id: 21, deviceId: 12, status: "starting" }, { id: 22, deviceId: 11, status: "stopped" }],
  mediaPoints: [], suspectedConstruction: [], openIssues: [], openAlerts: [], regions: [],
  freshness: { latestCapturedAt: null, isRealtime: true }, availability: {}
} satisfies ProjectSituationSnapshot;

test("stream deep link owns device selection and foreign identifiers fail closed", () => {
  assert.deepEqual(resolveWorkbenchSelection(snapshot, { deviceId: 11, streamId: 21 }), { deviceId: 12, streamId: 21 });
  assert.deepEqual(resolveWorkbenchSelection(snapshot, { deviceId: 999, streamId: 888 }), { deviceId: null, streamId: null });
});

test("device deep link selects its active stream and keeps devices without streams", () => {
  assert.deepEqual(resolveWorkbenchSelection(snapshot, { deviceId: 12 }), { deviceId: 12, streamId: 21 });
  assert.deepEqual(resolveWorkbenchSelection(snapshot, { deviceId: 11 }), { deviceId: 11, streamId: null });
});

test("query and transition helpers expose only active project state", () => {
  assert.equal(workbenchQuery({ deviceId: 12, streamId: 21 }), "deviceId=12&streamId=21");
  assert.deepEqual(activeProjectStreams(snapshot).map((stream) => stream.id), [21]);
  assert.equal(hasTransitionalLiveStream(snapshot), true);
  assert.equal(isLiveStreamPlayable("starting"), false);
  assert.equal(isLiveStreamPlayable("live"), true);
  assert.equal(liveStreamPollDecision(snapshot, 14), "poll");
  assert.equal(liveStreamPollDecision(snapshot, 15), "timeout");
  assert.equal(liveStreamPollDecision({ ...snapshot, liveStreams: [] }, 0), "stable");
});
