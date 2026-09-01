import assert from "node:assert/strict";
import test from "node:test";

import { correlationId, redactSensitive, structuredLog } from "./observability.ts";
import { parseWebRuntimeConfig } from "./runtime-config.ts";

test("runtime configuration reports all required values", () => {
  assert.throws(
    () => parseWebRuntimeConfig({}),
    /DATABASE_URL is required; AUTH_SECRET must contain at least 16 characters/
  );
});

test("runtime configuration accepts valid values and defaults log level", () => {
  const config = parseWebRuntimeConfig({
    DATABASE_URL: "postgresql://database.example/aerosight",
    AUTH_SECRET: "0123456789abcdef"
  });
  assert.equal(config.logLevel, "info");
});

test("correlation IDs accept safe upstream values and replace unsafe values", () => {
  assert.equal(correlationId("request-123"), "request-123");
  assert.notEqual(correlationId("contains spaces"), "contains spaces");
});

test("redaction removes nested secrets and credentials embedded in errors", () => {
  const redacted = redactSensitive({
    authorization: "Bearer top-secret",
    nested: { apiKey: "key-value", url: "https://example.test/run?token=token-value" },
    error: new Error("request failed with Bearer leaked-token")
  });
  const serialized = JSON.stringify(redacted);
  assert(!serialized.includes("top-secret"));
  assert(!serialized.includes("key-value"));
  assert(!serialized.includes("token-value"));
  assert(!serialized.includes("leaked-token"));
});

test("FlightHub log, trace and API snapshots share the sensitive-data redactor", () => {
  const secrets = [
    "organization-token-plaintext",
    "1581FABCDEFGHIJKL",
    "plain-serial-from-decrypt",
    "temporary-access-id",
    "temporary-access-secret",
    "temporary-security-token",
    "signed-value-plaintext",
    "live-playback-token",
    "raw-upstream-secret"
  ];
  const payload = {
    x_user_token: secrets[0],
    device_sn: secrets[1],
    sn_decrypt_result: { mapping: { ciphertext: secrets[2] } },
    storage_sts: {
      access_key_id: secrets[3],
      access_key_secret: secrets[4],
      security_token: secrets[5]
    },
    download_url: `https://objects.example/item?X-Amz-Signature=${secrets[6]}`,
    live_token: secrets[7],
    upstream_error: new Error(`${secrets[8]} Bearer ${secrets[0]} sn=${secrets[1]}`)
  };
  const snapshots = [
    JSON.stringify(redactSensitive(payload)),
    JSON.stringify({ trace: redactSensitive(payload) }),
    JSON.stringify({ api: redactSensitive(payload) })
  ];
  const original = console.info;
  let logged = "";
  console.info = (message?: unknown) => { logged += String(message); };
  try {
    structuredLog("info", "FlightHub probe", payload);
  } finally {
    console.info = original;
  }
  snapshots.push(logged);
  for (const snapshot of snapshots) {
    for (const secret of secrets) assert(!snapshot.includes(secret), `snapshot leaked ${secret}`);
  }
});
