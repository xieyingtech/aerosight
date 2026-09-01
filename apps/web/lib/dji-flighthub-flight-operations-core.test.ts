import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { Pool, type QueryResult, type QueryResultRow } from "pg";

import { readFlightHubFlightOperationsCore } from "./dji-flighthub-flight-operations-core.ts";

function result<T extends QueryResultRow>(rows: T[]): QueryResult<T> {
  return { command: "SELECT", rowCount: rows.length, oid: 0, fields: [], rows };
}

class FlightOperationsClient {
  statements: Array<{ text: string; values: unknown[] }> = [];
  released = false;
  private readonly authorized: boolean;
  private readonly canOperate: boolean;

  constructor(authorized: boolean, canOperate: boolean) {
    this.authorized = authorized;
    this.canOperate = canOperate;
  }

  async query<T extends QueryResultRow>(text: string, values: unknown[] = []): Promise<QueryResult<T>> {
    this.statements.push({ text, values });
    if (text.startsWith("begin") || text === "commit" || text === "rollback") return result([]) as QueryResult<T>;
    if (text.includes("flighthub-flight-operations:access")) return result(this.authorized ? [{
      projectId: 11, teamId: 7, role: "member", canOperate: this.canOperate,
    }] : []) as unknown as QueryResult<T>;
    if (text.includes("flighthub-flight-operations:connectors")) return result([{
      id: "41", name: "司空连接", status: "connected", lastCheckedAt: "2026-09-01T12:00:00Z",
      actionEnabled: true, actionVerified: true, credentialEnvelope: "TOKEN_SHOULD_NOT_ESCAPE",
    }]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-flight-operations:waylines")) return result([{
      id: "101", connectorId: "41", taskId: 21, name: "河道航线", status: "active", deviceModelKey: "dock-2",
      templateTypes: ["waypoint"], payloadCount: "1", sizeBytes: "4096", remoteUpdatedAt: "2026-09-01T11:00:00Z",
      lastSeenAt: "2026-09-01T12:00:00Z", remoteId: "REMOTE_UUID_SHOULD_NOT_ESCAPE",
    }]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-flight-operations:task-runs")) return result([{
      id: "102", connectorId: "41", taskRunId: 31, taskId: 21, taskName: "河道巡检", deviceId: 51,
      deviceName: "M3TD", status: "running", stateReason: null, taskType: "wayline", mediaUploadStatus: "uploading",
      resumableStatus: "ready", breakPointResume: true, currentWaypoint: "2", totalWaypoints: "8", exceptionCount: "0",
      startedAt: "2026-09-01T11:30:00Z", finishedAt: null, remoteUpdatedAt: "2026-09-01T11:59:00Z",
      identityJson: "SN_SHOULD_NOT_ESCAPE",
    }]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-flight-operations:tracks")) return result([{
      id: 31, connectorId: "41", taskRunId: 31, taskName: "河道巡检", deviceId: 51, deviceName: "M3TD",
      pointCount: 19, firstCapturedAt: "2026-09-01T11:30:00Z", lastCapturedAt: "2026-09-01T11:59:00Z",
      longitude: 116.12345, latitude: 39.12345,
    }]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-flight-operations:media")) return result([{
      id: "103", connectorId: "41", assetId: 61, taskRunId: 31, deviceId: 51, name: "巡检照片", kind: "image",
      mimeType: "image/jpeg", status: "available", fileType: "image", suffix: "jpg", sizeBytes: "1024",
      capturedAt: "2026-09-01T11:45:00Z", createdAt: "2026-09-01T11:46:00Z", storageKey: "SIGNED_OBJECT_KEY",
    }]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-flight-operations:flight-records")) return result([{
      id: "104", connectorId: "41", assetId: 62, taskRunId: null, deviceId: null, name: "飞行记录", kind: "file",
      mimeType: "text/csv", status: "available", contentType: "track", progress: "100", fileTypes: ["csv"],
      failedReasonCode: null, capturedAt: "2026-09-01T10:00:00Z", createdAt: "2026-09-01T10:00:00Z",
      signedUrl: "https://signed.example.test/secret",
    }]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-flight-operations:alerts")) return result([{
      id: "105", connectorId: "41", kind: "ai-alert", taskRunId: 31, perceptionEventId: "01990db9-b46a-7000-8000-000000000001",
      issueId: 71, title: "司空 AI 告警 · 人员", severity: "high", status: "open", alertCount: 1, confidence: "0.91",
      hasMedia: true, occurredAt: "2026-09-01T11:50:00Z", lastSeenAt: "2026-09-01T12:00:00Z",
      rawPayload: "RAW_ALERT_SHOULD_NOT_ESCAPE",
    }]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-flight-operations:actions")) return result([{
      id: "01990db9-b46a-7000-8000-000000000002", connectorId: "41", taskRunId: 31, deviceId: 51,
      waylineResourceId: "101", targetResourceId: null, resultResourceId: "102", action: "flight-task-create",
      status: "succeeded", attemptCount: 1, reconciliationCount: 2, lastErrorCode: null,
      acceptedAt: "2026-09-01T11:31:00Z", reconciledAt: "2026-09-01T11:32:00Z", unknownAt: null,
      completedAt: "2026-09-01T11:32:00Z", createdAt: "2026-09-01T11:30:00Z", updatedAt: "2026-09-01T11:32:00Z",
      requestEnvelope: "ENCRYPTED_REQUEST_SHOULD_NOT_ESCAPE", requestDigest: "DIGEST_SHOULD_NOT_ESCAPE",
    }]) as unknown as QueryResult<T>;
    throw new Error(`unexpected query: ${text}`);
  }

  release() { this.released = true; }
}

