import assert from "node:assert/strict";
import test from "node:test";

import {
  assertNoInlineSecrets,
  assertSupportedDeviceAdapterType,
  canManageDeviceAdapters,
  djiAdapterSetupInputSchema,
  publicDeviceAdapter
} from "./device-adapter-policy.ts";

test("only project owner and admin can manage device adapters", () => {
  assert.equal(canManageDeviceAdapters("owner"), true);
  assert.equal(canManageDeviceAdapters("admin"), true);
  assert.equal(canManageDeviceAdapters("member"), false);
  assert.equal(canManageDeviceAdapters(null), false);
});

test("known future protocol types fail explicitly instead of pretending to connect", () => {
  for (const type of ["ros2", "mqtt", "mavlink", "rtsp", "gb28181"] as const) {
    assert.throws(() => assertSupportedDeviceAdapterType(type), new RegExp(`ADAPTER_TYPE_NOT_SUPPORTED:${type}`));
  }
});

test("adapter config rejects nested inline secrets", () => {
  assert.throws(
    () => assertNoInlineSecrets({ transport: { apiKey: "do-not-store" } }),
    /INLINE_SECRET_NOT_ALLOWED/
  );
});

test("public adapter view never returns the secret reference", () => {
  const view = publicDeviceAdapter({ id: 1, name: "DJI", secretRef: "vault://aerosight/dji" });
  assert.deepEqual(view, { id: 1, name: "DJI", hasSecret: true });
  assert(!("secretRef" in view));
});

test("DJI setup accepts secret references without inline credentials", () => {
  const setup = djiAdapterSetupInputSchema.parse({
    name: "Dock fleet",
    mode: "public",
    mqttEndpoint: "mqtts://mqtt.example.com:8883",
    apiPublicBaseUrl: "https://api.example.com",
    websocketPublicUrl: "wss://api.example.com",
    mediaIngestBaseUrl: "rtmps://media.example.com:443",
    mediaPlaybackBaseUrl: "https://media.example.com",
    tlsRequired: true,
    mqttAnonymous: false,
    secretRef: "vault://aerosight/dji",
    gatewaySerials: ["DOCK-001"]
  });
  assert.equal(setup.secretRef, "vault://aerosight/dji");
  assert(!("password" in setup));
});
