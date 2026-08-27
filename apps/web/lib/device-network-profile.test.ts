import assert from "node:assert/strict";
import test from "node:test";

import {
  isUnroutableDeviceAddress,
  validateDeviceNetworkProfile,
  type DeviceNetworkProfileInput
} from "./device-network-profile.ts";
import type { HostResolver } from "./outbound-url-policy.ts";

const lanProfile: DeviceNetworkProfileInput = {
  mode: "lan",
  mqttEndpoint: "mqtt://192.168.20.10:1883",
  apiPublicBaseUrl: "http://192.168.20.10:3000",
  websocketPublicUrl: "ws://192.168.20.10:3000/device-events",
  mediaIngestBaseUrl: "rtmp://192.168.20.10:1935",
  mediaPlaybackBaseUrl: "http://192.168.20.10:8888",
  tlsRequired: false,
  mqttAnonymous: false,
  secretRef: "vault://aerosight/demo-mqtt"
};

const publicProfile: DeviceNetworkProfileInput = {
  mode: "public",
  mqttEndpoint: "mqtts://mqtt.example.test:8883",
  apiPublicBaseUrl: "https://api.example.test",
  websocketPublicUrl: "wss://api.example.test/device-events",
  mediaIngestBaseUrl: "rtmps://ingest.example.test:1936",
  mediaPlaybackBaseUrl: "https://media.example.test",
  tlsRequired: true,
  mqttAnonymous: false,
  secretRef: "vault://aerosight/production-mqtt"
};

const privateResolver: HostResolver = async () => [{ address: "192.168.20.10", family: 4 }];
const publicResolver: HostResolver = async () => [{ address: "203.0.113.10", family: 4 }];

test("LAN profiles accept RFC1918 endpoints for every device-facing protocol", async () => {
  const result = await validateDeviceNetworkProfile(lanProfile, { resolver: privateResolver });
  assert.equal(result.valid, true);
  assert.equal(Object.keys(result.endpoints).length, 5);
});

test("loopback and localhost endpoints are rejected in LAN and public profiles", async () => {
  for (const endpoint of ["mqtt://127.0.0.1:1883", "mqtt://localhost:1883"]) {
    const result = await validateDeviceNetworkProfile({ ...lanProfile, mqttEndpoint: endpoint }, { resolver: privateResolver });
    assert.equal(result.valid, false);
    assert(result.issues.some(({ field, code }) => field === "mqttEndpoint" && /LOOPBACK|UNROUTABLE/.test(code)));
  }
  assert.equal(isUnroutableDeviceAddress("::1"), true);
  assert.equal(isUnroutableDeviceAddress("169.254.10.2"), true);
  assert.equal(isUnroutableDeviceAddress("192.168.20.10"), false);
});

test("public profiles reject private DNS answers on all endpoint kinds", async () => {
  const result = await validateDeviceNetworkProfile(publicProfile, { resolver: privateResolver });
  assert.equal(result.valid, false);
  for (const field of ["mqttEndpoint", "apiPublicBaseUrl", "websocketPublicUrl", "mediaIngestBaseUrl", "mediaPlaybackBaseUrl"]) {
    assert(result.issues.some((issue) => issue.field === field && issue.code === "PUBLIC_ADDRESS_REQUIRED"), field);
  }
});

test("public profiles require encrypted schemes and the TLS policy flag", async () => {
  const result = await validateDeviceNetworkProfile({
    ...publicProfile,
    mqttEndpoint: "mqtt://mqtt.example.test:1883",
    apiPublicBaseUrl: "http://api.example.test",
    websocketPublicUrl: "ws://api.example.test/device-events",
    mediaIngestBaseUrl: "rtmp://ingest.example.test:1935",
    mediaPlaybackBaseUrl: "http://media.example.test",
    tlsRequired: false
  }, { resolver: publicResolver });
  assert.equal(result.valid, false);
  assert(result.issues.some((issue) => issue.field === "tlsRequired" && issue.code === "PUBLIC_TLS_REQUIRED"));
  for (const field of ["mqttEndpoint", "apiPublicBaseUrl", "websocketPublicUrl", "mediaIngestBaseUrl", "mediaPlaybackBaseUrl"]) {
    assert(result.issues.some((issue) => issue.field === field && issue.code === "PUBLIC_TLS_SCHEME_REQUIRED"), field);
  }
});

test("public MQTT cannot be anonymous and must use a secret reference", async () => {
  const result = await validateDeviceNetworkProfile({
    ...publicProfile,
    mqttAnonymous: true,
    secretRef: null
  }, { resolver: publicResolver });
  assert.deepEqual(
    result.issues.filter((issue) => issue.field === "mqttAnonymous" || issue.field === "secretRef"),
    [
      { field: "mqttAnonymous", code: "PUBLIC_MQTT_ANONYMOUS_FORBIDDEN" },
      { field: "secretRef", code: "PUBLIC_CREDENTIAL_REQUIRED" }
    ]
  );
});

test("valid authenticated public profiles pass with public DNS answers", async () => {
  const result = await validateDeviceNetworkProfile(publicProfile, { resolver: publicResolver });
  assert.equal(result.valid, true);
});

test("credentials must never be embedded in endpoint URLs", async () => {
  const result = await validateDeviceNetworkProfile({
    ...lanProfile,
    mqttEndpoint: "mqtt://demo:password@192.168.20.10:1883"
  }, { resolver: privateResolver });
  assert(result.issues.some((issue) => issue.code === "ENDPOINT_INLINE_CREDENTIALS_FORBIDDEN"));
});
