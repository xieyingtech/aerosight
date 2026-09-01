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
  tracks: [], activeTasks: [],
  taskSteps: [{ id: 10, name: "采集", status: "succeeded", occurredAt: "2026-08-24T10:02:00Z" }],
  algorithmRuns: [{ id: "algorithm-1", definitionName: "疑似违建", status: "succeeded", occurredAt: "2026-08-24T10:03:00Z" }],
  liveStreams: [],
  mediaPoints: [
    { id: 1, deviceId: 1, kind: "image", capturedAt: "2026-08-24T10:01:00Z" },
    { id: 2, deviceId: 1, kind: "image", capturedAt: "2026-08-24T10:01:01Z" }
  ],
  suspectedConstruction: [{ id: 7, label: "疑似违建", status: "active", capturedAt: "2026-08-24T10:04:00Z" }],
  openIssues: [{ id: 8, title: "现场复核", status: "open", updatedAt: "2026-08-24T10:05:00Z" }], openAlerts: [], regions: [],
  freshness: { latestCapturedAt: null, isRealtime: false }, availability: {}
} satisfies ProjectSituationSnapshot;

test("timeline has stable lane ordering and aggregates dense device media", () => {
  const model = buildTimelineModel(fixed, { from: "2026-08-24T10:00:00Z", to: "2026-08-24T10:10:00Z" });
  assert.deepEqual(model.lanes.map((lane) => lane.key), ["devices", "tasks", "media", "algorithms", "detections", "issues"]);
  for (const key of ["tasks", "algorithms", "detections", "issues"] as const) {
    assert.equal(model.lanes.find((lane) => lane.key === key)?.items.length, 1);
  }
  const media = model.lanes.find((lane) => lane.key === "media")!;
  assert.equal(media.items.length, 1);
  assert.equal(media.items[0].count, 2);
});

test("timeline marks empty lanes and long data gaps explicitly", () => {
  const model = buildTimelineModel(fixed, { from: "2026-08-24T10:00:00Z", to: "2026-08-24T10:10:00Z" });
  assert.equal(model.lanes.find((lane) => lane.key === "algorithms")?.gaps.length, 0);
  assert.equal(model.lanes.find((lane) => lane.key === "devices")?.gaps[0].reason, "data-gap");
});
