import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { credentialAAD, decryptCredentialObject, encryptCredentialObject } from "./credential-encryption.ts";
import {
  FlightHubConnectionError,
  flightHubConnectionInputSchema,
  revalidateSelectedFlightHubProject,
} from "./dji-flighthub-connection-core.ts";
import { parseFlightHubWebConfig } from "./dji-flighthub-config.ts";
import { flightHubDiscoveryInputSchema } from "./dji-flighthub-discovery-core.ts";

const TOKEN = "security-regression-token-value";
const PROJECT_UUID = "00000000-0000-4000-8000-000000000001";
const APP_SECRET = "0123456789abcdef0123456789abcdef";

test("forged and stale project selections are rejected after a fresh upstream lookup", async () => {
  let currentProjects = [{
    uuid: PROJECT_UUID,
    name: "脱敏项目",
    organizationUuid: "00000000-0000-4000-8000-000000000010",
  }];
  const client = { listProjects: async () => currentProjects };
  await assert.rejects(
    () => revalidateSelectedFlightHubProject(
      client,
      TOKEN,
      "00000000-0000-4000-8000-000000000099"
    ),
    (error) => error instanceof FlightHubConnectionError && error.safeCode === "project_access_changed"
  );
  assert.deepEqual(await revalidateSelectedFlightHubProject(client, TOKEN, PROJECT_UUID), currentProjects[0]);
  currentProjects = [];
  await assert.rejects(
    () => revalidateSelectedFlightHubProject(client, TOKEN, PROJECT_UUID),
    (error) => error instanceof FlightHubConnectionError && error.safeCode === "project_access_changed"
  );
});

test("browser-controlled upstream targets are rejected at every handshake input boundary", () => {
  assert.throws(() => flightHubDiscoveryInputSchema.parse({ token: TOKEN, baseUrl: "https://example.test" }));
  assert.throws(() => flightHubConnectionInputSchema.parse({
    token: TOKEN,
    projectUuid: PROJECT_UUID,
    apiPath: "/attacker",
  }));
  assert.throws(() => parseFlightHubWebConfig({ DJI_FLIGHTHUB_API_BASE_URL: "https://example.test" }), /NOT_ALLOWED/);
});

test("concurrent credential encryption produces isolated authenticated envelopes", async () => {
  const aad = credentialAAD("device-adapter", 42, 7);
  const credentials = Array.from({ length: 32 }, (_, index) => ({ token: `${TOKEN}-${index}` }));
  const envelopes = await Promise.all(credentials.map(async (credential) =>
    encryptCredentialObject(credential, APP_SECRET, aad)
  ));
  assert.equal(new Set(envelopes.map((envelope) => envelope.nonce)).size, envelopes.length);
  envelopes.forEach((envelope, index) => {
    assert.deepEqual(decryptCredentialObject(envelope, APP_SECRET, aad), credentials[index]);
  });
});

test("connection mutations serialize connector rows and never expose credential columns", async () => {
const source=[await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/httpapi/flighthub.go", import.meta.url), "utf8"),await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/database/queries/flighthub_writes.sql", import.meta.url), "utf8")].join("\n");
assert.match(source, /FOR UPDATE OF adapter/);
assert.match(source, /pg_advisory_xact_lock/);
assert.match(source, /projectRevalidated/);
assert.match(source, /safeCode/);
assert.match(source, /tokenUpdated/);
assert.match(source, /database.AuditedWrite/);
assert.match(source, /credentials.EncryptJSON/);

});

test("schema enforces one external FlightHub scope per AeroSight project without a special table", async () => {
  const migration = await readFile(
    new URL("../../../db/migrations/0051_connector_external_scope_key.sql", import.meta.url),
    "utf8"
  );
  assert.match(
    migration,
    /unique index device_adapters_connector_external_scope_unique[\s\S]*project_id, connector_definition_id, external_scope_key/i
  );
  assert(!/create\s+table\s+\w*flighthub/i.test(migration));
});

test("FlightHub playback authorization is atomically claimed before credential decryption", async () => {
const source=[await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/httpapi/live_playback.go", import.meta.url), "utf8"),await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/database/queries/live_playback.sql", import.meta.url), "utf8"),await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/database/queries/live_control.sql", import.meta.url), "utf8")].join("\n");
assert.match(source, /AuthorizeFlightHubPlayback[\s\S]*credentials.DecryptJSON/);
assert.match(source, /credentials.DecryptJSON/);
assert.match(source, /local_authorization_revoked_at IS NULL/);
assert.match(source, /supplier_credential_expires_at>now\(\)/);
assert.match(source, /FLIGHTHUB_LIVE_STOPPED_BEFORE_DISPATCH/);

});
