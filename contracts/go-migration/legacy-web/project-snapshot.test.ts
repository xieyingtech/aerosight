import assert from "node:assert/strict";
import test from "node:test";
import type { QueryResult, QueryResultRow } from "pg";
import { readProjectSituationSnapshot } from "./project-snapshot-core.ts";

function result<T extends QueryResultRow>(rows: T[]): QueryResult<T> {
  return { rows, command: "SELECT", rowCount: rows.length, oid: 0, fields: [] };
}

class SnapshotClient {
  statements: string[] = [];
  released = false;
  private readonly authorized: boolean;
  constructor(authorized: boolean) { this.authorized = authorized; }
  async query<T extends QueryResultRow>(text: string): Promise<QueryResult<T>> {
    this.statements.push(text);
    if (text.includes("snapshot:project-scope")) {
      return result(this.authorized ? [{ id: 7, name: "North", teamId: 3, role: "member", dependencyHealth: {} } as unknown as T] : []);
    }
    return result([]);
  }
  release() { this.released = true; }
}

test("snapshot reads every layer in one repeatable-read transaction", async () => {
  const client = new SnapshotClient(true);
  const snapshot = await readProjectSituationSnapshot(2, 7, async () => client as never);
  assert.equal(snapshot?.project.id, 7);
  assert.match(client.statements[0], /repeatable read read only/i);
  assert.equal(client.statements.at(-1), "commit");
  for (const marker of ["snapshot:devices", "snapshot:device-grants", "snapshot:tracks", "snapshot:active-tasks", "snapshot:live-streams", "snapshot:realtime-channels", "snapshot:diagnostics", "snapshot:media", "snapshot:suspected-construction", "snapshot:issues", "snapshot:alerts"]) {
    assert(client.statements.some((statement) => statement.includes(marker)));
  }
  const deviceStatement = client.statements.find((statement) => statement.includes("snapshot:devices"));
  assert.match(deviceStatement ?? "", /join device_types/i);
  assert.match(deviceStatement ?? "", /join driver_definitions/i);
  assert.match(deviceStatement ?? "", /rawCapabilities/);
  assert.match(deviceStatement ?? "", /rawChannels/);
  assert(!JSON.stringify(snapshot).includes("dependencyHealthJson"));
  assert(client.released);
});

test("unauthorized project id reveals no scoped resources", async () => {
  const client = new SnapshotClient(false);
  const snapshot = await readProjectSituationSnapshot(2, 999, async () => client as never);
  assert.equal(snapshot, null);
  assert(!client.statements.some((statement) => statement.includes("snapshot:devices")));
  assert.equal(client.statements.at(-1), "commit");
});
