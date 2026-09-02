import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { Pool, type QueryResult, type QueryResultRow } from "pg";

import { readFlightHubGeospatialCore } from "./dji-flighthub-geospatial-core.ts";

function result<T extends QueryResultRow>(rows: T[]): QueryResult<T> {
  return { command: "SELECT", rowCount: rows.length, oid: 0, fields: [], rows };
}

class GeospatialClient {
  statements: Array<{ text: string; values: unknown[] }> = [];
  released = false;
  private readonly authorized: boolean;
  constructor(authorized: boolean) { this.authorized = authorized; }

  async query<T extends QueryResultRow>(text: string, values: unknown[] = []): Promise<QueryResult<T>> {
    this.statements.push({ text, values });
    if (text.startsWith("begin") || text === "commit" || text === "rollback") return result([]) as QueryResult<T>;
    if (text.includes("flighthub-geospatial:access")) return result(this.authorized ? [{ projectId: 11, teamId: 7, role: "member" }] : []) as unknown as QueryResult<T>;
    if (text.includes("flighthub-geospatial:connectors")) return result([
      { projectId: 11, id: "41", name: "司空连接", status: "connected", lastCheckedAt: "2099-09-01T12:00:00Z", credentialEnvelope: "TOKEN_SHOULD_NOT_ESCAPE" },
      { projectId: 22, id: "99", name: "FOREIGN_CONNECTOR_SHOULD_NOT_ESCAPE", status: "connected", lastCheckedAt: null },
    ]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-geospatial:sync-states")) return result([
      { projectId: 11, connectorId: "41", status: "idle", attemptCount: 0, lastErrorCode: null, lastStartedAt: "2099-09-01T11:59:00Z", lastSucceededAt: "2099-09-01T12:00:00Z", nextAttemptAt: null },
      { projectId: 22, connectorId: "99", status: "idle", attemptCount: 0, lastErrorCode: null, lastStartedAt: null, lastSucceededAt: null, nextAttemptAt: null },
    ]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-geospatial:resources")) return result([
      { projectId: 11, id: "101", connectorId: "41", kind: "map-element", status: "active", remoteVersion: "MAP_VERSION_SHOULD_NOT_ESCAPE", remoteUpdatedAt: "2099-09-01T11:55:00Z", lastSeenAt: "2099-09-01T12:00:00Z", missingAt: null, name: "施工点", geometry: { type: "Point", coordinates: [116.3, 39.9] }, coordinateReference: "unverified", stateCode: "1", display: "true", areaType: null, progress: null, resultCode: null, modelCount: null, modelNames: null, warningLevel: null, deviceId: null, expiresAt: null, expired: null, issueId: null, remoteId: "REMOTE_MAP_UUID_SHOULD_NOT_ESCAPE" },
      { projectId: 11, id: "102", connectorId: "41", kind: "flight-area", status: "active", remoteVersion: "AREA_HASH_SHOULD_NOT_ESCAPE", remoteUpdatedAt: "2099-09-01T11:50:00Z", lastSeenAt: "2099-09-01T12:00:00Z", missingAt: null, name: "飞行区 A", geometry: { type: "Polygon", coordinates: [[[116.2, 39.8], [116.4, 39.8], [116.4, 40], [116.2, 39.8]]] }, coordinateReference: "unverified", stateCode: "enabled", display: null, areaType: "custom", progress: null, resultCode: null, modelCount: null, modelNames: null, warningLevel: null, deviceId: null, expiresAt: null, expired: null, issueId: null },
      { projectId: 11, id: "103", connectorId: "41", kind: "offline-map", status: "active", remoteVersion: "OFFLINE_DIGEST_SHOULD_NOT_ESCAPE", remoteUpdatedAt: "2099-09-01T11:45:00Z", lastSeenAt: "2099-09-01T12:00:00Z", missingAt: null, name: null, geometry: null, coordinateReference: null, stateCode: "ready", display: null, areaType: null, progress: "100", resultCode: "0", modelCount: "2", modelNames: ["模型甲", "模型乙"], warningLevel: null, deviceId: null, expiresAt: null, expired: null, issueId: null, signedUrl: "https://signed.example.test/secret" },
      { projectId: 11, id: "104", connectorId: "41", kind: "air-sense-warning", status: "active", remoteVersion: "AIRSENSE_DIGEST_SHOULD_NOT_ESCAPE", remoteUpdatedAt: "2099-09-01T11:59:00Z", lastSeenAt: "2099-09-01T12:00:00Z", missingAt: null, name: null, geometry: { type: "Point", coordinates: [116.5, 39.7] }, coordinateReference: "unverified", stateCode: null, display: null, areaType: null, progress: null, resultCode: null, modelCount: null, modelNames: null, warningLevel: "3", deviceId: "51", expiresAt: "2099-09-01T12:05:00Z", expired: "false", issueId: "71", icao: "ICAO_SHOULD_NOT_ESCAPE", deviceSerial: "SN_SHOULD_NOT_ESCAPE" },
      { projectId: 22, id: "999", connectorId: "99", kind: "flight-area", status: "active", remoteVersion: "foreign", remoteUpdatedAt: null, lastSeenAt: "2099-09-01T12:00:00Z", missingAt: null, name: "FOREIGN_RESOURCE_SHOULD_NOT_ESCAPE", geometry: { type: "Point", coordinates: [0, 0] }, coordinateReference: null, stateCode: null, display: null, areaType: null, progress: null, resultCode: null, modelCount: null, modelNames: null, warningLevel: null, deviceId: null, expiresAt: null, expired: null, issueId: null },
    ]) as unknown as QueryResult<T>;
    throw new Error(`unexpected query: ${text}`);
  }

  release() { this.released = true; }
}

