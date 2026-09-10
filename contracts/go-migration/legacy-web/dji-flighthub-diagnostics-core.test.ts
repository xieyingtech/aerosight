import assert from "node:assert/strict";
import test from "node:test";
import { Pool, type QueryResult, type QueryResultRow } from "pg";

import { readFlightHubConnectorDiagnostics } from "./dji-flighthub-diagnostics-core.ts";

function result<T extends QueryResultRow>(rows: T[]): QueryResult<T> {
  return { command: "SELECT", rowCount: rows.length, oid: 0, fields: [], rows };
}

class DiagnosticClient {
  statements: Array<{ text: string; values: unknown[] }> = [];
  released = false;
  private readonly authorized: boolean;

  constructor(authorized: boolean) { this.authorized = authorized; }

  async query<T extends QueryResultRow>(text: string, values: unknown[] = []): Promise<QueryResult<T>> {
    this.statements.push({ text, values });
    if (text.startsWith("begin") || text === "commit" || text === "rollback") return result([]) as QueryResult<T>;
    if (text.includes("flighthub-diagnostics:access")) {
      return result(this.authorized ? [{
        id: "41", name: "司空连接", status: "connected", lastErrorCode: "rate_limited", lastCheckedAt: "2026-09-01T12:00:00Z",
        credentialEnvelope: "TOKEN_SHOULD_NOT_ESCAPE",
      }] : []) as unknown as QueryResult<T>;
    }
    if (text.includes("flighthub-diagnostics:watermarks")) {
      return result([{
        resourceKind: "device-state", status: "backoff", attemptCount: 2, lastErrorCode: "rate_limited",
        lastStartedAt: "2026-09-01T11:59:00Z", lastSucceededAt: "2026-09-01T11:58:00Z", nextAttemptAt: "2026-09-01T12:01:00Z",
        cursor: "REMOTE_UUID_SHOULD_NOT_ESCAPE",
      }]) as unknown as QueryResult<T>;
    }
    if (text.includes("flighthub-diagnostics:capabilities")) {
      return result([{
        capabilityCode: "state.read", status: "supported", evidenceLevel: "live-read", region: "cn", deployment: "cn-public-cloud",
        deviceModel: "dock-model", firmwareVersion: "01.02.0300", reason: "read_probe_succeeded", endpointId: "458069501e0",
        layers: { contract: "supported", deployment: "supported", account: "supported", implementation: "supported", acceptance: "supported", token: "SECRET" },
        verifiedAt: "2026-09-01T11:00:00Z", expiresAt: "2026-09-01T11:30:00Z", expired: true,
        rawResponse: "https://signed.example/secret",
      }, {
        capabilityCode: "live.read", status: "empty", evidenceLevel: "live-read", region: "cn", deployment: "cn-public-cloud",
        deviceModel: null, firmwareVersion: null, reason: "upstream_empty", endpointId: "457494965e0",
        layers: { contract: "supported", deployment: "supported", account: "empty", implementation: "supported", acceptance: "supported" },
        verifiedAt: "2026-09-01T12:00:00Z", expiresAt: null, expired: false,
      }]) as unknown as QueryResult<T>;
    }
    throw new Error(`unexpected query: ${text}`);
  }

  release() { this.released = true; }
}

for (const role of ["member", "admin"] as const) {
  test(`${role} project membership can read redacted connector diagnostics`, async () => {
    const client = new DiagnosticClient(true);
    const diagnostics = await readFlightHubConnectorDiagnostics(role === "member" ? 7 : 8, 11, "41", async () => client as never);
    assert(diagnostics);
    assert.equal(diagnostics.connector.id, "41");
    assert.equal(diagnostics.resourceWatermarks[0].lastErrorCode, "rate_limited");
    assert.equal(diagnostics.capabilities[0].status, "unverified");
    assert.equal(diagnostics.capabilities[0].reason, "evidence_expired");
    assert.equal(diagnostics.capabilities[1].status, "empty");
    assert(client.released);
    const serialized = JSON.stringify(diagnostics);
    for (const secret of ["TOKEN_SHOULD_NOT_ESCAPE", "REMOTE_UUID_SHOULD_NOT_ESCAPE", "https://signed.example/secret", "SECRET"]) {
      assert(!serialized.includes(secret), `diagnostics leaked ${secret}`);
    }
    const access = client.statements.find(({ text }) => text.includes("flighthub-diagnostics:access"));
    assert.deepEqual(access?.values, [role === "member" ? 7 : 8, 11, "41"]);
    assert.match(access?.text ?? "", /team_members membership/);
  });
}

