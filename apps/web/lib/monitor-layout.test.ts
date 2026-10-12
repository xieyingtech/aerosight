import assert from "node:assert/strict";
import test from "node:test";
import { closestMonitorCorner, linkedFlightMonitor, mergeMonitorOrder, projectMonitorViews, swapMonitorViews } from "./monitor-layout.ts";
import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";

const snapshot = {
  devices: [{ id: 1, name: "设备一", channels: [{ dataType: "video", channelKey: "camera", displayName: "可见光" }, { dataType: "video", channelKey: "thermal", displayName: "热成像" }] }, { id: 2, name: "设备二", channels: [] }],
  liveStreams: [{ id: 11, deviceId: 1, streamKey: "camera", status: "live" }, { id: 12, deviceId: 1, streamKey: "thermal", status: "starting" }, { id: 13, deviceId: 2, streamKey: "camera", status: "live" }, { id: 14, deviceId: 99, streamKey: "camera", status: "live" }]
} as unknown as ProjectSituationSnapshot;

test("multiple devices and channels each own a tile, with a map and no foreign device", () => {
  assert.deepEqual(projectMonitorViews(snapshot).map(view => view.id), ["video:1:camera", "video:1:thermal", "video:2:camera", "map"]);
});
test("a replacement session keeps the same tile identity and requested channels remain after stopping", () => {
  const before = projectMonitorViews(snapshot);
  const next = { ...snapshot, liveStreams: [{ id: 99, deviceId: 1, streamKey: "camera", status: "live" }] };
  const after = projectMonitorViews(next, ["video:1:thermal"]);
  assert.equal(after.find(view => view.streamId === 99)?.id, before[0].id);
  assert.equal(after.find(view => view.id === "video:1:thermal")?.streamId, undefined);
  assert.equal(projectMonitorViews(next, ["video:99:camera"]).some(view => view.deviceId === 99), false);
});
test("saved layouts discard missing tiles and add new tiles; swaps preserve the source order", () => {
  const order = ["video:1:camera", "map", "missing"];
  const merged = mergeMonitorOrder(order, ["video:1:camera", "video:2:camera", "map"]);
  assert.deepEqual(merged, ["video:1:camera", "map", "video:2:camera"]);
  assert.deepEqual(swapMonitorViews(merged, "map", "video:1:camera"), ["map", "video:1:camera", "video:2:camera"]);
  assert.deepEqual(swapMonitorViews(merged, "unknown", "map"), merged);
  assert.deepEqual(order, ["video:1:camera", "map", "missing"]);
});
test("floating tiles snap to the nearest of the four corners", () => {
  assert.equal(closestMonitorCorner(10, 10, 1000, 600), "top-left");
  assert.equal(closestMonitorCorner(800, 20, 1000, 600), "top-right");
  assert.equal(closestMonitorCorner(200, 500, 1000, 600), "bottom-left");
  assert.equal(closestMonitorCorner(800, 500, 1000, 600), "bottom-right");
});

const flightSnapshot = {
  devices: [
    { id: 2, name: "机场", category: "dock", capabilities: [{ code: "stream.video.read" }], channels: [{ dataType: "video", channelKey: "dock", availability: "available" }] },
    { id: 1, name: "飞机", category: "aircraft", status: "offline", capabilities: [{ code: "stream.video.read" }], channels: [{ dataType: "video", channelKey: "176-0-0", availability: "unavailable" }, { dataType: "video", channelKey: "81-0-0", availability: "unavailable" }] },
    { id: 3, category: "aircraft", capabilities: [{ code: "stream.video.read" }], channels: [{ dataType: "video", channelKey: "other" }] },
  ],
  deviceRelations: [{ fromDeviceId: 2, toDeviceId: 1, relationType: "contains" }, { fromDeviceId: 2, toDeviceId: 3, relationType: "nearby" }, { fromDeviceId: 2, toDeviceId: 99, relationType: "contains" }],
  liveStreams: [{ id: 10, deviceId: 2, streamKey: "dock", status: "live" }],
} as unknown as ProjectSituationSnapshot;

test("flight monitoring adds offline aircraft main camera, dock and map using project topology", () => {
  const group = linkedFlightMonitor(flightSnapshot, 2)!;
  assert.deepEqual(group.deviceIds, [1, 2]);
  assert.deepEqual(group.preferredOrder, ["video:1:81-0-0", "video:2:dock", "map"]);
  assert.deepEqual(linkedFlightMonitor(flightSnapshot, 1), group);
  const views = projectMonitorViews(flightSnapshot, group.viewIds);
  assert.deepEqual(mergeMonitorOrder(group.preferredOrder, views.map(view => view.id)), group.preferredOrder);
  assert.equal(views.length, 3);
  assert.equal(linkedFlightMonitor(flightSnapshot, 99), null);
  assert.equal(linkedFlightMonitor(flightSnapshot, 3), null);
  assert.equal(linkedFlightMonitor({ ...flightSnapshot, deviceRelations: [] }, 2), null);
  const assistOnlyReady = structuredClone(flightSnapshot);
  assistOnlyReady.devices[1].channels![0].availability = "available";
  assert.equal(linkedFlightMonitor(assistOnlyReady, 2)!.viewIds[0], "video:1:81-0-0");
});

test("late aircraft channels fill the stable group without losing manual layout or existing sessions", () => {
  const waiting = { ...flightSnapshot, devices: flightSnapshot.devices.map(device => device.id === 1 ? { ...device, channels: [] } : device) };
  const before = linkedFlightMonitor(waiting, 2)!;
  const after = linkedFlightMonitor(flightSnapshot, 2)!;
  assert.equal(before.scope, after.scope);
  assert.deepEqual(before.viewIds, ["video:2:dock"]);
  assert.equal(after.preferredOrder[0], "video:1:81-0-0");
  const views = projectMonitorViews(flightSnapshot, after.viewIds);
  assert.equal(views.find(view => view.deviceId === 2)?.streamId, 10);
  const manual = swapMonitorViews(after.preferredOrder, "map", "video:1:81-0-0");
  assert.equal(mergeMonitorOrder(manual, views.map(view => view.id))[0], "map");
});
