import assert from "node:assert/strict";
import test from "node:test";

import { decideApproval } from "./approval-core.ts";
import { assertStreamCanStart, transitionLiveStream } from "./live-stream-core.ts";
import { evaluateMissionPreflight, type SafetyPolicy } from "./mission-preflight.ts";
import { planPerceptionEventAction } from "./perception-event-actions-core.ts";
import { createProjectMapModel } from "./project-map-model.ts";
import type { ProjectReplay } from "./project-replay-core.ts";
import { applyReplayToSnapshot } from "./replay-model.ts";
import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";
import { mapSuspectedConstructionDetections, suspectedConstructionTemplate } from "./suspected-construction-template.ts";
import { applyCommandAck, transitionTaskRun } from "./task-run-core.ts";
import { buildTimelineModel } from "./timeline-model.ts";

test("simulator vertical acceptance covers inspection through alert handling and replay", () => {
  const startedAt = "2026-08-27T08:00:00.000Z";
  const initialSnapshot: ProjectSituationSnapshot = {
    project: { id: 17, name: "北区 simulator 试点", teamId: 5 },
    generatedAt: "2026-08-27T08:10:00.000Z",
    consistency: "repeatable-read",
    devices: [{ id: 1, projectId: 17, name: "Simulator 无人机", type: "drone", status: "online", lastSeenAt: startedAt,
      pose: { longitude: 120.15, latitude: 30.27, altitudeMeters: 80, capturedAt: startedAt } }],
    tracks: [{ projectId: 17, deviceId: 1, startedAt, endedAt: "2026-08-27T08:05:00.000Z", pointCount: 2,
      geometry: { type: "LineString", coordinates: [[120.15, 30.27, 80], [120.16, 30.28, 85]] } }],
    activeTasks: [], liveStreams: [], mediaPoints: [], suspectedConstruction: [], openIssues: [], openAlerts: [], regions: [],
    freshness: { latestCapturedAt: "2026-08-27T08:05:00.000Z", isRealtime: true },
    availability: { devices: "available", tasks: "available", media: "available", alerts: "available", liveStreams: "available" }
  };
  const overview = createProjectMapModel(initialSnapshot);
  assert.ok(overview.features.some((feature) => feature.properties.layerKind === "device-drone"));
  assert.ok(overview.features.some((feature) => feature.properties.layerKind === "track"));

  const policy: SafetyPolicy = {
    policyVersionId: "sim-policy-v1",
    projectBoundary: [[120, 30], [121, 30], [121, 31], [120, 31], [120, 30]],
    restrictedAreas: [], maxAltitudeMeters: 120, maxSpeedMetersPerSecond: 15, minimumBatteryPercent: 30,
    requiredCompliance: ["flightApproval", "remoteIdentification"]
  };
  const preflight = evaluateMissionPreflight(policy, {
    route: [[120.15, 30.27, 80], [120.16, 30.28, 85]], plannedSpeedMetersPerSecond: 8,
    batteryPercent: 82, plannedStartAt: new Date(startedAt),
    compliance: { flightApproval: { reference: "SIM-APPROVAL" }, remoteIdentification: { reference: "SIM-RID" } }
  });
  assert.equal(preflight.allowed, true);
  const approval = decideApproval({
    id: "approval-1", requestedByUserId: 9, status: "pending", requiredApprovals: 1,
    requireSeparation: true, expiresAt: new Date("2026-08-27T09:00:00Z")
  }, [], { approverUserId: 10, decision: "approved", reason: "simulator preflight reviewed", decidedAt: new Date(startedAt) }, new Date(startedAt));
  assert.equal(approval.request.status, "approved");

  let run = transitionTaskRun({ status: "queued", stateVersion: 0 }, 0, "ready", "preflight passed");
  run = transitionTaskRun(run, 1, "dispatching", "approval complete");
  run = transitionTaskRun(run, 2, "running", "simulator command sent");
  const command = applyCommandAck([{ commandId: "sim-command-1", status: "sent" }], {
    commandId: "sim-command-1", outcome: "ack", result: { simulator: true }
  });
  assert.equal(command.entries[0].status, "acknowledged");
  run = transitionTaskRun(run, 3, "succeeded", "device ACK received");
  assert.equal(run.status, "succeeded");

  assert.doesNotThrow(() => assertStreamCanStart({ deviceStatus: "online", capabilities: ["camera.live"], adapterType: "simulator" }));
  assert.equal(transitionLiveStream("starting", "live"), "live");
  const detections = mapSuspectedConstructionDetections({
    response: { results: [{ id: "sim-detection-1", class: "new_building", score: 0.91,
      geometry: { type: "bbox", x: 10, y: 20, width: 100, height: 80 } }] },
    mapping: suspectedConstructionTemplate.outputMapping,
    labelMapping: suspectedConstructionTemplate.labelMapping,
    inputAsset: { assetId: 21, version: 1, checksumSha256: "a".repeat(64), mimeType: "image/jpeg" }
  });
  assert.equal(detections[0].label, "suspected-construction:new-building");
  const handled = planPerceptionEventAction({
    action: "confirm", currentStatus: "open", actualVersion: 0, expectedVersion: 0,
    permissions: new Set(["event:handle"]), actorUserId: 10
  });
  assert.equal(handled.status, "acknowledged");

  const completedSnapshot: ProjectSituationSnapshot = {
    ...initialSnapshot,
    activeTasks: [{ id: 42, projectId: 17, taskName: "疑似违建巡检", status: run.status, startedAt }],
    liveStreams: [{ id: 3, projectId: 17, deviceId: 1, status: "live", startedAt }],
    mediaPoints: [{ id: 21, projectId: 17, deviceId: 1, kind: "image", capturedAt: "2026-08-27T08:04:00.000Z",
      metadata: { longitude: 120.16, latitude: 30.28 } }],
    suspectedConstruction: [{ id: "group-1", projectId: 17, label: "疑似违建", status: "active",
      capturedAt: "2026-08-27T08:06:00.000Z", geometry: { type: "Polygon", coordinates: [[[120.16, 30.28], [120.17, 30.28], [120.17, 30.29], [120.16, 30.28]]] } }],
    openIssues: [], openAlerts: [{ id: "event-1", projectId: 17, title: "疑似违建", status: handled.status,
      updatedAt: "2026-08-27T08:07:00.000Z", longitude: 120.16, latitude: 30.28 }]
  };
  const timeline = buildTimelineModel(completedSnapshot, { from: startedAt, to: completedSnapshot.generatedAt });
  for (const lane of ["devices", "tasks", "media", "detections", "alerts"] as const) {
    assert.ok(timeline.lanes.find((item) => item.key === lane)?.items.length, `timeline lacks ${lane}`);
  }

  const replay: ProjectReplay = {
    projectId: 17, mode: "replay", window: { from: startedAt, to: "2026-08-27T08:05:00.000Z" },
    filters: { deviceTypes: ["drone"], bbox: null }, truncated: false,
    poses: [
      { deviceId: 1, deviceName: "Simulator 无人机", deviceType: "drone", capturedAt: startedAt, longitude: 120.15, latitude: 30.27, altitudeMeters: 80 },
      { deviceId: 1, deviceName: "Simulator 无人机", deviceType: "drone", capturedAt: "2026-08-27T08:05:00.000Z", longitude: 120.16, latitude: 30.28, altitudeMeters: 85 }
    ],
    media: completedSnapshot.mediaPoints,
    events: [{ eventType: "perception_event.acknowledged", occurredAt: "2026-08-27T08:07:00.000Z" }]
  };
  const historical = applyReplayToSnapshot(completedSnapshot, replay);
  assert.equal(historical.freshness.isRealtime, false);
  assert.equal(historical.liveStreams.length, 0);
  assert.ok(createProjectMapModel(historical).features.some((feature) => feature.properties.layerKind === "track"));
});
