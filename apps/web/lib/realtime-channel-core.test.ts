import assert from "node:assert/strict";
import test from "node:test";

import { buildRealtimeChannelView, type RealtimeChannel } from "./realtime-channel-core.ts";

const fixture = (overrides: Partial<RealtimeChannel>): RealtimeChannel => ({
  stableChannelId: "driver:adapter:device:channel", channelKey: "primary", displayName: "实时通道",
  dataType: "telemetry", schema: { type: "object" }, unit: null, quality: { qos: 1 },
  availability: "available", ...overrides
});

test("camera, aircraft telemetry and environment sensor select renderers from channel metadata", () => {
  const aircraftVideo = buildRealtimeChannelView(fixture({ stableChannelId: "dji:aircraft:video",
    dataType: "video", channelKey: "video.primary", displayName: "飞行器画面" }));
  const dockVideo = buildRealtimeChannelView(fixture({ stableChannelId: "dji:dock:video",
    dataType: "video", channelKey: "video.primary", displayName: "机场画面" }));
  assert.equal(aircraftVideo.renderer, "media");
  assert.equal(dockVideo.renderer, "media");
  assert.notEqual(aircraftVideo.id, dockVideo.id);
  const telemetry = buildRealtimeChannelView(fixture({ dataType: "telemetry", unit: "mixed",
    schema: { type: "object", properties: { latitude: { type: "number" }, height: { type: "number" } } } }));
  assert.equal(telemetry.renderer, "time-series");
  assert.deepEqual(telemetry.fields, ["latitude", "height"]);
  const sensor = buildRealtimeChannelView(fixture({ dataType: "sensor", channelKey: "sensor.primary",
    schema: { type: "object", properties: { samples: { type: "object" } } } }));
  assert.deepEqual(sensor.fields, ["samples"]);
});

test("an arbitrary robot channel needs no device-type branch", () => {
  const robot = buildRealtimeChannelView(fixture({
    stableChannelId: "ros2:robot-7:joints", channelKey: "joints", displayName: "关节状态",
    dataType: "telemetry", schema: { type: "object", properties: { joint_1: { type: "number" } } }
  }));
  assert.equal(robot.renderer, "time-series");
  assert.deepEqual(robot.fields, ["joint_1"]);
});

test("unavailable channels remain visible but cannot claim live data", () => {
  assert.equal(buildRealtimeChannelView(fixture({ availability: "unavailable" })).available, false);
  assert.throws(() => buildRealtimeChannelView(fixture({ stableChannelId: "" })), /ID_REQUIRED/);
});
