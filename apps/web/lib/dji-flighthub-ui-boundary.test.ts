import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const componentPath = new URL("../components/dji-flighthub-wizard.tsx", import.meta.url);
const pagePath = new URL("../app/(app)/projects/[id]/connectors/page.tsx", import.meta.url);

test("FlightHub wizard keeps tokens in component memory and clears every terminal path", async () => {
  const source = await readFile(componentPath, "utf8");
  assert.match(source, /useState\(""\)/);
  assert.match(source, /clearTransientToken\(\)/);
  assert.match(source, /useEffect\(\(\) => \(\) =>/);
  assert.match(source, /setUpdateTokens\(\(current\) => \(\{ \.\.\.current, \[connectorId\]: "" \}\)\)/);
  assert.doesNotMatch(source, /localStorage|sessionStorage|document\.cookie|URLSearchParams/);
  assert.doesNotMatch(source, /href=.*token|searchParams.*token/i);
});

test("FlightHub wizard serializes repeated actions and exposes no control action", async () => {
  const source = await readFile(componentPath, "utf8");
  assert.match(source, /disabled=\{busyAction !== null/);
  assert.match(source, /data\.deduplicated/);
  assert.match(source, /立即同步/);
  assert.match(source, /更新 Token/);
  assert.match(source, /断开/);
  assert.doesNotMatch(source, />\s*(任务下发|返航|机场调试|直播控制)\s*</);
});

test("ordinary project members fail before connector management UI is rendered", async () => {
  const source = await readFile(pagePath, "utf8");
  const guard = source.indexOf('if (project.role === "member")');
  const render = source.indexOf("<DjiFlightHubWizard");
  assert.ok(guard >= 0 && render > guard);
  assert.match(source, /PROJECT_ACCESS_DENIED/);
});
