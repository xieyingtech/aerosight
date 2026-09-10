import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path: string) => readFileSync(new URL(path, import.meta.url), "utf8");

test("every FlightHub high-risk web gate binds field acceptance to the current account", async () => {
for(const path of ["apps/server/internal/database/queries/fh_device_commands.sql","apps/server/internal/database/queries/project_reads.sql","apps/server/internal/database/queries/fh_control_sessions.sql","apps/server/internal/database/queries/fh_controlled_operations.sql","apps/server/internal/database/queries/fh_deviceadmin_actions.sql","apps/server/internal/database/queries/fh_flight_actions.sql","apps/server/internal/database/queries/fh_flightops_workspace.sql","apps/server/internal/database/queries/fh_geospatial_actions.sql","apps/server/internal/database/queries/fh_live_actions.sql","apps/server/internal/database/queries/fh_member_actions.sql","apps/server/internal/database/queries/fh_model_actions.sql","apps/server/internal/database/queries/fh_models_workspace.sql","apps/server/internal/database/queries/live_start.sql"]) { const source=await (await import("node:fs/promises")).readFile(new URL("../../../"+path,import.meta.url),"utf8");assert.match(source,/account_fingerprint/,path);assert.match(source,/capability\.region='cn'/,path);assert.match(source,/capability\.deployment='cn-public-cloud'/,path); }
});

test("device-bound high-risk gates require an exact model and firmware", async () => {
for(const path of ["apps/server/internal/database/queries/fh_device_commands.sql","apps/server/internal/database/queries/project_reads.sql","apps/server/internal/database/queries/fh_control_sessions.sql","apps/server/internal/database/queries/fh_controlled_operations.sql","apps/server/internal/database/queries/fh_flight_actions.sql","apps/server/internal/database/queries/fh_flightops_workspace.sql","apps/server/internal/database/queries/fh_live_actions.sql","apps/server/internal/database/queries/live_start.sql"]) {const source=await (await import("node:fs/promises")).readFile(new URL("../../../"+path,import.meta.url),"utf8");assert.match(source,/device_model/);assert.match(source,/firmware_version/);}
});

test("credential replacement clears account binding and legacy acceptance cannot match", async () => {
const source=[await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/database/queries/flighthub_writes.sql", import.meta.url), "utf8"),await (await import("node:fs/promises")).readFile(new URL("../../../db/migrations/0072_flighthub_field_acceptance_scope.sql", import.meta.url), "utf8")].join("\n");
assert.match(source, /discovery_scope_json=discovery_scope_json-'accountFingerprint'/);
assert.match(source, /account_fingerprint text/);
assert.match(source, /nulls not distinct/);

});
