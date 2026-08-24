import assert from "node:assert/strict";
import test from "node:test";
import {
  encodeServerSentEvent, encodeSnapshotRequired, parseEventCursor, replayDecision,
  shouldRecheckAuthorization, type StreamProjectEvent
} from "./project-stream-core.ts";

const event = (cursor: number): StreamProjectEvent => ({
  cursor: String(cursor), eventId: `event-${cursor}`, eventType: "device.pose.updated",
  payload: { deviceId: 4 }, occurredAt: "2026-08-24T10:00:00.000Z"
});

test("SSE preserves cursor and event identity for reconnect replay", () => {
  assert.equal(parseEventCursor("00042"), "42");
  assert.equal(parseEventCursor("invalid"), "0");
  const encoded = encodeServerSentEvent(event(42));
  assert.match(encoded, /^id: 42\nevent: device\.pose\.updated\ndata:/);
  assert.match(encoded, /"eventId":"event-42"/);
});

test("replay beyond the bounded window requires a consistent snapshot", () => {
  assert.equal(replayDecision([event(2), event(3)], 2).kind, "events");
  const decision = replayDecision([event(2), event(3), event(4)], 2);
  assert.deepEqual(decision, { kind: "snapshot-required", cursor: "4", events: [] });
  assert.match(encodeSnapshotRequired("4", 9), /\/api\/projects\/9\/snapshot/);
});

test("long-lived stream periodically rechecks authorization", () => {
  assert.equal(shouldRecheckAuthorization(1_000, 15_999, 15_000), false);
  assert.equal(shouldRecheckAuthorization(1_000, 16_000, 15_000), true);
});
