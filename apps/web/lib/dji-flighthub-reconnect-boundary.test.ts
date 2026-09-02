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
  const [lifecycle, route, component] = await Promise.all([
    readFile(lifecyclePath, "utf8"),
    readFile(routePath, "utf8"),
    readFile(componentPath, "utf8"),
  ]);
  const reconnectStart = lifecycle.indexOf("export async function reconnectFlightHubConnection");
  assert.ok(reconnectStart >= 0);
  const reconnect = lifecycle.slice(reconnectStart);

  assert.match(reconnect, /withAuditedProjectWrite/);
  assert.match(reconnect, /lockFlightHubConnector\(client, projectId, connectorId\)/);
  assert.match(reconnect, /connector\.status !== "disabled"/);
  assert.match(reconnect, /set status='connecting'/);
  assert.match(reconnect, /device_connector_bindings[\s\S]*set status='active', unbound_at=null/);
  assert.match(reconnect, /trigger: "reconnect"/);
  assert.doesNotMatch(reconnect, /credential_envelope_json|encryptCredentialObject|token:/i);

  assert.match(route, /export async function PUT/);
  assert.match(route, /reconnectFlightHubConnection/);
  assert.match(component, /selectedConnector\.status === "disabled"[\s\S]*重新连接/);
  assert.match(component, /action === "reconnect" \? "PUT"/);
});