for (const canOperate of [false, true]) {
  test(`${canOperate ? "operator" : "read-only member"} reads every redacted flight operations section`, async () => {
    const client = new FlightOperationsClient(true, canOperate);
    const operations = await readFlightHubFlightOperationsCore(canOperate ? 8 : 7, 11, async () => client as never);
    assert(operations);
    assert.equal(operations.access.mode, canOperate ? "operator" : "read-only");
    assert.equal(operations.waylines[0].taskId, 21);
    assert.equal(operations.taskRuns[0].taskRunId, 31);
    assert.equal(operations.tracks[0].pointCount, 19);
    assert.equal(operations.media[0].assetId, 61);
    assert.equal(operations.flightRecords[0].progress, 100);
    assert.equal(operations.alerts[0].issueId, 71);
    assert.equal(operations.actions[0].final, true);
    assert.equal(operations.actions[0].id, "01990db9-b46a-7000-8000-000000000002");
    assert.deepEqual(operations.connectors[0].availableActions, canOperate
      ? ["flight-task-create", "flight-task-status", "flight-task-resumption"] : []);
    const serialized = JSON.stringify(operations);
    for (const secret of ["TOKEN_SHOULD_NOT_ESCAPE", "REMOTE_UUID_SHOULD_NOT_ESCAPE", "SN_SHOULD_NOT_ESCAPE",
      "116.12345", "39.12345", "SIGNED_OBJECT_KEY", "https://signed.example.test/secret",
      "RAW_ALERT_SHOULD_NOT_ESCAPE", "ENCRYPTED_REQUEST_SHOULD_NOT_ESCAPE", "DIGEST_SHOULD_NOT_ESCAPE"]) {
      assert(!serialized.includes(secret), `flight operations leaked ${secret}`);
    }
    assert(client.released);
  });
}

test("cross-tenant project lookup stops before reading any operational projection", async () => {
  const client = new FlightOperationsClient(false, false);
  assert.equal(await readFlightHubFlightOperationsCore(9, 11, async () => client as never), null);
  assert.equal(client.statements.filter(({ text }) => text.includes("flighthub-flight-operations:")).length, 1);
  assert(client.released);
});

