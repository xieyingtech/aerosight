import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { Pool, type QueryResult, type QueryResultRow } from "pg";

import { readFlightHubModelsCore } from "./dji-flighthub-models-core.ts";

function result<T extends QueryResultRow>(rows: T[]): QueryResult<T> {
  return { command: "SELECT", rowCount: rows.length, oid: 0, fields: [], rows };
}

class ModelsClient {
  statements: string[] = [];
  released = false;
  private readonly authorized: boolean;
  private readonly canOperate: boolean;
  private readonly populated: boolean;
  constructor(authorized = true, canOperate = false, populated = false) {
    this.authorized = authorized;
    this.canOperate = canOperate;
    this.populated = populated;
  }
  async query<T extends QueryResultRow>(text: string): Promise<QueryResult<T>> {
    this.statements.push(text);
    if (text.startsWith("begin") || text === "commit" || text === "rollback") return result([]) as QueryResult<T>;
    if (text.includes("flighthub-models:access")) return result(this.authorized
      ? [{ projectId: 11, teamId: 7, role: this.canOperate ? "admin" : "member", canOperate: this.canOperate }] : []) as unknown as QueryResult<T>;
    if (text.includes("flighthub-models:connectors")) return result([
      { projectId: 11, id: "41", name: "司空连接器", status: "connected", lastCheckedAt: "2099-09-01T12:00:00Z" },
      { projectId: 22, id: "99", name: "FOREIGN_CONNECTOR_SHOULD_NOT_ESCAPE", status: "connected", lastCheckedAt: null }
    ]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-models:actions")) return result([
      { projectId: 11, connectorId: "41", action: "open-start", flagEnabled: true, capabilityVerified: true },
      { projectId: 11, connectorId: "41", action: "model-delete", flagEnabled: false, capabilityVerified: true },
      { projectId: 22, connectorId: "99", action: "open-start", flagEnabled: true, capabilityVerified: true }
    ]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-models:sync")) return result([{ projectId: 11, connectorId: "41", status: "idle",
      lastErrorCode: null, lastSucceededAt: "2099-09-01T12:00:00Z", nextAttemptAt: null }]) as unknown as QueryResult<T>;
    if (text.includes("flighthub-models:resources")) return result(this.populated ? [
      { projectId: 11, id: "101", connectorId: "41", kind: "model", status: "active", name: "传统模型",
        fileType: "model_3d", showOnMap: "true", sizeBytes: "1024", modelType: null, modelStatus: null,
        reconstructionProgress: null, errorCode: null, zipStatus: null, zipProgress: null, resourceStatus: null,
        fileCount: null, assetId: "31", assetKind: "model", assetStatus: "available", assetFailureCode: null,
        remoteUpdatedAt: "2099-09-01T11:55:00Z", lastSeenAt: "2099-09-01T12:00:00Z", remoteIdentity: "REMOTE_SHOULD_NOT_ESCAPE" },
      { projectId: 11, id: "102", connectorId: "41", kind: "model-resource", status: "active", name: null,
        fileType: null, showOnMap: null, sizeBytes: "2048", modelType: "2", modelStatus: "14",
        reconstructionProgress: "42", errorCode: "0", zipStatus: "1", zipProgress: "10", resourceStatus: null,
        fileCount: null, assetId: "32", assetKind: "model", assetStatus: "pending", assetFailureCode: null,
        remoteUpdatedAt: null, lastSeenAt: "2099-09-01T12:00:00Z" },
      { projectId: 22, id: "999", connectorId: "99", kind: "model", status: "active", name: "FOREIGN_MODEL_SHOULD_NOT_ESCAPE",
        fileType: null, showOnMap: null, sizeBytes: null, modelType: null, modelStatus: null,
        reconstructionProgress: null, errorCode: null, zipStatus: null, zipProgress: null, resourceStatus: null,
        fileCount: null, assetId: null, assetKind: null, assetStatus: null, assetFailureCode: null,
        remoteUpdatedAt: null, lastSeenAt: null }
    ] : []) as unknown as QueryResult<T>;
    if (text.includes("flighthub-models:jobs")) return result(this.populated ? [
      { projectId: 11, id: "00000000-0000-4000-8000-000000000001", connectorId: "41", jobType: "reconstruction",
        action: "open-start", status: "reconciling", progress: "42", stage: "reconstructing", attemptCount: "1",
        reconciliationCount: "3", lastErrorCode: null, assetIds: [], createdAt: "2099-09-01T11:00:00Z", updatedAt: "2099-09-01T12:00:00Z" },
      { projectId: 22, id: "00000000-0000-4000-8000-000000000099", connectorId: "99", jobType: "reconstruction",
        action: "open-start", status: "succeeded", progress: 100, stage: "completed", attemptCount: 1,
        reconciliationCount: 1, lastErrorCode: null, assetIds: [999], createdAt: null, updatedAt: null }
    ] : []) as unknown as QueryResult<T>;
    throw new Error(`unexpected query: ${text}`);
  }
  release() { this.released = true; }
}

