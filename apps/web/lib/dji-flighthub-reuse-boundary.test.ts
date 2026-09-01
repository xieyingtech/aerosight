import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

async function readRepoFile(relativePath: string) {
  return readFile(new URL(`../../../${relativePath}`, import.meta.url), "utf8");
}

test("FlightHub reuses the generic connector and device persistence boundary", async () => {
  const schema = await readRepoFile("db/schema.sql");
  for (const [label, pattern] of [
    ["connector definitions", /create table\s+"?connector_definitions"?/i],
    ["connector instances", /create view\s+"?connector_instances"?/i],
    ["connector sync runs", /create table\s+"?connector_sync_runs"?/i],
    ["external device identities", /create table\s+"?device_external_identities"?/i],
    ["device connector bindings", /create table\s+"?device_connector_bindings"?/i],
    ["outbox events", /create table\s+"?outbox_events"?/i],
    ["credential envelope", /"?credential_envelope_json"?\s+jsonb/i],
  ] as const) {
    assert(
      pattern.test(schema),
      `generic persistence primitive is missing: ${label}`
    );
  }

  assert(
    !/create\s+(?:unlogged\s+)?table(?:\s+if\s+not\s+exists)?\s+[\w.]*flighthub/i.test(schema),
    "FlightHub must not introduce a connector-specific table"
  );
  assert(
    !/create\s+(?:unlogged\s+)?table(?:\s+if\s+not\s+exists)?\s+[\w.]*dji_flighthub/i.test(schema),
    "FlightHub must not introduce a parallel DJI-specific ledger"
  );
});

test("FlightHub can reuse connector runtime, encrypted credentials, outbox, and DJI product types", async () => {
  const [registry, synchronizer, credentials, outbox, dock2, dock3] = await Promise.all([
    readRepoFile("apps/worker/internal/connector/registry.go"),
    readRepoFile("apps/worker/internal/connector/sync.go"),
    readRepoFile("apps/worker/internal/credentials/credentials.go"),
    readRepoFile("apps/worker/internal/outbox/outbox.go"),
    readRepoFile("apps/worker/internal/dji/products.go"),
    readRepoFile("apps/worker/internal/dji/dock3_products.go"),
  ]);

  assert.match(registry, /type ExternalDevice struct/);
  assert.match(registry, /CompleteSnapshot bool/);
  assert.match(registry, /ScopeFilter/);
  assert.match(synchronizer, /device_external_identities/);
  assert.match(synchronizer, /connector_sync_runs/);
  assert.match(credentials, /aes\.NewCipher/);
  assert.match(outbox, /outbox_events/);
  assert.match(dock2, /DJI Dock 2/);
  assert.match(dock3, /DJI Dock 3/);
});
