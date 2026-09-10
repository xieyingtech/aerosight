import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { flightHubPlaybackGate, safeLiveMediaSummary } from "./dji-flighthub-live-media-core.ts";

test("FlightHub playback authorization is short lived and fails after expiry", () => {
  const now = new Date("2026-09-02T04:00:00Z");
  assert.deepEqual(flightHubPlaybackGate({ canView: true, status: "live", startAcceptedAt: now, localAuthorizationRevokedAt: null,
    credentialExpiresAt: new Date(now.getTime() + 60_000), now }), { available: true });
  assert.deepEqual(flightHubPlaybackGate({ canView: true, status: "live", startAcceptedAt: now, localAuthorizationRevokedAt: null,
    credentialExpiresAt: now, now }), { available: false, reason: "playback-credential-expired" });
  assert.deepEqual(flightHubPlaybackGate({ canView: true, status: "starting", startAcceptedAt: now, localAuthorizationRevokedAt: null,
    credentialExpiresAt: new Date(now.getTime() + 60_000), now }), { available: true });
  assert.deepEqual(flightHubPlaybackGate({ canView: true, status: "starting", startAcceptedAt: null, localAuthorizationRevokedAt: null,
    credentialExpiresAt: new Date(now.getTime() + 60_000), now }), { available: false, reason: "stream-starting-unaccepted" });
});

test("permission or local authorization revocation denies the next playback request", () => {
  const base = { status: "live", startAcceptedAt: new Date("2026-09-02T04:00:00Z"), localAuthorizationRevokedAt: null, credentialExpiresAt: new Date("2026-09-02T05:00:00Z"),
    now: new Date("2026-09-02T04:00:00Z") };
  assert.throws(() => flightHubPlaybackGate({ ...base, canView: false }), /PERMISSION_REVOKED/);
  assert.deepEqual(flightHubPlaybackGate({ ...base, canView: true, localAuthorizationRevokedAt: base.now }),
    { available: false, reason: "playback-authorization-revoked" });
});

test("media catalog summary strips supplier destinations and credentials", () => {
  assert.deepEqual(safeLiveMediaSummary({ name: "relay", state: "running", url: "https://secret.invalid",
    password: "hidden", serverIp: "10.0.0.1" }), { name: "relay", state: "running" });
});

test("realtime media API remains scoped while the generic realtime page stays connector-neutral", async () => {
const source=[await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/httpapi/flighthub_live_media.go", import.meta.url), "utf8"),await (await import("node:fs/promises")).readFile(new URL("../../../apps/web/app/(app)/projects/realtime/page.tsx", import.meta.url), "utf8")].join("\n");
assert.match(source, /scopedRead/);
assert.match(source, /RealtimeOperationsWorkbench/);
assert.doesNotMatch(source, /credential_envelope/);
assert.doesNotMatch(source, /bypass_option/);
assert.doesNotMatch(source, /remote_id/);
});
