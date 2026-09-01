import type { PoolClient, QueryResult, QueryResultRow } from "pg";

type FlightOperationsClient = Pick<PoolClient, "query" | "release">;

const flightActionKinds = ["flight-task-create", "flight-task-status", "flight-task-resumption"] as const;
const secretLikeText = /https?:\/\/|(?:^|[?&])(token|signature|credential|secret|x-amz-[^=]*)=|\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b/i;
const safeCodePattern = /^[A-Za-z0-9_.:-]{1,128}$/;
const localUuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const mimePattern = /^[a-z0-9.+_-]+\/[a-z0-9.*+_-]+$/i;

type AccessRow = QueryResultRow & {
  projectId: number;
  teamId: number;
  role: "owner" | "admin" | "member";
  canOperate: boolean;
};

type ConnectorRow = QueryResultRow & {
  id: string;
  name: string;
  status: string;
  lastCheckedAt: Date | string | null;
  actionEnabled: boolean;
  actionVerified: boolean;
};

type WaylineRow = QueryResultRow & {
  id: string;
  connectorId: string;
  taskId: number | null;
  name: string | null;
  status: string;
  deviceModelKey: string | null;
  templateTypes: unknown;
  payloadCount: number | string | null;
  sizeBytes: number | string | null;
  remoteUpdatedAt: Date | string | null;
  lastSeenAt: Date | string;
};

type FlightTaskRunRow = QueryResultRow & {
  id: string;
  connectorId: string;
  taskRunId: number;
  taskId: number;
  taskName: string;
  deviceId: number | null;
  deviceName: string | null;
  status: string;
  stateReason: string | null;
  taskType: string | null;
  mediaUploadStatus: string | null;
  resumableStatus: string | null;
  breakPointResume: boolean | null;
  currentWaypoint: number | string | null;
  totalWaypoints: number | string | null;
  exceptionCount: number | string | null;
  startedAt: Date | string | null;
  finishedAt: Date | string | null;
  remoteUpdatedAt: Date | string | null;
};

type TrackRow = QueryResultRow & {
  id: number;
  connectorId: string;
  taskRunId: number;
  taskName: string;
  deviceId: number;
  deviceName: string;
  pointCount: number | string;
  firstCapturedAt: Date | string;
  lastCapturedAt: Date | string;
};

type AssetRow = QueryResultRow & {
  id: string;
  connectorId: string;
  assetId: number;
  taskRunId: number | null;
  deviceId: number | null;
  name: string | null;
  kind: string;
  mimeType: string | null;
  status: string;
  fileType: string | null;
  suffix: string | null;
  sizeBytes: number | string | null;
  contentType: string | null;
  progress: number | string | null;
  fileTypes: unknown;
  failedReasonCode: string | null;
  capturedAt: Date | string | null;
  createdAt: Date | string;
};

type AlertRow = QueryResultRow & {
  id: string;
  connectorId: string;
  kind: string;
  taskRunId: number | null;
  perceptionEventId: string | null;
  issueId: number | null;
  title: string | null;
  severity: string | null;
  status: string | null;
  alertCount: number | string | null;
  confidence: number | string | null;
  hasMedia: boolean | null;
  occurredAt: Date | string | null;
  lastSeenAt: Date | string;
};

type ActionRow = QueryResultRow & {
  id: string;
  connectorId: string;
  taskRunId: number;
  deviceId: number;
  waylineResourceId: string | null;
  targetResourceId: string | null;
  resultResourceId: string | null;
  action: string;
  status: string;
  attemptCount: number;
  reconciliationCount: number;
  lastErrorCode: string | null;
  acceptedAt: Date | string | null;
  reconciledAt: Date | string | null;
  unknownAt: Date | string | null;
  completedAt: Date | string | null;
  createdAt: Date | string;
  updatedAt: Date | string;
};

function safeLabel(value: unknown, fallback = "未命名", maximumLength = 200) {
  const normalized = typeof value === "string" ? value.trim() : "";
  if (!normalized || normalized.length > maximumLength || /[\u0000-\u001f\u007f]/.test(normalized) || secretLikeText.test(normalized)) {
    return fallback;
  }
  return normalized;
}

function safeCode(value: unknown, fallback = "unknown") {
  const normalized = typeof value === "string" ? value.trim() : "";
  return safeCodePattern.test(normalized) ? normalized : fallback;
}

function safeOptionalCode(value: unknown) {
  const normalized = typeof value === "string" ? value.trim() : "";
  return safeCodePattern.test(normalized) ? normalized : null;
}

function safeLocalUuid(value: unknown) {
  const normalized = typeof value === "string" ? value.trim() : "";
  return localUuidPattern.test(normalized) ? normalized : null;
}

