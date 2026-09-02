import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const componentPath = new URL("../components/dji-flighthub-wizard.tsx", import.meta.url);
const createDialogPath = new URL("../components/connector-create-dialog.tsx", import.meta.url);
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
  assert.match(source, /重新连接/);
  assert.doesNotMatch(source, />\s*(任务下发|返航|机场调试|直播控制)\s*</);
});

test("ordinary project members fail before connector management UI is rendered", async () => {
  const source = await readFile(pagePath, "utf8");
  const guard = source.indexOf('if (project.role === "member")');
  const render = source.indexOf("<DjiFlightHubConnections");
  assert.ok(guard >= 0 && render > guard);
  assert.match(source, /PROJECT_ACCESS_DENIED/);
});

test("connector management is list-first and creates only from the type chooser", async () => {
  const [pageSource, componentSource, dialogSource] = await Promise.all([
    readFile(pagePath, "utf8"),
    readFile(componentPath, "utf8"),
    readFile(createDialogPath, "utf8"),
  ]);
  assert.match(pageSource, /actions=\{<ConnectorCreateDialog/);
  assert.match(componentSource, /<Table>/);
  assert.match(componentSource, /useState<string \| null>\(null\)/);
  assert.match(componentSource, /已断开/);
  assert.doesNotMatch(pageSource, /DjiAdapterWizard/);
  assert.match(dialogSource, /选择连接器类型/);
  assert.match(dialogSource, /type ConnectorType = "dji\.flighthub2"/);
  assert.doesNotMatch(dialogSource, /DjiAdapterWizard|mqttEndpoint|appLicense/);
});

test("closing the create dialog unmounts setup and clears its selected type", async () => {
  const dialogSource = await readFile(createDialogPath, "utf8");
  assert.match(dialogSource, /if \(!nextOpen\) setSelectedType\(null\)/);
  assert.match(dialogSource, /selectedType === null/);
  assert.match(dialogSource, /<DjiFlightHubSetup/);
});
