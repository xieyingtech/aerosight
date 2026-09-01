import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

type ContractCase = {
  name: string;
  endpoint: string;
  httpStatus: number;
  headers?: Record<string, string>;
  body?: unknown;
  generatedBody?: { repeat: number; templateCase: string };
  expected: {
    kind: "success" | "error" | "incomplete";
    itemCount?: number;
    safeCode?: string;
    retryable?: boolean;
    retryAfterSeconds?: number;
  };
};

type ContractFixture = {
  contractVersion: string;
  deviceDirectoryLimit: number;
  cases: ContractCase[];
};

async function loadContractFixture(): Promise<ContractFixture> {
  const fixtureUrl = new URL(
    "../../../contracts/dji-flighthub/v2/fixtures/cases.json",
    import.meta.url
  );
  return JSON.parse(await readFile(fixtureUrl, "utf8")) as ContractFixture;
}

test("FlightHub contract fixture covers the versioned China public-cloud surface", async () => {
  const fixture = await loadContractFixture();
  assert.match(fixture.contractVersion, /^dji-flighthub-openapi-v2-cn-/);
  assert.equal(fixture.deviceDirectoryLimit, 1000);

  const names = new Set(fixture.cases.map((item) => item.name));
  for (const requiredName of [
    "project-list",
    "project-empty",
    "device-directory",
    "device-directory-limit",
    "http-401",
    "business-200401",
    "http-403",
    "http-404",
    "http-429",
    "http-503",
    "malformed-response",
  ]) {
    assert(names.has(requiredName), `missing shared contract case ${requiredName}`);
  }
});

test("FlightHub shared fixtures expose stable fields and safe error codes", async () => {
  const fixture = await loadContractFixture();
  const byName = new Map(fixture.cases.map((item) => [item.name, item]));

  const projects = byName.get("project-list")?.body as {
    code: number;
    data: { list: Array<{ name: string; uuid: string; org_uuid: string }> };
  };
  assert.equal(projects.code, 0);
  assert.equal(projects.data.list.length, 2);
  assert(projects.data.list.every((project) => project.name && project.uuid && project.org_uuid));

  const directory = byName.get("device-directory")?.body as {
    code: number;
    data: {
      list: Array<{
        gateway: { sn: string; device_model: { key: string; class: string } };
        drone: { sn: string; device_model: { key: string; class: string } };
      }>;
    };
  };
  assert.equal(directory.code, 0);
  assert.equal(directory.data.list[0]?.gateway.device_model.class, "airport");
  assert.equal(directory.data.list[0]?.drone.device_model.class, "drone");
  assert.match(directory.data.list[0]?.gateway.sn ?? "", /REDACTED/);
  assert.match(directory.data.list[0]?.drone.sn ?? "", /REDACTED/);

  const limit = byName.get("device-directory-limit");
  assert.equal(limit?.generatedBody?.repeat, fixture.deviceDirectoryLimit);
  assert.equal(limit?.expected.safeCode, "directory_limit_reached");

  const expectedErrors = new Map([
    ["http-401", "credential_invalid"],
    ["business-200401", "credential_invalid"],
    ["http-403", "scope_forbidden"],
    ["http-404", "scope_not_found"],
    ["http-429", "rate_limited"],
    ["http-503", "upstream_unavailable"],
    ["malformed-response", "schema_incompatible"],
  ]);
  for (const [name, safeCode] of expectedErrors) {
    assert.equal(byName.get(name)?.expected.safeCode, safeCode);
  }
  assert.equal(byName.get("http-429")?.headers?.["Retry-After"], "3");
  assert.equal(byName.get("http-429")?.expected.retryAfterSeconds, 3);
});

test("FlightHub shared fixtures contain no token or unsanitized serial", async () => {
  const fixtureUrl = new URL(
    "../../../contracts/dji-flighthub/v2/fixtures/cases.json",
    import.meta.url
  );
  const source = await readFile(fixtureUrl, "utf8");
  assert(!/eyJ[A-Za-z0-9_-]{10,}\./.test(source), "fixture must not contain a JWT");
  assert(!/7CT[A-Z0-9]{8,}/.test(source), "fixture must not contain a complete DJI Dock SN");
  assert(!/1581F[A-Z0-9]{8,}/.test(source), "fixture must not contain a complete DJI aircraft SN");
});
