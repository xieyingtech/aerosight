import assert from "node:assert/strict";
import test from "node:test";

import {
  assertNoInlineSecrets,
  assertSupportedDeviceAdapterType,
  buildDjiConfigurationSummary,
  canManageDeviceAdapters,
  djiAdapterSetupInputSchema,
  nonEmptyDjiCredentialUpdates,
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

test("DJI setup accepts write-only credentials", () => {
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
    mqttUsername: "dock", mqttPassword: "mqtt-secret",
    appId: "app-id", appKey: "app-key", appLicense: "app-license",
    mediaPublishUser: "publisher", mediaPublishPassword: "publish-secret",
    ntpServerHost: "time.example.com",
    ntpServerPort: 123,
    gatewaySerials: ["DOCK-001"]
  });
  assert.equal(setup.mqttUsername, "dock");
  assert.equal(setup.mqttPassword, "mqtt-secret");
});

test("DJI setup summary uses official fields and never renders credentials", () => {
  const summary = buildDjiConfigurationSummary({
    gatewaySerials: ["DOCK-001"], mqttEndpoint: "mqtts://mqtt.example.com:8883",
    ntpServerHost: "time.example.com", ntpServerPort: 123
  }, "aerosight-project-adapter");
  assert.equal(summary.gateway_sn[0], "DOCK-001");
  assert.equal(summary.mqtt_broker.enable_tls, true);
  assert.equal(summary.mqtt_broker.password, "[ENCRYPTED]");
  assert.equal(summary.config.app_license, "[ENCRYPTED]");
  assert.equal(summary.config.ntp_server_port, 123);
});

test("DJI credential update keeps blank fields and overwrites only non-empty values", () => {
  assert.deepEqual(nonEmptyDjiCredentialUpdates({ mqttUsername: "", mqttPassword: "new-secret" }), {
    mqttPassword: "new-secret"
  });
});
