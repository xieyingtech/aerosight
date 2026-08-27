import assert from "node:assert/strict";
import test from "node:test";

import { checkDeviceNetworkConnection } from "./device-connection-check-core.ts";
import type { DeviceNetworkProfileInput } from "./device-network-profile.ts";
import type { HostResolver } from "./outbound-url-policy.ts";

const profile: DeviceNetworkProfileInput = {
  mode: "public",
  mqttEndpoint: "mqtts://mqtt.example.test:8883/tenant?token=must-not-leak",
  apiPublicBaseUrl: "https://api.example.test/dji/callback?signature=must-not-leak",
  websocketPublicUrl: "wss://api.example.test/events?ticket=must-not-leak",
  mediaIngestBaseUrl: "rtmps://publish:password@ingest.example.test:1936/private-key",
  mediaPlaybackBaseUrl: "https://media.example.test/private-playback",
  tlsRequired: true,
  mqttAnonymous: false,
  secretRef: "vault://projects/1/mqtt-password"
};

const publicResolver: HostResolver = async () => [{ address: "203.0.113.20", family: 4 }];

test("healthy endpoints are server-verified while device reachability remains pending", async () => {
  const result = await checkDeviceNetworkConnection({
    ...profile,
    mediaIngestBaseUrl: "rtmps://ingest.example.test:1936/live"
  }, {
    resolver: publicResolver,
    probe: async () => undefined,
    now: () => new Date("2026-08-27T09:00:00.000Z")
  });
  assert.equal(result.ok, true);
  assert.equal(result.serverVerification, "verified");
  assert.equal(result.deviceVerification, "pending");
  assert.equal(result.checkedAt, "2026-08-27T09:00:00.000Z");
  assert(result.diagnostics.every((diagnostic) => diagnostic.status === "server_verified"));
  assert.equal(result.diagnostics.find(({ field }) => field === "mediaPlaybackBaseUrl")?.deviceVerification, "not_applicable");
});

test("failed fixtures identify the endpoint without returning transport error details", async () => {
  const result = await checkDeviceNetworkConnection({
    ...profile,
    mediaIngestBaseUrl: "rtmps://ingest.example.test:1936/live"
  }, {
    resolver: publicResolver,
    probe: async (endpoint) => {
      if (endpoint.field === "mqttEndpoint") throw new Error("password=super-secret ECONNREFUSED");
    }
  });
  assert.equal(result.ok, false);
  assert.equal(result.serverVerification, "failed");
  assert.deepEqual(result.diagnostics.find(({ field }) => field === "mqttEndpoint"), {
    field: "mqttEndpoint",
    endpoint: "mqtts://mqtt.example.test:8883",
    status: "failed",
    code: "ENDPOINT_PROBE_FAILED",
    deviceVerification: "pending"
  });
  assert.doesNotMatch(JSON.stringify(result), /super-secret|ECONNREFUSED/);
});

test("policy failures are not probed and all diagnostics are redacted", async () => {
  let probes = 0;
  const result = await checkDeviceNetworkConnection(profile, {
    resolver: publicResolver,
    probe: async () => { probes += 1; }
  });
  assert.equal(probes, 4);
  const serialized = JSON.stringify(result);
  assert.doesNotMatch(serialized, /must-not-leak|password|private-key|private-playback|signature|ticket|token/);
  assert(result.diagnostics.some(({ field, code }) => field === "mediaIngestBaseUrl" && code === "ENDPOINT_INLINE_CREDENTIALS_FORBIDDEN"));
});
