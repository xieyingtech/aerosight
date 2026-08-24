import assert from "node:assert/strict";
import test from "node:test";
import { initialSituationState, interpolateTimeline, situationReducer } from "./situation-state.ts";

test("map or timeline selection synchronizes entity detail and media time", () => {
  const next = situationReducer(initialSituationState, {
    type: "select", selection: { lane: "media", entityId: "8", label: "Frame 8", timestamp: "2026-08-24T10:05:00Z" }
  });
  assert.equal(next.selection?.entityId, "8");
  assert.equal(next.cursor, "2026-08-24T10:05:00Z");
});

test("drag cursor and reversed range enter ordered history mode", () => {
  const cursor = interpolateTimeline("2026-08-24T10:00:00Z", "2026-08-24T10:10:00Z", 500);
  let next = situationReducer(initialSituationState, { type: "set-cursor", cursor });
  assert.equal(next.mode, "history");
  assert.equal(cursor, "2026-08-24T10:05:00.000Z");
  next = situationReducer(next, { type: "set-range", from: "2026-08-24T10:08:00Z", to: "2026-08-24T10:02:00Z" });
  assert.deepEqual(next.range, { from: "2026-08-24T10:02:00Z", to: "2026-08-24T10:08:00Z" });
});

test("return live clears replay range, cursor, and stale selection", () => {
  const historical = situationReducer(initialSituationState, { type: "set-cursor", cursor: "2026-08-24T10:05:00Z" });
  assert.deepEqual(situationReducer(historical, { type: "return-live" }), initialSituationState);
});
