import assert from "node:assert/strict";
import test from "node:test";

import { FlightHubClientError } from "./dji-flighthub-client-core.ts";
import {
  buildFlightHubConnectionPlan,
  FlightHubConnectionError,
  flightHubConnectionInputSchema,
  flightHubScopeFingerprint,
  revalidateSelectedFlightHubProject,
} from "./dji-flighthub-connection-core.ts";

const PROJECT_UUID = "00000000-0000-4000-8000-000000000001";
const OTHER_PROJECT_UUID = "00000000-0000-4000-8000-000000000002";
const TOKEN = "token-must-remain-transient";
const projects = [{
  uuid: PROJECT_UUID,
  name: "脱敏项目",
  organizationUuid: "00000000-0000-4000-8000-000000000010",
}];

test("connection input accepts only token and selected project UUID", () => {
  assert.deepEqual(flightHubConnectionInputSchema.parse({ token: TOKEN, projectUuid: PROJECT_UUID }), {
    token: TOKEN,
    projectUuid: PROJECT_UUID,
  });
  assert.throws(() => flightHubConnectionInputSchema.parse({
    token: TOKEN,
    projectUuid: PROJECT_UUID,
    baseUrl: "https://example.test",
  }));
});

test("final connection always re-fetches projects and rejects a forged or revoked UUID", async () => {
  let calls = 0;
  const client = { listProjects: async (token: string) => {
    calls += 1;
    assert.equal(token, TOKEN);
    return projects;
  } };
  assert.deepEqual(await revalidateSelectedFlightHubProject(client, TOKEN, PROJECT_UUID), projects[0]);
  await assert.rejects(
    () => revalidateSelectedFlightHubProject(client, TOKEN, OTHER_PROJECT_UUID),
    (error) => error instanceof FlightHubConnectionError && error.safeCode === "project_access_changed"
  );
  assert.equal(calls, 2);
});

test("upstream validation errors are normalized and never contain the token", async () => {
  const client = { listProjects: async () => {
    throw new FlightHubClientError("credential_invalid", false, 401);
  } };
  await assert.rejects(
    () => revalidateSelectedFlightHubProject(client, TOKEN, PROJECT_UUID),
    (error) => error instanceof FlightHubConnectionError &&
      error.safeCode === "credential_invalid" && !error.message.includes(TOKEN)
  );
});

test("connection plan and audit fingerprint contain no credential", () => {
  const plan = buildFlightHubConnectionPlan(projects[0]);
  assert.equal(plan.connectorKey, "dji.flighthub2");
  assert.equal(plan.adapterType, "dji-flighthub2");
  assert.equal(plan.externalScopeKey, PROJECT_UUID);
  assert.equal(plan.discoveryScope.organizationUuid, projects[0].organizationUuid);
  assert.equal(plan.config.readOnly, true);
  assert.equal(plan.capabilities.inventoryRead, true);
  assert(!JSON.stringify(plan).includes(TOKEN));
  const fingerprint = flightHubScopeFingerprint(PROJECT_UUID);
  assert.match(fingerprint, /^[0-9a-f]{12}$/);
  assert.notEqual(fingerprint, PROJECT_UUID);
});
