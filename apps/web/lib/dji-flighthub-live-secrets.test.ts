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

test("live media and action API responses never select secret database columns", () => {
  const media = readFileSync(new URL("./dji-flighthub-live-media.ts",import.meta.url),"utf8");
  const actions = readFileSync(new URL("./dji-flighthub-live-actions.ts",import.meta.url),"utf8");
  const publicAction = actions.slice(actions.indexOf("export async function readFlightHubLiveActionJob"));
  assert.doesNotMatch(media,/credential_envelope|bypass_option|remote_id|schema_option/i);
  assert.doesNotMatch(publicAction,/request_envelope|request_digest/i);
});

test("ordinary live session projection omits encrypted credentials and playback locators", () => {
  const source = readFileSync(new URL("./live-streams.ts",import.meta.url),"utf8");
  const publicProjection = source.slice(source.indexOf("function publicSession"),source.indexOf("export async function startLiveStream"));
  assert.doesNotMatch(publicProjection,/supplierCredentialEnvelope|credential|playbackLocatorExpiresAt/);
  assert.match(publicProjection,/statusReason/);
});