test("member without operate permission gets a read-only empty model workspace and no available actions", async () => {
  const client = new ModelsClient(true, false, false);
  const workspace = await readFlightHubModelsCore(7, 11, async () => client as never);
  assert(workspace);
  assert.equal(workspace.access.mode, "read-only");
  assert.equal(workspace.connectors[0].actions.every((action) => !action.available), true);
  assert.deepEqual({ models: workspace.models.length, resources: workspace.resources.length, jobs: workspace.jobs.length },
    { models: 0, resources: 0, jobs: 0 });
  assert(client.released);
});

test("running model job exposes progress and pending artifact without leaking another tenant", async () => {
  const client = new ModelsClient(true, true, true);
  const workspace = await readFlightHubModelsCore(7, 11, async () => client as never);
  assert(workspace);
  assert.equal(workspace.connectors[0].actions.find((action) => action.action === "open-start")?.available, true);
  assert.equal(workspace.connectors[0].actions.find((action) => action.action === "model-delete")?.available, false);
  assert.equal(workspace.models[0].assetStatus, "available");
  assert.equal(workspace.resources[0].reconstructionProgress, 42);
  assert.deepEqual({ status: workspace.jobs[0].status, progress: workspace.jobs[0].progress, stage: workspace.jobs[0].stage },
    { status: "reconciling", progress: 42, stage: "reconstructing" });
  const serialized = JSON.stringify(workspace);
  for (const forbidden of ["FOREIGN_CONNECTOR_SHOULD_NOT_ESCAPE", "FOREIGN_MODEL_SHOULD_NOT_ESCAPE", "REMOTE_SHOULD_NOT_ESCAPE"]) {
    assert(!serialized.includes(forbidden), `model workspace leaked ${forbidden}`);
  }
});

test("unauthorized project stops before reading model projections", async () => {
  const client = new ModelsClient(false);
  assert.equal(await readFlightHubModelsCore(9, 11, async () => client as never), null);
  assert.equal(client.statements.filter((text) => text.includes("flighthub-models:")).length, 1);
  assert(client.released);
});

test("model API and page expose safe project projections and required sections", () => {
  const core = readFileSync(new URL("./dji-flighthub-models-core.ts", import.meta.url), "utf8");
  const route = readFileSync(new URL("../app/api/projects/[id]/models/route.ts", import.meta.url), "utf8");
  const page = readFileSync(new URL("../app/(app)/projects/[id]/models/page.tsx", import.meta.url), "utf8");
  for (const forbidden of ["remote_id", "remote_ids_json", "credential_envelope_json", "request_envelope_json", "storage_key"]) {
    assert(!core.includes(forbidden), `model core references secret-bearing field ${forbidden}`);
    assert(!route.includes(forbidden), `model route references secret-bearing field ${forbidden}`);
  }
  for (const heading of ["连接器与实际可用操作", "传统模型目录", "开放模型与资源", "模型作业"]) {
    assert(page.includes(`title="${heading}"`));
  }
  for (const label of ["进度", "产物", "失败原因"]) assert(page.includes(label));
});

