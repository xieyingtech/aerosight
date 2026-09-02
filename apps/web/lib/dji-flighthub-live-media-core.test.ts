import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { flightHubPlaybackGate, safeLiveMediaSummary } from "./dji-flighthub-live-media-core.ts";

test("FlightHub playback authorization is short lived and fails after expiry", () => {
  const now = new Date("2026-09-02T04:00:00Z");
  assert.deepEqual(flightHubPlaybackGate({ canView: true, status: "live", localAuthorizationRevokedAt: null,
    credentialExpiresAt: new Date(now.getTime() + 60_000), now }), { available: true });
  assert.deepEqual(flightHubPlaybackGate({ canView: true, status: "live", localAuthorizationRevokedAt: null,
    credentialExpiresAt: now, now }), { available: false, reason: "playback-credential-expired" });
});

test("permission or local authorization revocation denies the next playback request", () => {
  const base = { status: "live", localAuthorizationRevokedAt: null, credentialExpiresAt: new Date("2026-09-02T05:00:00Z"),
    now: new Date("2026-09-02T04:00:00Z") };
  assert.throws(() => flightHubPlaybackGate({ ...base, canView: false }), /PERMISSION_REVOKED/);
  assert.deepEqual(flightHubPlaybackGate({ ...base, canView: true, localAuthorizationRevokedAt: base.now }),
    { available: false, reason: "playback-authorization-revoked" });
});

test("media catalog summary strips supplier destinations and credentials", () => {
  assert.deepEqual(safeLiveMediaSummary({ name: "relay", state: "running", url: "https://secret.invalid",
    password: "hidden", serverIp: "10.0.0.1" }), { name: "relay", state: "running" });
});

test("realtime media API and page use project authorization without secret columns", () => {
  const service = readFileSync(new URL("./dji-flighthub-live-media.ts", import.meta.url), "utf8");
  const route = readFileSync(new URL("../app/api/projects/[id]/connectors/dji-flighthub/live-media/route.ts", import.meta.url), "utf8");
  const page = readFileSync(new URL("../app/(app)/projects/[id]/realtime/page.tsx", import.meta.url), "utf8");
  assert.match(service, /requireCurrentProjectPermission\(projectId, "project:view"\)/);
  assert.doesNotMatch(service, /credential_envelope|bypass_option|remote_id/);
  assert.match(route, /private, no-store/);
  assert.match(page, /DJIFlightHubLiveMediaPanel/);
});
