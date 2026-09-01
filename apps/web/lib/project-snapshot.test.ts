import assert from "node:assert/strict";
import test from "node:test";
import { Pool, type QueryResult, type QueryResultRow } from "pg";
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
  for (const marker of ["snapshot:devices", "snapshot:device-grants", "snapshot:tracks", "snapshot:active-tasks", "snapshot:task-steps", "snapshot:algorithm-runs", "snapshot:live-streams", "snapshot:realtime-channels", "snapshot:diagnostics", "snapshot:media", "snapshot:suspected-construction", "snapshot:issues", "snapshot:alerts", "snapshot:regions"]) {
    assert(client.statements.some((statement) => statement.includes(marker)));
  }
  const deviceStatement = client.statements.find((statement) => statement.includes("snapshot:devices"));
  assert.match(deviceStatement ?? "", /join device_types/i);
  assert.match(deviceStatement ?? "", /join driver_definitions/i);
  assert.match(deviceStatement ?? "", /rawCapabilities/);
  assert.match(deviceStatement ?? "", /rawChannels/);
  const regionStatement = client.statements.find((statement) => statement.includes("snapshot:regions"));
  assert.match(regionStatement ?? "", /version\.project_id=\$1/);
  assert.match(regionStatement ?? "", /policy\.project_id=\$1/g);
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

test("database snapshot returns an unverified original FlightHub pose with explicit provenance", {
  skip: !process.env.AEROSIGHT_TEST_DATABASE_URL,
}, async () => {
  const pool = new Pool({ connectionString: process.env.AEROSIGHT_TEST_DATABASE_URL });
  const suffix = Date.now();
  let teamId = 0;
  let userId = 0;
  try {
    userId = (await pool.query<{ id: number }>(`insert into users(name,email) values($1,$2) returning id`, ["snapshot fixture", `snapshot-${suffix}@example.test`])).rows[0].id;
    teamId = (await pool.query<{ id: number }>(`insert into teams(name) values($1) returning id`, [`snapshot-${suffix}`])).rows[0].id;
    await pool.query(`insert into team_members(team_id,user_id,role) values($1,$2,'member')`, [teamId, userId]);
    const projectId = (await pool.query<{ id: number }>(`insert into projects(team_id,name) values($1,$2) returning id`, [teamId, `snapshot-${suffix}`])).rows[0].id;
    const definitionId = (await pool.query<{ id: string }>(`select id::text from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`)).rows[0].id;
    const adapterId = (await pool.query<{ id: string }>(`insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status)
      values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id::text`, [projectId, teamId, `snapshot-${suffix}`, definitionId])).rows[0].id;
    const deviceId = (await pool.query<{ id: number }>(`insert into devices(project_id,adapter_id,device_type_id,name,type,status,data_freshness)
      select $1,$2,id,'Matrice 3TD','aircraft','online','fresh' from device_types where type_key='dji.matrice3td' and status='active'
      order by version desc limit 1 returning id`, [projectId, adapterId])).rows[0].id;
    const capturedAt = new Date("2026-09-01T10:00:00Z");
    const observationId = (await pool.query<{ id: string }>(`insert into observations(
      project_id,team_id,adapter_id,device_id,observation_type,source_event_id,captured_at,received_at,original_geometry,validity
    ) values($1,$2,$3,$4,'pose',$5,$6,$6,ST_MakePoint(120,30,20),'degraded') returning id::text`,
    [projectId, teamId, adapterId, deviceId, `snapshot-pose-${suffix}`, capturedAt])).rows[0].id;
    await pool.query(`insert into poses(observation_id,project_id,device_id,captured_at,original_position,transform_version,spatial_quality)
      values($1,$2,$3,$4,ST_MakePoint(120,30,20),'dji-flighthub-state/v1','unusable')`, [observationId, projectId, deviceId, capturedAt]);
    await pool.query(`insert into device_latest_telemetry(device_id,project_id,adapter_id,event_id,telemetry_type,captured_at,received_at,payload_json,quality_json)
      values($1,$2,$3,$4,'dji.flighthub.state',$5,$5,$6,$7)`, [deviceId, projectId, adapterId, `snapshot-state-${suffix}`, capturedAt,
      { position: { validity: "valid", coordinateReference: "unverified" } }, { source: "dji-flighthub-openapi" }]);
    const snapshot = await readProjectSituationSnapshot(userId, projectId, async () => pool.connect());
    const device = snapshot?.devices[0];
    assert.equal(device?.positionStatus, "unverified");
    assert.equal(device?.positionSource, "dji-flighthub-openapi");
    assert.equal((device?.pose as { calibrationStatus?: string } | null)?.calibrationStatus, "unverified");
    assert.equal((device?.pose as { longitude?: number } | null)?.longitude, 120);
  } finally {
    if (teamId) await pool.query(`delete from teams where id=$1`, [teamId]);
    if (userId) await pool.query(`delete from users where id=$1`, [userId]);
    await pool.end();
  }
});