test("cross-project connector lookup returns no diagnostics and reads no child tables", async () => {
  const client = new DiagnosticClient(false);
  const diagnostics = await readFlightHubConnectorDiagnostics(7, 12, "41", async () => client as never);
  assert.equal(diagnostics, null);
  assert.equal(client.statements.filter(({ text }) => text.includes("flighthub-diagnostics:")).length, 1);
  assert(!client.statements.some(({ text }) => text.includes("connector_resource_sync_states") || text.includes("connector_capability_snapshots")));
  assert(client.released);
});

test("Postgres diagnostics authorize member and admin while isolating another tenant", {
  skip: !process.env.AEROSIGHT_TEST_DATABASE_URL,
}, async () => {
  const pool = new Pool({ connectionString: process.env.AEROSIGHT_TEST_DATABASE_URL });
  const suffix = Date.now();
  const userIds: number[] = [];
  const teamIds: number[] = [];
  try {
    for (const label of ["member", "admin", "outsider"]) {
      userIds.push((await pool.query<{ id: number }>(`insert into users(name,email) values($1,$2) returning id`, [label, `${label}-${suffix}@example.test`])).rows[0].id);
    }
    for (const label of ["primary", "foreign"]) {
      teamIds.push((await pool.query<{ id: number }>(`insert into teams(name) values($1) returning id`, [`diagnostic-${label}-${suffix}`])).rows[0].id);
    }
    await pool.query(`insert into team_members(team_id,user_id,role) values($1,$2,'member'),($1,$3,'admin'),($4,$5,'owner')`,
      [teamIds[0], userIds[0], userIds[1], teamIds[1], userIds[2]]);
    const primaryProjectId = (await pool.query<{ id: number }>(`insert into projects(team_id,name) values($1,$2) returning id`, [teamIds[0], `primary-${suffix}`])).rows[0].id;
    const foreignProjectId = (await pool.query<{ id: number }>(`insert into projects(team_id,name) values($1,$2) returning id`, [teamIds[1], `foreign-${suffix}`])).rows[0].id;
    const definitionId = (await pool.query<{ id: string }>(`select id::text from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`)).rows[0].id;
    const primaryConnectorId = (await pool.query<{ id: string }>(`insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status)
      values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id::text`, [primaryProjectId, teamIds[0], `primary-${suffix}`, definitionId])).rows[0].id;
    const foreignConnectorId = (await pool.query<{ id: string }>(`insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status)
      values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id::text`, [foreignProjectId, teamIds[1], `foreign-${suffix}`, definitionId])).rows[0].id;
    await pool.query(`insert into connector_resource_sync_states(project_id,team_id,connector_instance_id,resource_kind,status,last_succeeded_at)
      values($1,$2,$3,'device-state','idle',now())`, [primaryProjectId, teamIds[0], primaryConnectorId]);
    await pool.query(`insert into connector_capability_snapshots(project_id,team_id,connector_instance_id,capability_code,status,evidence_level,region,deployment,details_json,verified_at)
      values($1,$2,$3,'state.read','supported','live-read','cn','cn-public-cloud',$4,now())`, [primaryProjectId, teamIds[0], primaryConnectorId,
      { reason: "read_probe_succeeded", endpointId: "458069501e0", layers: { account: "supported" } }]);
    for (const userId of [userIds[0], userIds[1]]) {
      const diagnostics = await readFlightHubConnectorDiagnostics(userId, primaryProjectId, primaryConnectorId, () => pool.connect());
      assert.equal(diagnostics?.resourceWatermarks[0].resourceKind, "device-state");
      assert.equal(diagnostics?.capabilities[0].status, "supported");
    }
    assert.equal(await readFlightHubConnectorDiagnostics(userIds[2], primaryProjectId, primaryConnectorId, () => pool.connect()), null);
    assert.equal(await readFlightHubConnectorDiagnostics(userIds[0], foreignProjectId, foreignConnectorId, () => pool.connect()), null);
  } finally {
    for (const teamId of teamIds) await pool.query(`delete from teams where id=$1`, [teamId]);
    for (const userId of userIds) await pool.query(`delete from users where id=$1`, [userId]);
    await pool.end();
  }
});
