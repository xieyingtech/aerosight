import assert from "node:assert/strict";
import test from "node:test";

import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";
import { activeProjectStreams, hasTransitionalLiveStream, isLiveStreamPlayable, liveStreamPollDecision, resolveWorkbenchSelection, workbenchQuery } from "./realtime-workbench-core.ts";

test("selection updates retain static project scope and filters while replacing old selections", () => {
  const query = new URLSearchParams(workbenchQuery({deviceId: 12, streamId: 21}, "projectId=7&deviceId=11&deviceId=99&streamId=5&layer=a&layer=b"));
  assert.equal(query.get("projectId"), "7");
  assert.deepEqual(query.getAll("deviceId"), ["12"]);
  assert.equal(query.get("streamId"), "21");
  assert.deepEqual(query.getAll("layer"), ["a", "b"]);
  const cleared = new URLSearchParams(workbenchQuery({deviceId:null,streamId:null}, query.toString()));
  assert.equal(cleared.get("projectId"), "7");
  assert.equal(cleared.has("deviceId"), false);
  assert.equal(cleared.has("streamId"), false);
});

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