test("flight operations SQL and API avoid secret-bearing source columns", () => {
  const core = readFileSync(new URL("./dji-flighthub-flight-operations-core.ts", import.meta.url), "utf8");
  const route = readFileSync(new URL("../app/api/projects/[id]/flight-operations/route.ts", import.meta.url), "utf8");
  for (const forbidden of ["request_envelope_json", "request_digest", "identity_json", "storage_key", "credential_envelope_json", "remote_id"]) {
    assert(!core.includes(forbidden), `core references secret-bearing column ${forbidden}`);
    assert(!route.includes(forbidden), `route references secret-bearing column ${forbidden}`);
  }
  assert.match(core, /resource\.summary_json->>'name'/);
  assert.match(core, /count\(\*\)::int as "pointCount"/);
});

test("flight operations page keeps write entry operator-only and renders every empty-safe section", () => {
  const page = readFileSync(new URL("../app/(app)/projects/[id]/flight-operations/page.tsx", import.meta.url), "utf8");
  assert.match(page, /operations\.access\.canOperate \? <Button/);
  assert.match(page, /不提供任何写操作入口/);
  for (const heading of ["航线", "飞行任务运行", "轨迹摘要", "飞行媒体", "飞行记录", "飞行与 AI 告警", "受控写操作结果"]) {
    assert(page.includes(`title="${heading}"`), `missing flight operations section ${heading}`);
  }
});