test("authorized member reads all four project-scoped geospatial projections with safe versions", async () => {
  const client = new GeospatialClient(true);
  const workspace = await readFlightHubGeospatialCore(7, 11, async () => client as never);
  assert(workspace);
  assert.equal(workspace.source, "dji-flighthub-openapi");
  assert.deepEqual({ map: workspace.mapElements.length, area: workspace.flightAreas.length, offline: workspace.offlineMaps.length, airSense: workspace.airSenseWarnings.length }, { map: 1, area: 1, offline: 1, airSense: 1 });
  assert.match(workspace.mapElements[0].versionFingerprint ?? "", /^v1-[0-9a-f]{12}$/);
  assert.equal(workspace.mapElements[0].geometry?.type, "Point");
  assert.equal(workspace.flightAreas[0].geometry?.type, "Polygon");
  assert.deepEqual(workspace.offlineMaps[0].modelNames, ["模型甲", "模型乙"]);
  assert.equal(workspace.airSenseWarnings[0].issueId, 71);
  const serialized = JSON.stringify(workspace);
  for (const forbidden of ["TOKEN_SHOULD_NOT_ESCAPE", "FOREIGN_CONNECTOR_SHOULD_NOT_ESCAPE", "FOREIGN_RESOURCE_SHOULD_NOT_ESCAPE", "MAP_VERSION_SHOULD_NOT_ESCAPE", "AREA_HASH_SHOULD_NOT_ESCAPE", "OFFLINE_DIGEST_SHOULD_NOT_ESCAPE", "AIRSENSE_DIGEST_SHOULD_NOT_ESCAPE", "REMOTE_MAP_UUID_SHOULD_NOT_ESCAPE", "https://signed.example.test/secret", "ICAO_SHOULD_NOT_ESCAPE", "SN_SHOULD_NOT_ESCAPE"]) {
    assert(!serialized.includes(forbidden), `geospatial workspace leaked ${forbidden}`);
  }
  assert(client.released);
});

test("cross-tenant project lookup stops before reading geospatial projections", async () => {
  const client = new GeospatialClient(false);
  assert.equal(await readFlightHubGeospatialCore(9, 11, async () => client as never), null);
  assert.equal(client.statements.filter(({ text }) => text.includes("flighthub-geospatial:")).length, 1);
  assert(client.released);
});

test("geospatial SQL and API do not select remote identity or secret-bearing fields", () => {
  const core = readFileSync(new URL("./dji-flighthub-geospatial-core.ts", import.meta.url), "utf8");
  const route = readFileSync(new URL("../app/api/projects/[id]/geospatial/route.ts", import.meta.url), "utf8");
  for (const forbidden of ["remote_id", "credential_envelope_json", "identity_json", "storage_key", "downloadUrl", "signedUrl", "icao", "deviceSerial"]) {
    assert(!core.includes(forbidden), `core references secret-bearing field ${forbidden}`);
    assert(!route.includes(forbidden), `route references secret-bearing field ${forbidden}`);
  }
  assert.match(core, /resource\.summary_json->'geometry'/);
  assert.match(core, /membership\.team_id=project\.team_id/);
});

test("geospatial page renders map and source, version and freshness for all four domains", () => {
  const page = readFileSync(new URL("../app/(app)/projects/[id]/geospatial/page.tsx", import.meta.url), "utf8");
  assert.match(page, /FlightHubGeospatialMap/);
  for (const heading of ["空域地图", "司空标注", "飞行区", "离线地图", "AirSense"]) assert(page.includes(`title="${heading}"`));
  for (const label of ["来源", "版本", "新鲜度"]) assert(page.includes(`label: "${label}"`));
});