function safeMime(value: unknown) {
  const normalized = typeof value === "string" ? value.trim() : "";
  return mimePattern.test(normalized) ? normalized : null;
}

function safeInteger(value: unknown) {
  const numeric = typeof value === "number" ? value : typeof value === "string" ? Number(value) : Number.NaN;
  return Number.isSafeInteger(numeric) && numeric >= 0 ? numeric : null;
}

function safeNumber(value: unknown) {
  const numeric = typeof value === "number" ? value : typeof value === "string" ? Number(value) : Number.NaN;
  return Number.isFinite(numeric) ? numeric : null;
}

function safeTimestamp(value: unknown) {
  if (!(value instanceof Date) && typeof value !== "string") return null;
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? null : date.toISOString();
}

function safeStringArray(value: unknown) {
  if (!Array.isArray(value)) return [];
  return value.map((item) => safeOptionalCode(item)).filter((item): item is string => item !== null).slice(0, 32);
}

async function query<T extends QueryResultRow>(client: FlightOperationsClient, text: string, values: unknown[] = []) {
  return client.query<T>(text, values) as Promise<QueryResult<T>>;
}

export type FlightHubFlightOperations = ReturnType<typeof presentFlightOperations>;

function presentFlightOperations(
  access: AccessRow,
  connectors: ConnectorRow[],
  waylines: WaylineRow[],
  taskRuns: FlightTaskRunRow[],
  tracks: TrackRow[],
  media: AssetRow[],
  flightRecords: AssetRow[],
  alerts: AlertRow[],
  actions: ActionRow[]
) {
  const canOperate = Boolean(access.canOperate);
  return {
    access: { mode: canOperate ? "operator" as const : "read-only" as const, canOperate },
    connectors: connectors.map((row) => {
      const actionEnabled = Boolean(row.actionEnabled);
      const actionVerified = Boolean(row.actionVerified);
      const actionReady = canOperate && row.status === "connected" && actionEnabled && actionVerified;
      return {
        id: String(row.id),
        name: safeLabel(row.name, "司空连接器"),
        status: safeCode(row.status),
        lastCheckedAt: safeTimestamp(row.lastCheckedAt),
        actionEnabled,
        actionVerified,
        actionReady,
        availableActions: actionReady ? [...flightActionKinds] : [],
      };
    }),
    waylines: waylines.map((row) => ({
      id: String(row.id), connectorId: String(row.connectorId), taskId: safeInteger(row.taskId),
      name: safeLabel(row.name, "未命名航线"), status: safeCode(row.status),
      deviceModelKey: safeOptionalCode(row.deviceModelKey), templateTypes: safeStringArray(row.templateTypes),
      payloadCount: safeInteger(row.payloadCount), sizeBytes: safeInteger(row.sizeBytes),
      remoteUpdatedAt: safeTimestamp(row.remoteUpdatedAt), lastSeenAt: safeTimestamp(row.lastSeenAt),
    })),
    taskRuns: taskRuns.map((row) => ({
      id: String(row.id), connectorId: String(row.connectorId), taskRunId: row.taskRunId, taskId: row.taskId,
      taskName: safeLabel(row.taskName, "司空飞行任务"), deviceId: safeInteger(row.deviceId),
      deviceName: row.deviceName ? safeLabel(row.deviceName, "设备") : null, status: safeCode(row.status),
      stateReason: safeOptionalCode(row.stateReason), taskType: safeOptionalCode(row.taskType),
      mediaUploadStatus: safeOptionalCode(row.mediaUploadStatus), resumableStatus: safeOptionalCode(row.resumableStatus),
      breakPointResume: typeof row.breakPointResume === "boolean" ? row.breakPointResume : null,
      currentWaypoint: safeInteger(row.currentWaypoint), totalWaypoints: safeInteger(row.totalWaypoints),
      exceptionCount: safeInteger(row.exceptionCount), startedAt: safeTimestamp(row.startedAt),
      finishedAt: safeTimestamp(row.finishedAt), remoteUpdatedAt: safeTimestamp(row.remoteUpdatedAt),
    })),
    tracks: tracks.map((row) => ({
      id: row.id, connectorId: String(row.connectorId), taskRunId: row.taskRunId,
      taskName: safeLabel(row.taskName, "司空飞行任务"), deviceId: row.deviceId,
      deviceName: safeLabel(row.deviceName, "设备"), pointCount: safeInteger(row.pointCount) ?? 0,
      firstCapturedAt: safeTimestamp(row.firstCapturedAt), lastCapturedAt: safeTimestamp(row.lastCapturedAt),
    })),
    media: media.map((row) => ({
      id: String(row.id), connectorId: String(row.connectorId), assetId: row.assetId,
      taskRunId: safeInteger(row.taskRunId), deviceId: safeInteger(row.deviceId), name: safeLabel(row.name, "飞行媒体"),
      kind: safeCode(row.kind), mimeType: safeMime(row.mimeType), status: safeCode(row.status),
      fileType: safeOptionalCode(row.fileType), suffix: safeOptionalCode(row.suffix), sizeBytes: safeInteger(row.sizeBytes),
      capturedAt: safeTimestamp(row.capturedAt), createdAt: safeTimestamp(row.createdAt),
    })),
    flightRecords: flightRecords.map((row) => ({
      id: String(row.id), connectorId: String(row.connectorId), assetId: row.assetId,
      taskRunId: safeInteger(row.taskRunId), name: safeLabel(row.name, "飞行记录"), kind: safeCode(row.kind),
      mimeType: safeMime(row.mimeType), status: safeCode(row.status), contentType: safeOptionalCode(row.contentType),
      progress: safeNumber(row.progress), fileTypes: safeStringArray(row.fileTypes),
      failedReasonCode: safeOptionalCode(row.failedReasonCode), capturedAt: safeTimestamp(row.capturedAt),
      createdAt: safeTimestamp(row.createdAt),
    })),
    alerts: alerts.map((row) => ({
      id: String(row.id), connectorId: String(row.connectorId), kind: safeCode(row.kind),
      taskRunId: safeInteger(row.taskRunId), perceptionEventId: safeLocalUuid(row.perceptionEventId),
      issueId: safeInteger(row.issueId), title: safeLabel(row.title, row.kind === "ai-alert" ? "司空 AI 告警" : "司空飞行告警"),
      severity: safeOptionalCode(row.severity), status: safeOptionalCode(row.status), alertCount: safeInteger(row.alertCount),
      confidence: safeNumber(row.confidence), hasMedia: typeof row.hasMedia === "boolean" ? row.hasMedia : false,
      occurredAt: safeTimestamp(row.occurredAt), lastSeenAt: safeTimestamp(row.lastSeenAt),
    })),
    actions: actions.map((row) => ({
      id: safeLocalUuid(row.id) ?? "", connectorId: String(row.connectorId), taskRunId: row.taskRunId,
      deviceId: row.deviceId, waylineResourceId: row.waylineResourceId ? String(row.waylineResourceId) : null,
      targetResourceId: row.targetResourceId ? String(row.targetResourceId) : null,
      resultResourceId: row.resultResourceId ? String(row.resultResourceId) : null,
      action: safeCode(row.action), status: safeCode(row.status),
      final: ["succeeded", "failed", "blocked"].includes(row.status), attemptCount: row.attemptCount,
      reconciliationCount: row.reconciliationCount, lastErrorCode: safeOptionalCode(row.lastErrorCode),
      acceptedAt: safeTimestamp(row.acceptedAt), reconciledAt: safeTimestamp(row.reconciledAt),
      unknownAt: safeTimestamp(row.unknownAt), completedAt: safeTimestamp(row.completedAt),
      createdAt: safeTimestamp(row.createdAt), updatedAt: safeTimestamp(row.updatedAt),
    })),
  };
}