test("Postgres flight operations authorize reader and operator while isolating another tenant", {
  skip: !process.env.AEROSIGHT_TEST_DATABASE_URL,
}, async () => {
  const pool = new Pool({ connectionString: process.env.AEROSIGHT_TEST_DATABASE_URL });
  const suffix = Date.now();
  const userIds: number[] = [];
  const teamIds: number[] = [];
  try {
    for (const label of ["reader", "operator", "outsider"]) {
      userIds.push((await pool.query<{ id: number }>(`insert into users(name,email) values($1,$2) returning id`,
        [`flight-operations-${label}`, `flight-operations-${label}-${suffix}@example.test`])).rows[0].id);
    }
    for (const label of ["primary", "foreign"]) {
      teamIds.push((await pool.query<{ id: number }>(`insert into teams(name) values($1) returning id`,
        [`flight-operations-${label}-${suffix}`])).rows[0].id);
    }
    await pool.query(`insert into team_members(team_id,user_id,role) values($1,$2,'member'),($1,$3,'member'),($4,$5,'owner')`,
      [teamIds[0], userIds[0], userIds[1], teamIds[1], userIds[2]]);
    const primaryProjectId = (await pool.query<{ id: number }>(`insert into projects(team_id,name) values($1,$2) returning id`,
      [teamIds[0], `flight-operations-primary-${suffix}`])).rows[0].id;
    const foreignProjectId = (await pool.query<{ id: number }>(`insert into projects(team_id,name) values($1,$2) returning id`,
      [teamIds[1], `flight-operations-foreign-${suffix}`])).rows[0].id;
    await pool.query(`insert into project_permissions(project_id,team_id,user_id,permission) values($1,$2,$3,'mission:operate')`,
      [primaryProjectId, teamIds[0], userIds[1]]);
    const definitionId = (await pool.query<{ id: string }>(`select id::text from connector_definitions
      where connector_key='dji.flighthub2' and version='1.0.0'`)).rows[0].id;
    const connectorId = (await pool.query<{ id: string }>(`insert into device_adapters(
      project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,last_checked_at
    ) values($1,$2,$3,'dji-flighthub2',$4,'2','connected',now()) returning id::text`,
      [primaryProjectId, teamIds[0], `primary-${suffix}`, definitionId])).rows[0].id;
    const foreignConnectorId = (await pool.query<{ id: string }>(`insert into device_adapters(
      project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status
    ) values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id::text`,
      [foreignProjectId, teamIds[1], `foreign-${suffix}`, definitionId])).rows[0].id;
    await pool.query(`insert into project_feature_flags(project_id,flighthub_action_flags_json)
      values($1,'{"flight.execute":true}')`, [primaryProjectId]);
    await pool.query(`insert into connector_capability_snapshots(
      project_id,team_id,connector_instance_id,capability_code,status,evidence_level,region,deployment,verified_at
    ) values($1,$2,$3,'flight.execute','supported','field-write','cn','test',now())`,
      [primaryProjectId, teamIds[0], connectorId]);
    const taskId = (await pool.query<{ id: number }>(`insert into tasks(project_id,team_id,name,trigger_type,script)
      values($1,$2,$3,'manual','return {}') returning id`, [primaryProjectId, teamIds[0], `巡检-${suffix}`])).rows[0].id;
    const deviceId = (await pool.query<{ id: number }>(`insert into devices(project_id,adapter_id,device_type_id,name,type,status,data_freshness)
      select $1,$2,id,'Matrice 3TD','aircraft','online','fresh' from device_types
       where type_key='dji.matrice3td' and status='active' order by version desc limit 1 returning id`,
      [primaryProjectId, connectorId])).rows[0].id;
    const runId = (await pool.query<{ id: number }>(`insert into task_runs(
      project_id,team_id,task_id,selected_device_id,trigger_source,status
    ) values($1,$2,$3,$4,'manual','running') returning id`,
      [primaryProjectId, teamIds[0], taskId, deviceId])).rows[0].id;
    const waylineId = (await pool.query<{ id: string }>(`insert into connector_remote_resources(
      project_id,team_id,connector_instance_id,resource_kind,remote_id,status,summary_json,canonical_target_type,canonical_target_id
    ) values($1,$2,$3,'wayline',$4,'active',$5,'task',$6) returning id::text`,
      [primaryProjectId, teamIds[0], connectorId, `remote-wayline-${suffix}`,
        { name: "河道航线", templateTypes: ["waypoint"], payloadCount: 1, sizeBytes: 4096 }, String(taskId)])).rows[0].id;
    const flightTaskId = (await pool.query<{ id: string }>(`insert into connector_remote_resources(
      project_id,team_id,connector_instance_id,resource_kind,remote_id,status,summary_json,canonical_target_type,canonical_target_id
    ) values($1,$2,$3,'flight-task',$4,'active',$5,'task_run',$6) returning id::text`,
      [primaryProjectId, teamIds[0], connectorId, `remote-task-${suffix}`,
        { taskType: "wayline", currentWaypoint: 2, totalWaypoints: 8, exceptionCount: 0 }, String(runId)])).rows[0].id;
    await pool.query(`insert into observations(
      project_id,team_id,adapter_id,device_id,observation_type,source_event_id,captured_at,received_at,task_run_id
    ) values($1,$2,$3,$4,'pose',$5,now(),now(),$6)`,
      [primaryProjectId, teamIds[0], connectorId, deviceId, `flight-track-${suffix}`, runId]);
    const mediaAssetId = (await pool.query<{ id: number }>(`insert into assets(
      project_id,team_id,device_id,task_run_id,kind,mime_type,storage_key,logical_key,status
    ) values($1,$2,$3,$4,'image','image/jpeg',$5,$6,'available') returning id`,
      [primaryProjectId, teamIds[0], deviceId, runId, `private/media-${suffix}`, `flight-media-${suffix}`])).rows[0].id;
    const recordAssetId = (await pool.query<{ id: number }>(`insert into assets(
      project_id,team_id,kind,mime_type,storage_key,logical_key,status
    ) values($1,$2,'file','text/csv',$3,$4,'available') returning id`,
      [primaryProjectId, teamIds[0], `private/record-${suffix}`, `flight-record-${suffix}`])).rows[0].id;
    await pool.query(`insert into connector_remote_resources(
      project_id,team_id,connector_instance_id,resource_kind,remote_id,status,summary_json,canonical_target_type,canonical_target_id
    ) values
      ($1,$2,$3,'flight-media',$4,'active',$5,'asset',$6),
      ($1,$2,$3,'flight-record',$7,'active',$8,'asset',$9),
      ($1,$2,$3,'flight-alert',$10,'active',$11,'task_run',$12),
      ($1,$2,$3,'ai-alert',$13,'active',$14,null,null)`,
      [primaryProjectId, teamIds[0], connectorId,
        `remote-media-${suffix}`, { name: "巡检照片", fileType: "image", suffix: "jpg", sizeBytes: 1024 }, String(mediaAssetId),
        `remote-record-${suffix}`, { name: "飞行记录", contentType: "track", progress: 100, fileTypes: ["csv"] }, String(recordAssetId),
        `remote-alert-${suffix}`, { taskName: "河道巡检", alertCount: 2, taskRunId: runId }, String(runId),
        `remote-ai-alert-${suffix}`, { label: "人员", confidence: 0.9, hasMedia: true, taskRunId: runId }]);
    const approvalId = (await pool.query<{ id: string }>(`insert into approval_requests(
      id,project_id,team_id,resource_type,resource_id,action,requested_by_user_id,status,expires_at
    ) values(gen_random_uuid(),$1,$2,'task_run',$3,'flight-task-create',$4,'approved',now()+interval '1 hour') returning id::text`,
      [primaryProjectId, teamIds[0], String(runId), userIds[1]])).rows[0].id;
    await pool.query(`insert into connector_action_jobs(
      project_id,team_id,connector_instance_id,task_run_id,device_id,wayline_resource_id,remote_result_resource_id,
      approval_request_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json,
      status,attempt_count,reconciliation_count,completed_at
    ) values($1,$2,$3,$4,$5,$6,$7,$8,$9,'flight-task-create',$10,$11,$12,'succeeded',1,2,now())`,
      [primaryProjectId, teamIds[0], connectorId, runId, deviceId, waylineId, flightTaskId, approvalId, userIds[1],
        `flight-action-${suffix}`, "a".repeat(64), { ciphertext: "encrypted" }]);
    await pool.query(`insert into connector_remote_resources(
      project_id,team_id,connector_instance_id,resource_kind,remote_id,status,summary_json
    ) values($1,$2,$3,'wayline',$4,'active',$5)`,
      [foreignProjectId, teamIds[1], foreignConnectorId, `foreign-wayline-${suffix}`, { name: "FOREIGN_SHOULD_NOT_ESCAPE" }]);

    const reader = await readFlightHubFlightOperationsCore(userIds[0], primaryProjectId, () => pool.connect());
    const operator = await readFlightHubFlightOperationsCore(userIds[1], primaryProjectId, () => pool.connect());
    assert.equal(reader?.access.mode, "read-only");
    assert.deepEqual(reader?.connectors[0].availableActions, []);
    assert.equal(operator?.access.mode, "operator");
    assert.deepEqual(operator?.connectors[0].availableActions,
      ["flight-task-create", "flight-task-status", "flight-task-resumption"]);
    assert.deepEqual({ waylines: operator?.waylines.length, taskRuns: operator?.taskRuns.length,
      tracks: operator?.tracks.length, media: operator?.media.length, records: operator?.flightRecords.length,
      alerts: operator?.alerts.length, actions: operator?.actions.length },
      { waylines: 1, taskRuns: 1, tracks: 1, media: 1, records: 1, alerts: 2, actions: 1 });
    assert(!JSON.stringify(operator).includes("FOREIGN_SHOULD_NOT_ESCAPE"));
    assert.equal(await readFlightHubFlightOperationsCore(userIds[2], primaryProjectId, () => pool.connect()), null);
    assert.equal(await readFlightHubFlightOperationsCore(userIds[0], foreignProjectId, () => pool.connect()), null);
  } finally {
    for (const teamId of teamIds) await pool.query(`delete from teams where id=$1`, [teamId]);
    for (const userId of userIds) await pool.query(`delete from users where id=$1`, [userId]);
    await pool.end();
  }
});
