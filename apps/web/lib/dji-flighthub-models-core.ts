import type { PoolClient, QueryResult, QueryResultRow } from "pg";

type ModelsClient = Pick<PoolClient, "query" | "release">;
type AccessRow = { projectId: number; teamId: number; role: string; canOperate: boolean };
type ConnectorRow = { projectId: number; id: string; name: string; status: string; lastCheckedAt: unknown };
type ActionRow = { projectId: number; connectorId: string; action: string; flagEnabled: boolean; capabilityVerified: boolean };
type SyncRow = { projectId: number; connectorId: string; status: string; lastErrorCode: string | null;
  lastSucceededAt: unknown; nextAttemptAt: unknown };
type ResourceRow = { projectId: number; id: string; connectorId: string; kind: string; status: string;
  name: string | null; fileType: string | null; showOnMap: unknown; sizeBytes: unknown; modelType: unknown;
  modelStatus: unknown; reconstructionProgress: unknown; errorCode: unknown; zipStatus: unknown; zipProgress: unknown;
  resourceStatus: unknown; fileCount: unknown; assetId: string | null; assetKind: string | null;
  assetStatus: string | null; assetFailureCode: string | null; remoteUpdatedAt: unknown; lastSeenAt: unknown };
type JobRow = { projectId: number; id: string; connectorId: string; jobType: string; action: string; status: string;
  progress: unknown; stage: string | null; attemptCount: unknown; reconciliationCount: unknown;
  lastErrorCode: string | null; assetIds: unknown; createdAt: unknown; updatedAt: unknown };

const source = "dji-flighthub-openapi";
const safeCodePattern = /^[a-z0-9][a-z0-9._:-]{0,127}$/i;
function safeCode(value: unknown, fallback = "unknown") {
  return typeof value === "string" && safeCodePattern.test(value) ? value : fallback;
}
function safeText(value: unknown, fallback = "—") {
  if (typeof value !== "string") return fallback;
  const normalized = value.trim();
  return normalized && normalized.length <= 256 ? normalized : fallback;
}
function safeOptionalCode(value: unknown) {
  return typeof value === "string" && safeCodePattern.test(value) ? value : null;
}
function safeInteger(value: unknown) {
  const parsed = typeof value === "number" ? value : typeof value === "string" ? Number(value) : Number.NaN;
  return Number.isSafeInteger(parsed) ? parsed : null;
}
function safeBoolean(value: unknown) {
  return value === true || value === "true" || value === 1 || value === "1";
}
function safeTimestamp(value: unknown) {
  if (value === null || value === undefined) return null;
  const date = value instanceof Date ? value : new Date(String(value));
  return Number.isNaN(date.valueOf()) ? null : date.toISOString();
}
function safeLocalIds(value: unknown) {
  if (!Array.isArray(value)) return [];
  return value.map((item) => safeInteger(item)).filter((item): item is number => item !== null && item > 0);
}

