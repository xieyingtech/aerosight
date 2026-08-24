import assert from "node:assert/strict";
import test from "node:test";
import { buildTimelineModel } from "./timeline-model.ts";
import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";

const fixed = {
  project: { id: 1, name: "Fixture", teamId: 1 }, generatedAt: "2026-08-24T10:10:00Z", consistency: "repeatable-read",
  devices: [
    { id: 1, name: "Drone", status: "online", pose: { capturedAt: "2026-08-24T10:00:01Z" } },
    { id: 2, name: "Robot", status: "offline", pose: { capturedAt: "2026-08-24T10:09:00Z" } }
  ],
  tracks: [], activeTasks: [], liveStreams: [],
  mediaPoints: [
    { id: 1, deviceId: 1, kind: "image", capturedAt: "2026-08-24T10:01:00Z" },
    { id: 2, deviceId: 1, kind: "image", capturedAt: "2026-08-24T10:01:01Z" }
  ],
  suspectedConstruction: [], openAlerts: [], regions: [],
  freshness: { latestCapturedAt: null, isRealtime: false }, availability: {}
} satisfies ProjectSituationSnapshot;

test("timeline has stable lane ordering and aggregates dense device media", () => {
  const model = buildTimelineModel(fixed, { from: "2026-08-24T10:00:00Z", to: "2026-08-24T10:10:00Z" });
  assert.deepEqual(model.lanes.map((lane) => lane.key), ["devices", "tasks", "media", "algorithms", "detections", "alerts"]);
  const media = model.lanes.find((lane) => lane.key === "media")!;
  assert.equal(media.items.length, 1);
  assert.equal(media.items[0].count, 2);
});

test("timeline marks empty lanes and long data gaps explicitly", () => {
  const model = buildTimelineModel(fixed, { from: "2026-08-24T10:00:00Z", to: "2026-08-24T10:10:00Z" });
  assert.equal(model.lanes.find((lane) => lane.key === "algorithms")?.gaps[0].reason, "no-data");
  assert.equal(model.lanes.find((lane) => lane.key === "devices")?.gaps[0].reason, "data-gap");
});