test("Postgres geospatial workspace isolates another project end to end", {
  skip: !process.env.AEROSIGHT_TEST_DATABASE_URL,
}, async () => {
  const pool = new Pool({ connectionString: process.env.AEROSIGHT_TEST_DATABASE_URL });
  const suffix = Date.now();
  const userIds: number[] = [];
  const teamIds: number[] = [];
  try {
    for (const label of ["reader", "outsider"]) userIds.push((await pool.query<{ id: number }>(
      `insert into users(name,email) values($1,$2) returning id`, [`geospatial-${label}`, `geospatial-${label}-${suffix}@example.test`])).rows[0].id);
    for (const label of ["primary", "foreign"]) teamIds.push((await pool.query<{ id: number }>(
      `insert into teams(name) values($1) returning id`, [`geospatial-${label}-${suffix}`])).rows[0].id);
    await pool.query(`insert into team_members(team_id,user_id,role) values($1,$2,'member'),($3,$4,'owner')`, [teamIds[0], userIds[0], teamIds[1], userIds[1]]);
    const primaryProjectId = (await pool.query<{ id: number }>(`insert into projects(team_id,name) values($1,$2) returning id`, [teamIds[0], `geospatial-primary-${suffix}`])).rows[0].id;
    const foreignProjectId = (await pool.query<{ id: number }>(`insert into projects(team_id,name) values($1,$2) returning id`, [teamIds[1], `geospatial-foreign-${suffix}`])).rows[0].id;
    const definitionId = (await pool.query<{ id: string }>(`select id::text from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`)).rows[0].id;
    const connectorId = (await pool.query<{ id: string }>(`insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,last_checked_at) values($1,$2,$3,'dji-flighthub2',$4,'2','connected',now()) returning id::text`, [primaryProjectId, teamIds[0], `primary-${suffix}`, definitionId])).rows[0].id;
    const foreignConnectorId = (await pool.query<{ id: string }>(`insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status) values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id::text`, [foreignProjectId, teamIds[1], `foreign-${suffix}`, definitionId])).rows[0].id;
    await pool.query(`insert into connector_resource_sync_states(project_id,team_id,connector_instance_id,resource_kind,status,last_started_at,last_succeeded_at) values($1,$2,$3,'geospatial','idle',now(),now())`, [primaryProjectId, teamIds[0], connectorId]);
    const kinds = ["map-element", "flight-area", "offline-map", "air-sense-warning"];
    const summaries = [
      { name: "项目标注", geometry: { type: "Point", coordinates: [116.3, 39.9] }, coordinateReference: "unverified", display: 1 },
      { name: "项目飞行区", geometry: { type: "Polygon", coordinates: [[[116.2, 39.8], [116.4, 39.8], [116.4, 40], [116.2, 39.8]]] }, coordinateReference: "unverified", areaType: "custom" },
      { status: "ready", result: 0, percent: 100, modelCount: 1, modelNames: ["项目模型"] },
      { warningLevel: 2, latitude: 39.7, longitude: 116.5, deviceId: 51, capturedAt: new Date().toISOString(), expiresAt: new Date(Date.now() + 300_000).toISOString(), expired: false, coordinateReference: "unverified" },
    ];
    for (let index = 0; index < kinds.length; index++) await pool.query(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id,remote_version,remote_updated_at,status,summary_json) values($1,$2,$3,$4,$5,$6,now(),'active',$7)`, [primaryProjectId, teamIds[0], connectorId, kinds[index], `primary-${index}-${suffix}`, `version-${index}`, summaries[index]]);
    await pool.query(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id,remote_version,status,summary_json) values($1,$2,$3,'flight-area',$4,'foreign','active',$5)`, [foreignProjectId, teamIds[1], foreignConnectorId, `foreign-${suffix}`, { name: "FOREIGN_SHOULD_NOT_ESCAPE", geometry: { type: "Point", coordinates: [0, 0] } }]);

    const workspace = await readFlightHubGeospatialCore(userIds[0], primaryProjectId, () => pool.connect());
    assert.deepEqual({ map: workspace?.mapElements.length, area: workspace?.flightAreas.length, offline: workspace?.offlineMaps.length, airSense: workspace?.airSenseWarnings.length }, { map: 1, area: 1, offline: 1, airSense: 1 });
    assert(!JSON.stringify(workspace).includes("FOREIGN_SHOULD_NOT_ESCAPE"));
    assert.equal(await readFlightHubGeospatialCore(userIds[1], primaryProjectId, () => pool.connect()), null);
    assert.equal(await readFlightHubGeospatialCore(userIds[0], foreignProjectId, () => pool.connect()), null);
  } finally {
    for (const teamId of teamIds) await pool.query(`delete from teams where id=$1`, [teamId]);
    for (const userId of userIds) await pool.query(`delete from users where id=$1`, [userId]);
    await pool.end();
  }
});