export function presentFlightHubModels(projectId: number, access: AccessRow, connectors: ConnectorRow[], actions: ActionRow[],
  syncRows: SyncRow[], resources: ResourceRow[], jobs: JobRow[]) {
  const scopedConnectors = connectors.filter((row) => Number(row.projectId) === projectId);
  const connectorIDs = new Set(scopedConnectors.map((row) => row.id));
  const actualActions = actions.filter((row) => Number(row.projectId) === projectId && connectorIDs.has(row.connectorId));
  return {
    projectId, source,
    access: { role: safeCode(access.role), canOperate: Boolean(access.canOperate), mode: access.canOperate ? "operator" : "read-only" },
    connectors: scopedConnectors.map((row) => ({ id: row.id, name: safeText(row.name, "司空连接器"),
      status: safeCode(row.status), lastCheckedAt: safeTimestamp(row.lastCheckedAt),
      actions: actualActions.filter((item) => item.connectorId === row.id).map((item) => ({ action: safeCode(item.action),
        available: Boolean(access.canOperate) && new Set(["connecting", "connected", "degraded"]).has(row.status)
          && Boolean(item.flagEnabled) && Boolean(item.capabilityVerified),
        flagEnabled: Boolean(item.flagEnabled), capabilityVerified: Boolean(item.capabilityVerified) })) })),
    syncStates: syncRows.filter((row) => Number(row.projectId) === projectId && connectorIDs.has(row.connectorId)).map((row) => ({
      connectorId: row.connectorId, status: safeCode(row.status), lastErrorCode: safeOptionalCode(row.lastErrorCode),
      lastSucceededAt: safeTimestamp(row.lastSucceededAt), nextAttemptAt: safeTimestamp(row.nextAttemptAt) })),
    models: resources.filter((row) => Number(row.projectId) === projectId && connectorIDs.has(row.connectorId) && row.kind === "model")
      .map((row) => ({ id: row.id, connectorId: row.connectorId, name: safeText(row.name, "司空模型"),
        status: safeCode(row.status), fileType: safeOptionalCode(row.fileType), showOnMap: safeBoolean(row.showOnMap),
        sizeBytes: safeInteger(row.sizeBytes), assetId: safeInteger(row.assetId), assetStatus: safeOptionalCode(row.assetStatus),
        assetFailureCode: safeOptionalCode(row.assetFailureCode), remoteUpdatedAt: safeTimestamp(row.remoteUpdatedAt),
        lastSeenAt: safeTimestamp(row.lastSeenAt), source })),
    resources: resources.filter((row) => Number(row.projectId) === projectId && connectorIDs.has(row.connectorId)
      && row.kind === "model-resource").map((row) => ({ id: row.id, connectorId: row.connectorId,
        status: safeCode(row.status), modelType: safeInteger(row.modelType), modelStatus: safeInteger(row.modelStatus),
        reconstructionProgress: safeInteger(row.reconstructionProgress), errorCode: safeInteger(row.errorCode),
        zipStatus: safeInteger(row.zipStatus), zipProgress: safeInteger(row.zipProgress),
        resourceStatus: safeInteger(row.resourceStatus), fileCount: safeInteger(row.fileCount), sizeBytes: safeInteger(row.sizeBytes),
        assetId: safeInteger(row.assetId), assetKind: safeOptionalCode(row.assetKind), assetStatus: safeOptionalCode(row.assetStatus),
        assetFailureCode: safeOptionalCode(row.assetFailureCode), lastSeenAt: safeTimestamp(row.lastSeenAt), source })),
    jobs: jobs.filter((row) => Number(row.projectId) === projectId && connectorIDs.has(row.connectorId)).map((row) => ({
      id: row.id, connectorId: row.connectorId, jobType: safeCode(row.jobType), action: safeCode(row.action),
      status: safeCode(row.status), progress: safeInteger(row.progress), stage: safeOptionalCode(row.stage),
      attemptCount: safeInteger(row.attemptCount) ?? 0, reconciliationCount: safeInteger(row.reconciliationCount) ?? 0,
      lastErrorCode: safeOptionalCode(row.lastErrorCode), assetIds: safeLocalIds(row.assetIds),
      createdAt: safeTimestamp(row.createdAt), updatedAt: safeTimestamp(row.updatedAt) }))
  };
}

async function query<T extends QueryResultRow>(client: ModelsClient, text: string, values: unknown[] = []) {
  return client.query<T>(text, values) as Promise<QueryResult<T>>;
}

