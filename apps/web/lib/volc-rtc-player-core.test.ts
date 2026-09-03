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

test("Volc RTC remount waits for the previous fixed viewer identity to leave", () => {
  const source = readFileSync(new URL("../components/volc-rtc-player.tsx", import.meta.url), "utf8");
  assert.match(source, /await volcRTCCleanupTail;[\s\S]*createEngine/);
  assert.match(source, /enqueueVolcRTCCleanup\(release\)/);
});

test("Volc RTC timeout is presented as a local viewer failure with a local-only retry", () => {
  const source = readFileSync(new URL("../components/volc-rtc-player.tsx", import.meta.url), "utf8");
  assert.match(source, /直播已启动，但当前浏览器未建立 RTC 观看连接/);
  assert.match(source, /重试观看/);
  assert.match(source, /setViewerAttempt\(\(current\) => current \+ 1\)/);
  assert.match(source, /\[credential, viewerAttempt\]/);
  assert.doesNotMatch(source, /fetch\(|live-stream\/start/);
});
