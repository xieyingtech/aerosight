import assert from "node:assert/strict";
import test from "node:test";

import {
  assertStreamCanStart,
  assertLiveStreamProjectScope,
  normalizeAdapterLiveStatus,
  playbackAvailability,
  SimulatorPlaybackLocator,
  transitionLiveStream,
  type LiveStreamSession
} from "./live-stream-core.ts";

const session: LiveStreamSession = {
  id: 9, projectId: 17, deviceId: 42, streamKey: "camera.main", sourceType: "simulator",
  status: "live", playbackRef: "simulator://drone/camera.main", lastActiveAt: null, statusReason: null
};

test("live stream state machine rejects invalid terminal transitions", () => {
  assert.equal(transitionLiveStream("starting", "live"), "live");
  assert.equal(transitionLiveStream("live", "degraded"), "degraded");
  assert.equal(transitionLiveStream("stopping", "stopped"), "stopped");
  assert.throws(() => transitionLiveStream("stopped", "live"), /INVALID_LIVE_STREAM_TRANSITION/);
});

test("adapter stream status maps to canonical state or fails explicitly", () => {
  assert.equal(normalizeAdapterLiveStatus("started"), "live");
  assert.equal(normalizeAdapterLiveStatus("interrupted"), "degraded");
  assert.equal(normalizeAdapterLiveStatus("stopped"), "stopped");
  assert.throws(() => normalizeAdapterLiveStatus("buffering-forever"), /UNKNOWN_ADAPTER_LIVE_STATUS/);
});

test("stream start requires online capable device and implemented adapter", () => {
  assert.doesNotThrow(() => assertStreamCanStart({
    deviceStatus: "online", capabilities: ["camera.live"], adapterType: "simulator"
  }));
  assert.throws(() => assertStreamCanStart({
    deviceStatus: "offline", capabilities: ["camera.live"], adapterType: "simulator"
  }), /DEVICE_OFFLINE/);
  assert.throws(() => assertStreamCanStart({
    deviceStatus: "online", capabilities: [], adapterType: "simulator"
  }), /NOT_SUPPORTED/);
  assert.throws(() => assertStreamCanStart({
    deviceStatus: "online", capabilities: ["camera.live"], adapterType: "ros2"
  }), /ADAPTER_UNAVAILABLE/);
});

test("simulator locator is short lived and tamper evident", () => {
  let now = new Date("2026-08-27T00:00:00.000Z");
  const issuer = new SimulatorPlaybackLocator("0123456789abcdef", () => now);
  const locator = issuer.issue({ projectId: 17, streamId: 9, playbackRef: session.playbackRef!, ttlSeconds: 30 });
  const parsed = new URL(locator.url, "https://aerosight.test");
  const input = {
    projectId: 17,
    streamId: 9,
    playbackRef: session.playbackRef!,
    expires: parsed.searchParams.get("expires")!,
    signature: parsed.searchParams.get("signature")!
  };
  assert.equal(issuer.verify(input), true);
  assert.equal(issuer.verify({ ...input, projectId: 18 }), false);
  now = new Date("2026-08-27T00:01:00.000Z");
  assert.equal(issuer.verify(input), false);
});

test("failed source and expired locator expose explicit unavailability", () => {
  const now = new Date("2026-08-27T00:01:00.000Z");
  assert.deepEqual(playbackAvailability({ ...session, status: "failed" }, now, null), {
    available: false, reason: "stream-failed"
  });
  assert.deepEqual(playbackAvailability(session, now, new Date("2026-08-27T00:00:30.000Z")), {
    available: false, reason: "locator-expired"
  });
});

test("playback scope does not disclose another project stream", () => {
  assert.equal(assertLiveStreamProjectScope(session, 17), session);
  assert.throws(() => assertLiveStreamProjectScope(session, 18), /LIVE_STREAM_NOT_FOUND/);
});