test("Postgres model workspace returns a running job and local asset through project scope", {
  skip: !process.env.AEROSIGHT_TEST_DATABASE_URL,
}, async () => {
  const pool = new Pool({ connectionString: process.env.AEROSIGHT_TEST_DATABASE_URL });
  const suffix = Date.now();
  let userId = 0;
  let teamId = 0;
  try {
    userId = (await pool.query<{ id: number }>(`insert into users(name,email) values($1,$2) returning id`,
      ["model-reader", `model-reader-${suffix}@example.test`])).rows[0].id;
    teamId = (await pool.query<{ id: number }>(`insert into teams(name) values($1) returning id`, [`models-${suffix}`])).rows[0].id;
    await pool.query(`insert into team_members(team_id,user_id,role) values($1,$2,'owner')`, [teamId,userId]);
    const projectId = (await pool.query<{ id: number }>(`insert into projects(team_id,name,created_by_user_id)
      values($1,$2,$3) returning id`, [teamId,`models-${suffix}`,userId])).rows[0].id;
    const definitionId = (await pool.query<{ id: string }>(`select id::text from connector_definitions
      where connector_key='dji.flighthub2' and version='1.0.0'`)).rows[0].id;
    const connectorId = (await pool.query<{ id: string }>(`insert into device_adapters(project_id,team_id,name,
      adapter_type,connector_definition_id,protocol_version,status) values($1,$2,$3,'dji-flighthub2',$4,'2','connected')
      returning id::text`, [projectId,teamId,`models-${suffix}`,definitionId])).rows[0].id;
    await pool.query(`insert into project_feature_flags(project_id,flighthub_action_flags_json)
      values($1,'{"model.write":true}'::jsonb)`, [projectId]);
    await pool.query(`insert into connector_capability_snapshots(project_id,team_id,connector_instance_id,
      capability_code,status,evidence_level,region,deployment,verified_at,expires_at)
      values($1,$2,$3,'model.write','supported','field-write','cn','cn-public-cloud',now(),now()+interval '1 hour')`,
    [projectId,teamId,connectorId]);
    const assetId = (await pool.query<{ id: string }>(`insert into assets(project_id,team_id,kind,storage_key,logical_key,status)
      values($1,$2,'model',$3,$4,'pending') returning id::text`,
    [projectId,teamId,`connector/${connectorId}/model/synthetic`, `model-${suffix}`])).rows[0].id;
    await pool.query(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,
      remote_id,remote_version,status,summary_json,canonical_target_type,canonical_target_id)
      values($1,$2,$3,'model-resource',$4,'version-1','active',$5,'asset',$6)`,
    [projectId,teamId,connectorId,`model:${suffix}`, { modelType: 2, modelStatus: 14,
      reconstructionProgress: 42, sizeBytes: 2048 }, assetId]);
    await pool.query(`insert into connector_model_jobs(project_id,team_id,connector_instance_id,requested_by_user_id,
      action_kind,idempotency_key,request_digest,request_envelope_json,status,progress,stage,submit_attempt_count,
      reconciliation_count) values($1,$2,$3,$4,'open-start',$5,$6,'{}'::jsonb,'reconciling',42,'reconstructing',1,3)`,
    [projectId,teamId,connectorId,userId,`model-job-${suffix}`,"a".repeat(64)]);

    const workspace = await readFlightHubModelsCore(userId, projectId, () => pool.connect());
    assert(workspace);
    assert.equal(workspace.connectors[0].actions.find((action) => action.action === "open-start")?.available, true);
    assert.equal(workspace.resources[0].assetId, Number(assetId));
    assert.deepEqual({ status: workspace.jobs[0].status, progress: workspace.jobs[0].progress },
      { status: "reconciling", progress: 42 });
  } finally {
    if (teamId) await pool.query(`delete from teams where id=$1`, [teamId]);
    if (userId) await pool.query(`delete from users where id=$1`, [userId]);
    await pool.end();
  }
});
