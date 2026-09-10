import assert from "node:assert/strict";
import test from "node:test";
import type { QueryResult, QueryResultRow } from "pg";
import { parseReplayQuery, readProjectReplay } from "./project-replay-core.ts";

function result<T extends QueryResultRow>(rows: T[]): QueryResult<T> { return { rows, command: "SELECT", rowCount: rows.length, oid: 0, fields: [] }; }
class ReplayClient {
  statements: string[] = [];
  constructor(privateAuthorized: boolean) { this.authorized = privateAuthorized; }
  authorized: boolean;
  async query<T extends QueryResultRow>(text: string): Promise<QueryResult<T>> {
    this.statements.push(text);
    if (text.includes("replay:project-scope")) return result(this.authorized ? [{ allowed: 1 } as unknown as T] : []);
    return result([]);
  }
  release() {}
}

test("replay query validates time, device, and spatial filters", () => {
  const query = parseReplayQuery(new URL("https://example.test/replay?from=2026-08-24T10:00:00Z&to=2026-08-24T11:00:00Z&deviceTypes=drone,ground_robot&bbox=120,30,121,31"));
  assert.deepEqual(query.deviceTypes, ["drone", "ground_robot"]);
  assert.deepEqual(query.bbox, [120, 30, 121, 31]);
  assert.throws(() => parseReplayQuery(new URL("https://example.test/replay?from=2026-08-01&to=2026-08-24")), /REPLAY_WINDOW_TOO_LARGE/);
});

test("replay uses one scoped transaction and leaks nothing cross-project", async () => {
  const denied = new ReplayClient(false);
  const input = parseReplayQuery(new URL("https://example.test/replay?from=2026-08-24T10:00:00Z&to=2026-08-24T11:00:00Z"));
  assert.equal(await readProjectReplay(1, 999, input, async () => denied as never), null);
  assert(!denied.statements.some((statement) => statement.includes("replay:poses")));
  const allowed = new ReplayClient(true);
  const replay = await readProjectReplay(1, 7, input, async () => allowed as never);
  assert.equal(replay?.mode, "replay");
  for (const marker of ["replay:poses", "replay:media", "replay:events"]) assert(allowed.statements.some((statement) => statement.includes(marker)));
  const poseStatement = allowed.statements.find((statement) => statement.includes("replay:poses"));
  assert.match(poseStatement ?? "", /join device_types/i);
  assert.match(poseStatement ?? "", /join driver_definitions/i);
});
