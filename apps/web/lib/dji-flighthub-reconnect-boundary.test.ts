import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const lifecyclePath = new URL("./dji-flighthub-lifecycle.ts", import.meta.url);
const routePath = new URL(
  "../app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/route.ts",
  import.meta.url
);
const componentPath = new URL("../components/dji-flighthub-wizard.tsx", import.meta.url);

test("disabled FlightHub connectors expose an explicit reconnect lifecycle", async () => {
const source=[await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/httpapi/flighthub_lifecycle_main.go", import.meta.url), "utf8"),await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/database/queries/fh_lifecycle_main.sql", import.meta.url), "utf8")].join("\n");
assert.match(source, /database.AuditedWrite/);
assert.match(source, /LockFlightHubConnector/);
assert.match(source, /connector_not_disabled/);
assert.match(source, /status='connecting'/);
assert.match(source, /status='active',unbound_at=null/);
assert.match(source, /queueFlightHubSync/);

});
