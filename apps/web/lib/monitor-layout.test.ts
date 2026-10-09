import assert from "node:assert/strict";
import test from "node:test";
import { closestMonitorCorner, mergeMonitorOrder, projectMonitorViews, swapMonitorViews } from "./monitor-layout.ts";
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
