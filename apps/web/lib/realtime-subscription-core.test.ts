import assert from "node:assert/strict";
import test from "node:test";

import {
  authorizeRealtimeSubscription, encodeRealtimeAccessRevoked, encodeRealtimeSample, parseRealtimeCursor,
  realtimeFlowDecision,
  type RealtimeSubscriptionGrant, type RealtimeSubscriptionTarget
} from "./realtime-subscription-core.ts";

const target: RealtimeSubscriptionTarget = {
  requestProjectId: 10, channelProjectId: 10, deviceId: 42, deviceTypeId: "7",
  capabilityCode: "stream.sensor.read", availability: "available"
};

const grant = (overrides: Partial<RealtimeSubscriptionGrant> = {}): RealtimeSubscriptionGrant => ({
  scopeType: "device", deviceTypeId: null, deviceId: 42,
  actionPattern: "stream.sensor.read", effect: "allow", ...overrides
});

test("subscription fails closed across projects and for unrelated device grants", () => {
  assert.throws(() => authorizeRealtimeSubscription({ role: "admin", target: { ...target, channelProjectId: 11 }, grants: [] }), /SCOPE_DENIED/);
  assert.throws(() => authorizeRealtimeSubscription({ role: "member", target, grants: [grant({ deviceId: 41 })] }), /NOT_GRANTED/);
  assert.equal(authorizeRealtimeSubscription({ role: "member", target, grants: [grant()] }), true);
});

test("device deny revokes an existing broader stream grant", () => {
  const grants = [
    grant({ scopeType: "project", deviceId: null, actionPattern: "stream.*" }),
    grant({ effect: "deny" })
  ];
  assert.throws(() => authorizeRealtimeSubscription({ role: "member", target, grants }), /EXPLICITLY_DENIED/);
  assert.match(encodeRealtimeAccessRevoked("sensor:42"), /capability_revoked/);
});

test("cursor replay and samples preserve stable channel identity", () => {
  assert.equal(parseRealtimeCursor("00019"), "19");
  assert.equal(parseRealtimeCursor("not-a-cursor"), "0");
  const event = encodeRealtimeSample("sensor:42", {
    cursor: "19", eventId: "sample-19", capturedAt: "2026-08-27T01:00:00.000Z",
    payload: { temperature: 23 }, quality: { source: "fixture" }
  });
  assert.match(event, /^id: 19\nevent: channel\.sample/);
  assert.match(event, /"channelId":"sensor:42"/);
});

test("only stream capabilities and usable channels can be subscribed", () => {
  assert.throws(() => authorizeRealtimeSubscription({ role: "admin", target: { ...target, capabilityCode: "state.read" }, grants: [] }), /ACTION_INVALID/);
  assert.throws(() => authorizeRealtimeSubscription({ role: "admin", target: { ...target, availability: "unavailable" }, grants: [] }), /CHANNEL_UNAVAILABLE/);
});

test("slow subscribers pause without advancing the replay cursor", () => {
  assert.equal(realtimeFlowDecision(1, 20), "deliver");
  assert.equal(realtimeFlowDecision(0, 20), "pause");
  assert.equal(realtimeFlowDecision(-1, 501), "terminate");
});
