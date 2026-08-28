import assert from "node:assert/strict";
import test from "node:test";

import { createLiveStreamPanelModel } from "./live-stream-panel-model.ts";
import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";

const snapshot = {
  project: { id: 17, name: "P", teamId: 5 }, generatedAt: "2026-08-27T00:10:00Z", consistency: "repeatable-read",
  devices: [{ id: 1 }, { id: 2 }], tracks: [], activeTasks: [],
  liveStreams: [{ id: 7, deviceId: 1, status: "live" }, { id: 8, deviceId: 2, status: "degraded" }],
  mediaPoints: [
    { id: 20, deviceId: 1, capturedAt: "2026-08-27T00:05:00Z" },
    { id: 21, deviceId: 1, capturedAt: "2026-08-27T00:08:00Z" }
  ],
  suspectedConstruction: [], openIssues: [], openAlerts: [], regions: [],
  freshness: { latestCapturedAt: null, isRealtime: true }, availability: {}
} satisfies ProjectSituationSnapshot;

test("selected device drives the active live stream", () => {
  const model = createLiveStreamPanelModel({
    snapshot, selection: { lane: "device-drone", entityId: "2" }, mode: "live", cursor: null
  });
  assert.equal(model.stream?.id, 8);
});

test("device without a stream exposes an explicit empty live state", () => {
  const model = createLiveStreamPanelModel({
    snapshot, selection: { lane: "device-drone", entityId: "99" }, mode: "live", cursor: null
  });
  assert.equal(model.stream, null);
});

test("history mode selects the latest media at or before the cursor", () => {
  const model = createLiveStreamPanelModel({
    snapshot, selection: { lane: "device-drone", entityId: "1" }, mode: "history",
    cursor: "2026-08-27T00:06:00Z"
  });
  assert.equal(model.media?.id, 20);
  assert.equal(model.stream, null);
});