export async function readFlightHubModelsCore(userId: number, projectId: number,
  connect: () => Promise<ModelsClient>) {
  if (!Number.isSafeInteger(userId) || userId <= 0 || !Number.isSafeInteger(projectId) || projectId <= 0) return null;
  const client = await connect();
  try {
    await client.query("begin isolation level repeatable read read only");
    const access = (await query<AccessRow>(client, `/* flighthub-models:access */
      select project.id::int as "projectId",project.team_id::int as "teamId",membership.role,
        (membership.role in('owner','admin') or exists(select 1 from project_permissions permission
          where permission.project_id=project.id and permission.team_id=project.team_id
            and permission.user_id=$1 and permission.permission='mission:operate')) as "canOperate"
      from projects project join team_members membership on membership.team_id=project.team_id and membership.user_id=$1
      where project.id=$2`, [userId,projectId])).rows[0];
    if (!access) { await client.query("commit"); return null; }
    const connectors = await query<ConnectorRow>(client, `/* flighthub-models:connectors */
      select adapter.project_id::int as "projectId",adapter.id::text,adapter.name,adapter.status,
        adapter.last_checked_at as "lastCheckedAt" from device_adapters adapter
      join connector_definitions definition on definition.id=adapter.connector_definition_id
      where adapter.project_id=$1 and adapter.team_id=$2 and definition.connector_key='dji.flighthub2'`, [projectId,access.teamId]);
    const actions = await query<ActionRow>(client, `/* flighthub-models:actions */
      select adapter.project_id::int as "projectId",adapter.id::text as "connectorId",policy.action,
        coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(policy.flag,true),false) as "flagEnabled",
        exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
          and capability.connector_instance_id=adapter.id and capability.capability_code=policy.capability
          and capability.status='supported' and capability.evidence_level='field-write'
          and (capability.expires_at is null or capability.expires_at>now())
          and capability.device_model is null and capability.firmware_version is null) as "capabilityVerified"
      from device_adapters adapter cross join (values
        ('traditional-create','model.write','model.write'),('open-start','model.write','model.write'),
        ('open-stop','model.write','model.write'),('open-upload','model.write','model.write'),
        ('model-delete','model.delete','flighthub.model.delete'),
        ('model-resource-delete','model.resource.delete','flighthub.model-resource.delete')
      ) policy(action,capability,flag)
      left join project_feature_flags flags on flags.project_id=adapter.project_id
      join connector_definitions definition on definition.id=adapter.connector_definition_id
      where adapter.project_id=$1 and adapter.team_id=$2 and definition.connector_key='dji.flighthub2'`, [projectId,access.teamId]);
    const syncRows = await query<SyncRow>(client, `/* flighthub-models:sync */
      select state.project_id::int as "projectId",state.connector_instance_id::text as "connectorId",state.status,
        state.last_error_code as "lastErrorCode",state.last_succeeded_at as "lastSucceededAt",state.next_attempt_at as "nextAttemptAt"
      from connector_resource_sync_states state where state.project_id=$1 and state.team_id=$2 and state.resource_kind='models'`,
    [projectId,access.teamId]);
    const resources = await query<ResourceRow>(client, `/* flighthub-models:resources */
      select resource.project_id::int as "projectId",resource.id::text,resource.connector_instance_id::text as "connectorId",
        resource.resource_kind as kind,resource.status,resource.summary_json->>'name' as name,
        resource.summary_json->>'fileType' as "fileType",resource.summary_json->>'showOnMap' as "showOnMap",
        resource.summary_json->>'sizeBytes' as "sizeBytes",resource.summary_json->>'modelType' as "modelType",
        resource.summary_json->>'modelStatus' as "modelStatus",
        resource.summary_json->>'reconstructionProgress' as "reconstructionProgress",
        resource.summary_json->>'errorCode' as "errorCode",resource.summary_json->>'zipStatus' as "zipStatus",
        resource.summary_json->>'zipProgress' as "zipProgress",resource.summary_json->>'resourceStatus' as "resourceStatus",
        resource.summary_json->>'fileCount' as "fileCount",asset.id::text as "assetId",asset.kind as "assetKind",
        asset.status as "assetStatus",asset.failure_code as "assetFailureCode",
        resource.remote_updated_at as "remoteUpdatedAt",resource.last_seen_at as "lastSeenAt"
      from connector_remote_resources resource left join assets asset on resource.canonical_target_type='asset'
        and asset.project_id=resource.project_id and asset.id::text=resource.canonical_target_id
      where resource.project_id=$1 and resource.team_id=$2 and resource.resource_kind in('model','model-resource')
      order by resource.last_seen_at desc,resource.id desc`, [projectId,access.teamId]);
    const jobs = await query<JobRow>(client, `/* flighthub-models:jobs */
      select job.project_id::int as "projectId",job.id::text,job.connector_instance_id::text as "connectorId",
        'reconstruction'::text as "jobType",job.action_kind as action,job.status,job.progress,job.stage,
        job.submit_attempt_count as "attemptCount",job.reconciliation_count as "reconciliationCount",
        job.last_error_code as "lastErrorCode",job.asset_ids_json as "assetIds",job.created_at as "createdAt",job.updated_at as "updatedAt"
      from connector_model_jobs job where job.project_id=$1 and job.team_id=$2
      union all select upload.project_id::int,upload.id::text,upload.connector_instance_id::text,'upload',
        'open-upload',upload.status,null,null,upload.callback_attempt_count,upload.reconciliation_count,
        upload.last_error_code,case when upload.asset_id is null then '[]'::jsonb else jsonb_build_array(upload.asset_id) end,
        upload.created_at,upload.updated_at from connector_open_model_uploads upload where upload.project_id=$1 and upload.team_id=$2
      union all select deletion.project_id::int,deletion.id::text,deletion.connector_instance_id::text,'deletion',
        deletion.action_kind,deletion.status,null,null,deletion.attempt_count,deletion.reconciliation_count,
        deletion.last_error_code,'[]'::jsonb,deletion.created_at,deletion.updated_at
        from connector_model_delete_jobs deletion where deletion.project_id=$1 and deletion.team_id=$2
      order by "updatedAt" desc`, [projectId,access.teamId]);
    await client.query("commit");
    return presentFlightHubModels(projectId, access, connectors.rows, actions.rows, syncRows.rows, resources.rows, jobs.rows);
  } catch (error) {
    try { await client.query("rollback"); } catch { /* keep original error */ }
    throw error;
  } finally { client.release(); }
}
