import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { parseVolcRTCPlaybackCredential } from "./volc-rtc-player-core.ts";

test("Volc RTC credential parser accepts DJI query and JSON envelopes without widening fields", () => {
  assert.deepEqual(parseVolcRTCPlaybackCredential("app_id=app&room_id=room&token=secret&user_id=viewer&expire_time=1"),
    { appId: "app", roomId: "room", token: "secret", userId: "viewer" });
  assert.deepEqual(parseVolcRTCPlaybackCredential(JSON.stringify({ app_id: "app", room_id: "room", token: "secret", user_id: "viewer" })),
    { appId: "app", roomId: "room", token: "secret", userId: "viewer" });
});

test("Volc RTC credential parser rejects missing, oversized, and structured secret values", () => {
  for (const value of ["app_id=app&room_id=room&token=secret", " app_id=app", JSON.stringify({ app_id: "app", room_id: "room", token: {}, user_id: "viewer" })]) {
    assert.throws(() => parseVolcRTCPlaybackCredential(value), /VOLC_RTC_CREDENTIAL_INVALID/);
  }
});

test("Volc RTC viewer becomes invisible before joining with default network negotiation", () => {
  const source = readFileSync(new URL("../components/volc-rtc-player.tsx", import.meta.url), "utf8");
  assert.ok(source.indexOf("await engine.setUserVisibility(false)") < source.indexOf("engine.joinRoom("));
  assert.doesNotMatch(source, /JOIN_ROOM_CONFIG|joinWithTcpOnly/);
});
