import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { redactSensitive } from "./observability.ts";

const SECRET = "must-never-appear-live-secret";

test("live supplier URL and token are removed from logs and traces", () => {
  const redacted = JSON.stringify(redactSensitive({ supplier:"volc",url:`https://media.invalid/?token=${SECRET}`,
    token:SECRET,nested:{credential:SECRET},error:new Error(`live_token=${SECRET}`) }));
  assert.doesNotMatch(redacted,new RegExp(SECRET));
  assert.match(redacted,/REDACTED/);
});

test("live media and action API responses never select secret database columns", async () => {
const source=[await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/database/queries/flighthub_live_media.sql", import.meta.url), "utf8")].join("\n");
assert.match(source, /project_id/);
assert.match(source, /supplier/);
assert.doesNotMatch(source, /credential_envelope/);
assert.doesNotMatch(source, /remote_id/);
assert.doesNotMatch(source, /bypass_option/);
});

test("ordinary live session projection omits encrypted credentials and playback locators", async () => {
const source=[await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/database/queries/flighthub_live_media.sql", import.meta.url), "utf8")].join("\n");
assert.match(source, /source_type/);
assert.doesNotMatch(source, /supplier_credential_envelope_json/);
assert.doesNotMatch(source, /playback_ref/);
assert.doesNotMatch(source, /supplier_reference_digest/);
});
