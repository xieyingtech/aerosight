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
const AUTH_SECRET = "0123456789abcdef0123456789abcdef";

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
    encryptCredentialObject(credential, AUTH_SECRET, aad)
  ));
  assert.equal(new Set(envelopes.map((envelope) => envelope.nonce)).size, envelopes.length);
  envelopes.forEach((envelope, index) => {
    assert.deepEqual(decryptCredentialObject(envelope, AUTH_SECRET, aad), credentials[index]);
  });
});

test("connection mutations serialize connector rows and never expose credential columns", async () => {
  const lifecycleSource = await readFile(new URL("./dji-flighthub-lifecycle.ts", import.meta.url), "utf8");
  const connectionSource = await readFile(new URL("./dji-flighthub-connections.ts", import.meta.url), "utf8");
  const routeSource = await readFile(
    new URL("../app/api/projects/[id]/connectors/dji-flighthub/route.ts", import.meta.url),
    "utf8"
  );

  assert.match(lifecycleSource, /for update of adapter/i);
  assert.match(lifecycleSource, /pg_advisory_xact_lock/);
  assert.match(
    lifecycleSource,
    /projectRevalidated:\s*false[\s\S]*errorCode:\s*safeError\.safeCode[\s\S]*tokenUpdated:\s*false/,
    "failed token validation must create a credential-free audit result before rethrowing"
  );
  assert.match(connectionSource, /withAuditedProjectWrite/);
  assert.match(connectionSource, /credential_envelope_json/);
  assert(!routeSource.includes("credential_envelope_json"));
  assert(!routeSource.includes("ciphertext"));
  assert(!routeSource.includes("authenticationTag"));
  assert(!routeSource.includes("localStorage"));
  assert(!routeSource.includes("sessionStorage"));
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
  const source = await readFile(new URL("./live-streams.ts", import.meta.url), "utf8");
  const branchStart = source.indexOf('if (session.sourceType === "dji_flighthub")');
  assert(branchStart >= 0, "FlightHub playback branch is missing");
  const branch = source.slice(branchStart, source.indexOf('if (session.sourceType !== "simulator"', branchStart));
  const authorizationUpdate = branch.indexOf("update live_streams set");
  const decrypt = branch.indexOf("decryptCredentialObject");
  assert(authorizationUpdate >= 0 && decrypt > authorizationUpdate,
    "supplier credential must only be decrypted after the conditional authorization update");
  assert.match(branch, /status in\('live','degraded'\)/);
  assert.match(branch, /local_authorization_revoked_at is null/);
  assert.match(branch, /supplier_credential_expires_at>now\(\)/);
  assert.match(branch, /supplier_credential_envelope_json is not null[\s\S]*returning[\s\S]*supplier_credential_envelope_json/);
  assert.match(source, /FLIGHTHUB_LIVE_STOPPED_BEFORE_DISPATCH/);
  assert.match(source, /not\(source_type='dji_flighthub' and status='failed'\)/);
});
