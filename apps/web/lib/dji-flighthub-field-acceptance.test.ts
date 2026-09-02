import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path: string) => readFileSync(new URL(path, import.meta.url), "utf8");

test("every FlightHub high-risk web gate binds field acceptance to the current account", () => {
  for (const path of [
    "./device-commands.ts",
    "./device-tree.ts",
    "./dji-flighthub-control-sessions.ts",
    "./dji-flighthub-controlled-operations.ts",
    "./dji-flighthub-device-admin.ts",
    "./dji-flighthub-flight-actions.ts",
    "./dji-flighthub-flight-operations-core.ts",
    "./dji-flighthub-geospatial-actions.ts",
    "./dji-flighthub-live-actions.ts",
    "./dji-flighthub-management-write.ts",
    "./dji-flighthub-model-actions.ts",
    "./dji-flighthub-models-core.ts",
    "./live-streams.ts",
  ]) {
    const source = read(path);
    assert.match(source, /account_fingerprint/, `${path} is missing the account-scoped acceptance gate`);
	assert.match(source, /capability\.region='cn'/, `${path} is missing the region acceptance gate`);
	assert.match(source, /capability\.deployment='cn-public-cloud'/, `${path} is missing the deployment acceptance gate`);
  }
});

test("device-bound high-risk gates require an exact model and firmware", () => {
  for (const path of [
    "./device-commands.ts",
    "./device-tree.ts",
    "./dji-flighthub-control-sessions.ts",
    "./dji-flighthub-controlled-operations.ts",
    "./dji-flighthub-flight-actions.ts",
    "./dji-flighthub-flight-operations-core.ts",
    "./dji-flighthub-live-actions.ts",
    "./live-streams.ts",
  ]) {
    const source = read(path);
    assert.match(source, /device_model/);
    assert.match(source, /firmware_version/);
  }
});

test("credential replacement clears account binding and legacy acceptance cannot match", () => {
  const lifecycle = read("./dji-flighthub-lifecycle.ts");
  const migration = read("../../../db/migrations/0072_flighthub_field_acceptance_scope.sql");
  assert.match(lifecycle, /discovery_scope_json=discovery_scope_json-'accountFingerprint'/);
  assert.match(migration, /account_fingerprint text/);
  assert.match(migration, /account_fingerprint ~ '\^\[a-f0-9\]\{64\}\$'/);
  assert.match(migration, /account_fingerprint, device_model, firmware_version[\s\S]*nulls not distinct/);
});
