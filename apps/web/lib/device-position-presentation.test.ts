import assert from "node:assert/strict";
import test from "node:test";

import { presentDevicePosition } from "./device-position-presentation.ts";

test("device position presentation distinguishes available, missing, and stale data", () => {
  assert.equal(presentDevicePosition({
    dataFreshness: "fresh", positionStatus: "available", positionSource: "fixture.driver",
    pose: { longitude: 120, latitude: 30, capturedAt: "2026-09-01T10:00:00Z", calibrationStatus: "calibrated" },
  }).state, "available");
  assert.deepEqual(presentDevicePosition({ dataFreshness: "unknown", positionStatus: "missing", pose: null }), {
    state: "missing", label: "暂无位置", reason: "尚未收到有效坐标", source: "unknown", capturedAt: null, coordinate: null,
  });
  assert.equal(presentDevicePosition({
    dataFreshness: "unknown", positionStatus: "missing", pose: { longitude: null, latitude: null },
  }).coordinate, null);
  const stale = presentDevicePosition({
    dataFreshness: "expired", positionStatus: "available", positionSource: "dji-flighthub-openapi",
    pose: { longitude: 120, latitude: 30, capturedAt: "2026-09-01T09:00:00Z", calibrationStatus: "calibrated" },
  });
  assert.equal(stale.state, "stale");
  assert.equal(stale.capturedAt, "2026-09-01T09:00:00Z");
});

test("unverified and invalid positions remain explicit and never look calibrated", () => {
  assert.equal(presentDevicePosition({
    dataFreshness: "fresh", positionStatus: "unverified",
    pose: { longitude: 120, latitude: 30, calibrationStatus: "unverified" },
  }).state, "unverified");
  assert.equal(presentDevicePosition({ dataFreshness: "fresh", positionStatus: "invalid", positionReason: "coordinate_zero_sentinel" }).state, "invalid");
});
