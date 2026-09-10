import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path: string) => readFileSync(new URL(`../../../${path}`, import.meta.url), "utf8");

test("FlightHub diagnostics route is private and uses authorized read/probe services", async () => {
const source=[await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/httpapi/flighthub.go", import.meta.url), "utf8"),await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/httpapi/flighthub_lifecycle_main.go", import.meta.url), "utf8")].join("\n");
assert.match(source, /group.GET\("\/:connectorId\/diagnostics"/);
assert.match(source, /group.POST\("\/:connectorId\/diagnostics"/);
assert.match(source, /fhWorkspace\("Diagnostics"\)/);
assert.match(source, /readOnlyUpstream/);
assert.match(source, /no-store/);

});

test("diagnostic SQL exposes watermarks and evidence without secret-bearing columns", async () => {
const source=[await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/database/queries/fh_diagnostics_workspace.sql", import.meta.url), "utf8")].join("\n");
assert.match(source, /ReadFHDiagnosticsWatermarks/);
assert.match(source, /ReadFHDiagnosticsCapabilities/);
assert.doesNotMatch(source, /credential_envelope_json/);
assert.doesNotMatch(source, /config_json/);
assert.doesNotMatch(source, /cursor_json/);
assert.doesNotMatch(source, /remote_id/);
assert.doesNotMatch(source, /identity_json/);
});
