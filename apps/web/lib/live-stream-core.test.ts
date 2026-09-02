import assert from "node:assert/strict";
import test from "node:test";

import {
  assertStreamCanStart,
  assertLiveStreamConcurrency,
  assertLiveStreamProjectScope,
  buildDJIVideoID,
  buildRTMPIngestURL,
  buildRTMPIngestEndpoint,
  buildPlaybackCandidates,
  createLiveStreamIngestRef,
  normalizeAdapterLiveStatus,
  MediaPlaybackTokenIssuer,
  nextPlaybackCandidate,
  playbackAvailability,
  planLiveStreamStop,
  SimulatorPlaybackLocator,
  recoverExpiredLiveStream,
  transitionLiveStream,
  type LiveStreamSession
} from "./live-stream-core.ts";

const session: LiveStreamSession = {
  id: 9, projectId: 17, deviceId: 42, streamKey: "camera.main", sourceType: "simulator",
  status: "live", playbackRef: "simulator://drone/camera.main", lastActiveAt: null, statusReason: null
};

test("live stream state machine rejects invalid terminal transitions", () => {
  assert.equal(transitionLiveStream("requested", "starting"), "starting");
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
  assert.doesNotThrow(() => assertStreamCanStart({
    deviceStatus: "online", capabilities: [], adapterType: "dji-flighthub2", connectorLiveControl: true
  }));
  assert.throws(() => assertStreamCanStart({
    deviceStatus: "online", capabilities: [], adapterType: "dji-flighthub2", connectorLiveControl: false
  }), /NOT_SUPPORTED/);
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

test("stream concurrency rejects conflicts at the driver declared limit", () => {
  assert.equal(assertLiveStreamConcurrency({ activeCount: 0, maxConcurrentSessions: 1 }), true);
  assert.throws(() => assertLiveStreamConcurrency({ activeCount: 1, maxConcurrentSessions: 1 }), /CONCURRENCY_CONFLICT/);
  assert.throws(() => assertLiveStreamConcurrency({ activeCount: 0, maxConcurrentSessions: 0 }), /CONCURRENCY_INVALID/);
});

test("each session receives a random opaque ingest reference", () => {
  const first = createLiveStreamIngestRef();
  const second = createLiveStreamIngestRef();
  assert.match(first, /^demo\/aerosight\/[A-Za-z0-9_-]{32}$/);
  assert.notEqual(first, second);
});

test("DJI RTMP destination and video id are derived from server-owned topology", () => {
  const destination = buildRTMPIngestURL({
    baseURL: "rtmp://media.lan:1935", ingestRef: "demo/aerosight/opaque",
    username: "publisher", password: "secret"
  });
  assert.equal(destination, "rtmp://media.lan:1935/demo/aerosight/opaque?user=publisher&pass=secret");
  assert.equal(buildRTMPIngestEndpoint("rtmp://media.lan:1935", "demo/aerosight/opaque"),
    "rtmp://media.lan:1935/demo/aerosight/opaque");
  assert.equal(buildDJIVideoID({ sourceExternalId: "AIRCRAFT-SN", cameraType: 98, cameraSubtype: 0 }),
    "AIRCRAFT-SN/98-0-0/normal-0");
  assert.throws(() => buildRTMPIngestURL({ baseURL: "https://media.lan", ingestRef: "x", username: "u", password: "p" }), /RTMP_URL_REQUIRED/);
});

test("DJI stop enters stopping and repeated stop is idempotent", () => {
  assert.deepEqual(planLiveStreamStop("live", "dji"), { status: "stopping", replayed: false });
  assert.deepEqual(planLiveStreamStop("live", "dji_flighthub"), { status: "stopping", replayed: false });
  assert.deepEqual(planLiveStreamStop("failed", "dji_flighthub"), { status: "failed", replayed: true });
  assert.deepEqual(planLiveStreamStop("stopping", "dji"), { status: "stopping", replayed: true });
  assert.deepEqual(planLiveStreamStop("live", "simulator"), { status: "stopped", replayed: false });
  assert.deepEqual(planLiveStreamStop("stopped", "simulator"), { status: "stopped", replayed: true });
});

test("expired session leases recover every non-terminal state deterministically", () => {
  assert.deepEqual(recoverExpiredLiveStream("requested"), { status: "failed", reason: "session-start-lease-expired" });
  assert.deepEqual(recoverExpiredLiveStream("starting"), { status: "failed", reason: "session-start-lease-expired" });
  assert.deepEqual(recoverExpiredLiveStream("stopping"), { status: "stopped", reason: "session-stop-lease-expired" });
  assert.deepEqual(recoverExpiredLiveStream("live"), { status: "failed", reason: "session-owner-lease-expired" });
  assert.deepEqual(recoverExpiredLiveStream("stopped"), { status: "stopped", reason: null });
  assert.deepEqual(recoverExpiredLiveStream("stopping", "dji_flighthub"), {
    status: "stopping", reason: "flighthub-remote-evidence-required"
  });
  assert.deepEqual(recoverExpiredLiveStream("stopped", "dji_flighthub"), { status: "stopped", reason: null });
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

test("failed source is unavailable while an expired locator can be refreshed", () => {
  const now = new Date("2026-08-27T00:01:00.000Z");
  assert.deepEqual(playbackAvailability({ ...session, status: "failed" }, now, null), {
    available: false, reason: "stream-failed"
  });
  assert.deepEqual(playbackAvailability(session, now, new Date("2026-08-27T00:00:30.000Z")), {
    available: true, reason: null
  });
});

test("playback scope does not disclose another project stream", () => {
  assert.equal(assertLiveStreamProjectScope(session, 17), session);
  assert.throws(() => assertLiveStreamProjectScope(session, 18), /LIVE_STREAM_NOT_FOUND/);
});

test("short-lived playback token is path/protocol scoped and expires", () => {
  let now = new Date("2026-08-27T00:00:00.000Z");
  const issuer = new MediaPlaybackTokenIssuer("0123456789abcdef", () => now);
  const issued = issuer.issue({ projectId: 17, streamId: 9, path: "demo/aerosight/session",
    protocols: ["webrtc", "hls"], ttlSeconds: 30 });
  assert.equal(issuer.verify(issued.token, { path: "demo/aerosight/session", protocol: "hls" })?.projectId, 17);
  assert.equal(issuer.verify(issued.token, { path: "demo/aerosight/other", protocol: "hls" }), null);
  assert.equal(issuer.verify(`${issued.token}tampered`, { path: "demo/aerosight/session", protocol: "hls" }), null);
  now = new Date("2026-08-27T00:00:31.000Z");
  assert.equal(issuer.verify(issued.token, { path: "demo/aerosight/session", protocol: "hls" }), null);
});

test("browser playback prefers WebRTC and falls back to HLS", () => {
  const candidates = buildPlaybackCandidates({ path: "demo/aerosight/session", token: "signed",
    webrtcBaseURL: "https://media.example/webrtc", hlsBaseURL: "https://media.example/hls" });
  assert.equal(nextPlaybackCandidate(candidates, [])?.protocol, "webrtc");
  assert.equal(nextPlaybackCandidate(candidates, ["webrtc"])?.protocol, "hls");
  assert.equal(nextPlaybackCandidate(candidates, ["webrtc", "hls"]), null);
});
