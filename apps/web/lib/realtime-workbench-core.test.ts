import assert from "node:assert/strict";
import test from "node:test";

import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";
import { activeProjectStreams, deviceHasLiveSignal, hasTransitionalLiveStream, isLiveStreamPlayable, liveStreamPollDecision, realtimeDeviceModules, relatedLiveDevices, resolveWorkbenchSelection, workbenchQuery } from "./realtime-workbench-core.ts";

test("selection updates retain static project scope and filters while replacing old selections", () => {
  const query = new URLSearchParams(workbenchQuery({deviceId: 12, streamId: 21}, 7, "projectId=7&deviceId=11&deviceId=99&streamId=5&layer=a&layer=b"));
  assert.equal(query.get("projectId"), "7");
  assert.deepEqual(query.getAll("deviceId"), ["12"]);
  assert.equal(query.get("streamId"), "21");
  assert.deepEqual(query.getAll("layer"), ["a", "b"]);
  const cleared = new URLSearchParams(workbenchQuery({deviceId:null,streamId:null}, 7, query.toString()));
  assert.equal(cleared.get("projectId"), "7");
  assert.equal(cleared.has("deviceId"), false);
  assert.equal(cleared.has("streamId"), false);
});

const snapshot = {
  project: { id: 7, name: "North", teamId: 3 }, generatedAt: "2026-08-28T00:00:00Z", consistency: "repeatable-read",
  devices: [{ id: 11, name: "Dock" }, { id: 12, name: "Drone" }], tracks: [], activeTasks: [], taskSteps: [], algorithmRuns: [],
  liveStreams: [{ id: 21, deviceId: 12, status: "starting" }, { id: 22, deviceId: 11, status: "stopped" }],
  mediaPoints: [], algorithmResults: [], openIssues: [], openAlerts: [], regions: [],
  freshness: { latestCapturedAt: null, isRealtime: true }, availability: {}
} satisfies ProjectSituationSnapshot;

test("automatic live connection requires a fresh signal from the selected online device", () => {
  const now = Date.parse("2026-09-29T03:00:00Z");
  const signal = { deviceId: 11, latestCapturedAt: new Date(now - 5000).toISOString(), latestPayload: { live: { available: true, active: true } } };
  const online = { ...snapshot, devices: [{ id: 11, status: "online" }, { id: 12, status: "online" }], realtimeChannels: [signal] };
  assert.equal(deviceHasLiveSignal(online, 11, now), true);
  assert.equal(deviceHasLiveSignal(online, 12, now), false);
  assert.equal(deviceHasLiveSignal({ ...online, devices: [{ id: 11, status: "offline" }] }, 11, now), false);
  assert.equal(deviceHasLiveSignal({ ...online, realtimeChannels: [{ ...signal, latestCapturedAt: new Date(now - 60000).toISOString() }] }, 11, now), false);
  assert.equal(deviceHasLiveSignal({ ...online, realtimeChannels: [{ ...signal, latestCapturedAt: new Date(now + 60000).toISOString() }] }, 11, now), false);
  assert.equal(deviceHasLiveSignal({ ...online, realtimeChannels: [{ ...signal, latestPayload: { live: { available: true, active: false } } }] }, 11, now), false);
  assert.equal(deviceHasLiveSignal({ ...online, realtimeChannels: [] }, 11, now), false);
});

test("workbench modules follow device capabilities, including temporarily unavailable video", () => {
  const capability = (code: string, availability = "available") => ({code, availability, reason: null, risk: "low" as const, authorized: true, actions: []});
  assert.deepEqual(realtimeDeviceModules(null), { live: false, timeline: false });
  assert.deepEqual(realtimeDeviceModules({capabilities: [capability("state.read")]}), {live: false, timeline: true});
  assert.deepEqual(realtimeDeviceModules({capabilities: [capability("stream.video.read", "unavailable")]}), {live: true, timeline: false});
  assert.deepEqual(realtimeDeviceModules({capabilities: [capability("stream.video.control"), capability("state.read")]}), {live: true, timeline: true});
  assert.deepEqual(realtimeDeviceModules({id: 11}), {live: false, timeline: false});
});

test("stream deep link owns device selection and foreign identifiers fail closed", () => {
  assert.deepEqual(resolveWorkbenchSelection(snapshot, { deviceId: 11, streamId: 21 }), { deviceId: 12, streamId: 21 });
  assert.deepEqual(resolveWorkbenchSelection(snapshot, { deviceId: 999, streamId: 888 }), { deviceId: null, streamId: null });
});

test("device deep link selects its active stream and keeps devices without streams", () => {
  assert.deepEqual(resolveWorkbenchSelection(snapshot, { deviceId: 12 }), { deviceId: 12, streamId: 21 });
  assert.deepEqual(resolveWorkbenchSelection(snapshot, { deviceId: 11 }), { deviceId: 11, streamId: null });
});

test("query and transition helpers expose only active project state", () => {
  assert.equal(workbenchQuery({ deviceId: 12, streamId: 21 }, 7), "projectId=7&deviceId=12&streamId=21");
  assert.equal(workbenchQuery({ deviceId: null, streamId: null }, 7), "projectId=7");
  assert.equal(new URLSearchParams(workbenchQuery({ deviceId: 12, streamId: 21 }, 7, "projectId=99")).get("projectId"), "7");
  assert.deepEqual(activeProjectStreams(snapshot).map((stream) => stream.id), [21]);
  assert.equal(hasTransitionalLiveStream(snapshot), true);
  assert.equal(isLiveStreamPlayable("starting"), false);
  assert.equal(isLiveStreamPlayable("live"), true);
  assert.equal(liveStreamPollDecision(snapshot, 14), "poll");
  assert.equal(liveStreamPollDecision(snapshot, 15), "timeout");
  assert.equal(liveStreamPollDecision({ ...snapshot, liveStreams: [] }, 0), "stable");
});

test("paired live devices follow explicit topology, including an offline aircraft", () => {
  const capability = {code: "stream.video.read", availability: "available", reason: null, risk: "low" as const, authorized: true, actions: []};
  const paired: ProjectSituationSnapshot = {...snapshot,
    devices: [{id: 11, capabilities: [capability]}, {id: 12, status: "offline", capabilities: [capability]}, {id: 13, capabilities: [capability]}],
    deviceRelations: [{fromDeviceId: 11, toDeviceId: 12, relationType: "contains"}, {fromDeviceId: 11, toDeviceId: 13, relationType: "nearby"}]
  };
  assert.deepEqual(relatedLiveDevices(paired, 11).map(device => device.id), [11, 12]);
  assert.deepEqual(relatedLiveDevices(paired, 12).map(device => device.id), [11, 12]);
  assert.deepEqual(relatedLiveDevices(paired, 99), []);
  assert.deepEqual(relatedLiveDevices({...paired, deviceRelations: []}, 11).map(device => device.id), [11]);
});
