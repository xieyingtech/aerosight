import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path: string) => readFileSync(new URL(`../../../${path}`, import.meta.url), "utf8");

test("FlightHub diagnostics route is private, read-only, and uses the authorized diagnostic service", () => {
  const route = read("apps/web/app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/diagnostics/route.ts");
  assert.match(route, /export async function GET/);
  assert.match(route, /readFlightHubConnectorDiagnostics/);
  assert.match(route, /private, no-store/);
  assert(!/export async function (POST|PUT|PATCH|DELETE)/.test(route));
});

test("diagnostic SQL exposes watermarks and evidence without secret-bearing columns", () => {
  const core = read("apps/web/lib/dji-flighthub-diagnostics-core.ts");
  for (const marker of ["flighthub-diagnostics:access", "flighthub-diagnostics:watermarks", "flighthub-diagnostics:capabilities"]) {
    assert.match(core, new RegExp(marker));
  }
  for (const forbidden of ["credential_envelope_json", "config_json", "cursor_json", "remote_id", "identity_json"]) {
    assert(!core.includes(forbidden), `diagnostics selected ${forbidden}`);
  }
});