export async function readFlightHubFlightOperationsCore(
  userId: number,
  projectId: number,
  connect: () => Promise<FlightOperationsClient>
): Promise<FlightHubFlightOperations | null> {
  if (!Number.isSafeInteger(userId) || userId <= 0 || !Number.isSafeInteger(projectId) || projectId <= 0) return null;
  const client = await connect();
  try {
    await client.query("begin isolation level repeatable read read only");
    const access = (await query<AccessRow>(client, `/* flighthub-flight-operations:access */
      select project.id::int as "projectId",project.team_id::int as "teamId",membership.role,
             (membership.role in('owner','admin') or exists(
               select 1 from project_permissions permission
                where permission.project_id=project.id and permission.team_id=project.team_id
                  and permission.user_id=membership.user_id and permission.permission='mission:operate'
             )) as "canOperate"
        from projects project
        join team_members membership on membership.team_id=project.team_id and membership.user_id=$1
       where project.id=$2`, [userId, projectId])).rows[0];
    if (!access) {
      await client.query("commit");
      return null;
    }

    const connectors = await query<ConnectorRow>(client, `/* flighthub-flight-operations:connectors */
      select adapter.id::text,adapter.name,adapter.status,adapter.last_checked_at as "lastCheckedAt",
             coalesce(flags.flighthub_action_flags_json @> '{"flight.execute":true}'::jsonb,false) as "actionEnabled",
             exists(select 1 from connector_capability_snapshots capability
               where capability.project_id=adapter.project_id and capability.connector_instance_id=adapter.id
                 and capability.capability_code='flight.execute' and capability.status='supported'
                 and capability.evidence_level='field-write'
                 and (capability.expires_at is null or capability.expires_at>now())) as "actionVerified"
        from device_adapters adapter
        join connector_definitions definition on definition.id=adapter.connector_definition_id
        left join project_feature_flags flags on flags.project_id=adapter.project_id
       where adapter.project_id=$1 and adapter.team_id=$2
         and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
       order by adapter.updated_at desc limit 50`, [projectId, access.teamId]);

    const waylines = await query<WaylineRow>(client, `/* flighthub-flight-operations:waylines */
      select resource.id::text,resource.connector_instance_id::text as "connectorId",task.id::int as "taskId",
             resource.summary_json->>'name' as name,resource.status,
             resource.summary_json->>'deviceModelKey' as "deviceModelKey",
             resource.summary_json->'templateTypes' as "templateTypes",
             resource.summary_json->>'payloadCount' as "payloadCount",
             resource.summary_json->>'sizeBytes' as "sizeBytes",
             resource.remote_updated_at as "remoteUpdatedAt",resource.last_seen_at as "lastSeenAt"
        from connector_remote_resources resource
        join device_adapters adapter on adapter.id=resource.connector_instance_id and adapter.project_id=resource.project_id
        join connector_definitions definition on definition.id=adapter.connector_definition_id
        left join tasks task on resource.canonical_target_type='task'
          and resource.canonical_target_id=task.id::text and task.project_id=resource.project_id
       where resource.project_id=$1 and resource.team_id=$2 and resource.resource_kind='wayline'
         and definition.connector_key='dji.flighthub2'
       order by resource.remote_updated_at desc nulls last,resource.id desc limit 250`, [projectId, access.teamId]);

    const taskRuns = await query<FlightTaskRunRow>(client, `/* flighthub-flight-operations:task-runs */
      select resource.id::text,resource.connector_instance_id::text as "connectorId",run.id::int as "taskRunId",
             task.id::int as "taskId",task.name as "taskName",device.id::int as "deviceId",device.name as "deviceName",
             run.status,run.state_reason as "stateReason",resource.summary_json->>'taskType' as "taskType",
             resource.summary_json->>'mediaUploadStatus' as "mediaUploadStatus",
             resource.summary_json->>'resumableStatus' as "resumableStatus",
             case when resource.summary_json ? 'breakPointResume' then (resource.summary_json->>'breakPointResume')::boolean end as "breakPointResume",
             resource.summary_json->>'currentWaypoint' as "currentWaypoint",
             resource.summary_json->>'totalWaypoints' as "totalWaypoints",
             resource.summary_json->>'exceptionCount' as "exceptionCount",
             run.started_at as "startedAt",run.finished_at as "finishedAt",resource.remote_updated_at as "remoteUpdatedAt"
        from connector_remote_resources resource
        join task_runs run on resource.canonical_target_type='task_run'
          and resource.canonical_target_id=run.id::text and run.project_id=resource.project_id
        join tasks task on task.id=run.task_id and task.project_id=run.project_id
        left join devices device on device.id=run.selected_device_id and device.project_id=run.project_id
       where resource.project_id=$1 and resource.team_id=$2 and resource.resource_kind='flight-task'
       order by coalesce(resource.remote_updated_at,run.created_at) desc limit 250`, [projectId, access.teamId]);

    const tracks = await query<TrackRow>(client, `/* flighthub-flight-operations:tracks */
      select run.id::int as id,observation.adapter_id::text as "connectorId",run.id::int as "taskRunId",
             task.name as "taskName",device.id::int as "deviceId",device.name as "deviceName",
             count(*)::int as "pointCount",min(observation.captured_at) as "firstCapturedAt",
             max(observation.captured_at) as "lastCapturedAt"
        from observations observation
        join task_runs run on run.id=observation.task_run_id and run.project_id=observation.project_id
        join tasks task on task.id=run.task_id and task.project_id=run.project_id
        join devices device on device.id=observation.device_id and device.project_id=observation.project_id
        join device_adapters adapter on adapter.id=observation.adapter_id and adapter.project_id=observation.project_id
        join connector_definitions definition on definition.id=adapter.connector_definition_id
       where observation.project_id=$1 and observation.team_id=$2 and observation.task_run_id is not null
         and definition.connector_key='dji.flighthub2'
       group by run.id,observation.adapter_id,task.name,device.id,device.name
       order by max(observation.captured_at) desc limit 250`, [projectId, access.teamId]);

    const assetQuery = (kind: "flight-media" | "flight-record", marker: string) => query<AssetRow>(client, `/* flighthub-flight-operations:${marker} */
      select resource.id::text,resource.connector_instance_id::text as "connectorId",asset.id::int as "assetId",
             asset.task_run_id::int as "taskRunId",asset.device_id::int as "deviceId",resource.summary_json->>'name' as name,
             asset.kind,asset.mime_type as "mimeType",asset.status,
             resource.summary_json->>'fileType' as "fileType",resource.summary_json->>'suffix' as suffix,
             coalesce(resource.summary_json->>'sizeBytes',asset.size_bytes::text) as "sizeBytes",
             resource.summary_json->>'contentType' as "contentType",resource.summary_json->>'progress' as progress,
             resource.summary_json->'fileTypes' as "fileTypes",resource.summary_json->>'failedReasonCode' as "failedReasonCode",
             asset.captured_at as "capturedAt",asset.created_at as "createdAt"
        from connector_remote_resources resource
        join assets asset on resource.canonical_target_type='asset'
          and resource.canonical_target_id=asset.id::text and asset.project_id=resource.project_id
       where resource.project_id=$1 and resource.team_id=$2 and resource.resource_kind=$3
       order by coalesce(asset.captured_at,asset.created_at) desc limit 250`, [projectId, access.teamId, kind]);
    const media = await assetQuery("flight-media", "media");
    const flightRecords = await assetQuery("flight-record", "flight-records");

    const alerts = await query<AlertRow>(client, `/* flighthub-flight-operations:alerts */
      select resource.id::text,resource.connector_instance_id::text as "connectorId",resource.resource_kind as kind,
             coalesce(run.id,(resource.summary_json->>'taskRunId')::int)::int as "taskRunId",
             event.id::text as "perceptionEventId",issue.id::int as "issueId",
             coalesce(event.title,issue.title,resource.summary_json->>'taskName',resource.summary_json->>'label') as title,
             event.severity,event.status,resource.summary_json->>'alertCount' as "alertCount",
             resource.summary_json->>'confidence' as confidence,
             case when resource.summary_json ? 'hasMedia' then (resource.summary_json->>'hasMedia')::boolean end as "hasMedia",
             coalesce(resource.remote_updated_at,event.last_detected_at) as "occurredAt",resource.last_seen_at as "lastSeenAt"
        from connector_remote_resources resource
        left join task_runs run on resource.canonical_target_type='task_run'
          and resource.canonical_target_id=run.id::text and run.project_id=resource.project_id
        left join perception_events event on resource.canonical_target_type='perception_event'
          and resource.canonical_target_id=event.id::text and event.project_id=resource.project_id
        left join issue_links issue_link on issue_link.project_id=resource.project_id
          and issue_link.link_type='perception_event' and issue_link.target_id=event.id::text
        left join issues issue on issue.id=issue_link.issue_id and issue.project_id=resource.project_id
       where resource.project_id=$1 and resource.team_id=$2 and resource.resource_kind in('flight-alert','ai-alert')
       order by coalesce(resource.remote_updated_at,resource.last_seen_at) desc limit 250`, [projectId, access.teamId]);

    const actions = await query<ActionRow>(client, `/* flighthub-flight-operations:actions */
      select job.id::text,job.connector_instance_id::text as "connectorId",job.task_run_id::int as "taskRunId",
             job.device_id::int as "deviceId",job.wayline_resource_id::text as "waylineResourceId",
             job.target_resource_id::text as "targetResourceId",job.remote_result_resource_id::text as "resultResourceId",
             job.action_kind as action,job.status,job.attempt_count::int as "attemptCount",
             job.reconciliation_count::int as "reconciliationCount",job.last_error_code as "lastErrorCode",
             job.accepted_at as "acceptedAt",job.reconciled_at as "reconciledAt",job.unknown_at as "unknownAt",
             job.completed_at as "completedAt",job.created_at as "createdAt",job.updated_at as "updatedAt"
        from connector_action_jobs job
       where job.project_id=$1 and job.team_id=$2
       order by job.updated_at desc limit 250`, [projectId, access.teamId]);

    await client.query("commit");
    return presentFlightOperations(access, connectors.rows, waylines.rows, taskRuns.rows, tracks.rows,
      media.rows, flightRecords.rows, alerts.rows, actions.rows);
  } catch (error) {
    await client.query("rollback").catch(() => undefined);
    throw error;
  } finally {
    client.release();
  }
}
