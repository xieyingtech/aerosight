import { createHash } from "node:crypto";
import type { PoolClient, QueryResult, QueryResultRow } from "pg";

type GeospatialClient = Pick<PoolClient, "query" | "release">;
type GeospatialKind = "map-element" | "flight-area" | "offline-map" | "air-sense-warning";
type Freshness = "fresh" | "stale" | "missing" | "unknown";

const source = "dji-flighthub-openapi" as const;
const secretLikeText = /https?:\/\/|(?:^|[?&])(token|signature|credential|secret|x-amz-[^=]*)=|\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b/i;
const safeCodePattern = /^[A-Za-z0-9_.:-]{1,128}$/;
const catalogFreshnessMilliseconds = 24 * 60 * 60 * 1_000;
const airSenseFreshnessMilliseconds = 5 * 60 * 1_000;

type AccessRow = QueryResultRow & { projectId: number; teamId: number; role: "owner" | "admin" | "member" };
type ConnectorRow = QueryResultRow & {
  projectId: number;
  id: string;
  name: string;
  status: string;
  lastCheckedAt: Date | string | null;
};
type SyncRow = QueryResultRow & {
  projectId: number;
  connectorId: string;
  status: string;
  attemptCount: number | string;
  lastErrorCode: string | null;
  lastStartedAt: Date | string | null;
  lastSucceededAt: Date | string | null;
  nextAttemptAt: Date | string | null;
};
type ResourceRow = QueryResultRow & {
  projectId: number;
  id: string;
  connectorId: string;
  kind: GeospatialKind;
  status: string;
  remoteVersion: string | null;
  remoteUpdatedAt: Date | string | null;
  lastSeenAt: Date | string;
  missingAt: Date | string | null;
  name: string | null;
  geometry: unknown;
  coordinateReference: string | null;
  stateCode: string | null;
  display: string | null;
  areaType: string | null;
  progress: string | null;
  resultCode: string | null;
  modelCount: string | null;
  modelNames: unknown;
  warningLevel: string | null;
  deviceId: string | null;
  expiresAt: string | null;
  expired: string | null;
  issueId: number | string | null;
};

export type FlightHubGeospatialGeometry = {
  type: "Point" | "LineString" | "Polygon";
  coordinates: number[] | number[][] | number[][][];
};

function safeLabel(value: unknown, fallback: string, maximumLength = 200) {
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

function safeBoolean(value: unknown) {
  if (typeof value === "boolean") return value;
  if (value === "true" || value === "1") return true;
  if (value === "false" || value === "0") return false;
  return null;
}

function safeStringArray(value: unknown) {
  if (!Array.isArray(value)) return [];
  return value.map((item) => safeLabel(item, "", 100)).filter(Boolean).slice(0, 32);
}

function safePosition(value: unknown): number[] | null {
  if (!Array.isArray(value) || value.length < 2 || value.length > 3) return null;
  const coordinates = value.map(Number);
  if (!coordinates.every(Number.isFinite) || coordinates[0] < -180 || coordinates[0] > 180 || coordinates[1] < -90 || coordinates[1] > 90) return null;
  return coordinates;
}

function safeLine(value: unknown): number[][] | null {
  if (!Array.isArray(value) || value.length < 2 || value.length > 10_000) return null;
  const positions = value.map(safePosition);
  return positions.every((item): item is number[] => item !== null) ? positions : null;
}

function safePolygon(value: unknown): number[][][] | null {
  if (!Array.isArray(value) || value.length === 0 || value.length > 1_024) return null;
  const rings = value.map((ring) => safeLine(ring));
  if (!rings.every((item): item is number[][] => item !== null && item.length >= 4)) return null;
  for (const ring of rings) {
    const first = ring[0];
    const last = ring[ring.length - 1];
    if (first.length !== last.length || first.some((coordinate, index) => coordinate !== last[index])) return null;
  }
  return rings;
}

function safeGeometry(value: unknown): { geometry: FlightHubGeospatialGeometry | null; geometryType: string | null } {
  if (!value || typeof value !== "object") return { geometry: null, geometryType: null };
  const candidate = value as { type?: unknown; coordinates?: unknown };
  const geometryType = safeOptionalCode(candidate.type);
  if (geometryType === "Point") {
    const coordinates = safePosition(candidate.coordinates);
    return { geometry: coordinates ? { type: "Point", coordinates } : null, geometryType };
  }
  if (geometryType === "LineString" || geometryType === "Polyline") {
    const coordinates = safeLine(candidate.coordinates);
    return { geometry: coordinates ? { type: "LineString", coordinates } : null, geometryType };
  }
  if (geometryType === "Polygon") {
    const coordinates = safePolygon(candidate.coordinates);
    return { geometry: coordinates ? { type: "Polygon", coordinates } : null, geometryType };
  }
  if (geometryType === "Circle" && Array.isArray(candidate.coordinates)) {
    const center = Array.isArray(candidate.coordinates[0]) ? safePosition(candidate.coordinates[0]) : safePosition(candidate.coordinates.slice(0, 2));
    return { geometry: center ? { type: "Point", coordinates: center } : null, geometryType };
  }
  return { geometry: null, geometryType };
}

function versionFingerprint(value: unknown) {
  const normalized = typeof value === "string" ? value.trim() : "";
  if (!normalized) return null;
  return `v1-${createHash("sha256").update(normalized).digest("hex").slice(0, 12)}`;
}

function freshness(row: ResourceRow, now: Date): Freshness {
  if (row.status === "missing" || row.status === "deleted") return "missing";
  if (row.status === "failed") return "stale";
  if (row.status !== "active") return "unknown";
  if (row.kind === "air-sense-warning") {
    const expiresAt = safeTimestamp(row.expiresAt);
    if (safeBoolean(row.expired) === true || (expiresAt && new Date(expiresAt).getTime() <= now.getTime())) return "stale";
  }
  const lastSeenAt = safeTimestamp(row.lastSeenAt);
  if (!lastSeenAt) return "unknown";
  const age = Math.max(0, now.getTime() - new Date(lastSeenAt).getTime());
  const limit = row.kind === "air-sense-warning" ? airSenseFreshnessMilliseconds : catalogFreshnessMilliseconds;
  return age <= limit ? "fresh" : "stale";
}

function commonResource(row: ResourceRow, now: Date, fallbackName: string) {
  const safe = safeGeometry(row.geometry);
  return {
    id: String(row.id), connectorId: String(row.connectorId), name: safeLabel(row.name, fallbackName), source,
    status: safeCode(row.status), versionFingerprint: versionFingerprint(row.remoteVersion), freshness: freshness(row, now),
    coordinateReference: safeOptionalCode(row.coordinateReference), geometry: safe.geometry, geometryType: safe.geometryType,
    remoteUpdatedAt: safeTimestamp(row.remoteUpdatedAt), lastSeenAt: safeTimestamp(row.lastSeenAt), missingAt: safeTimestamp(row.missingAt),
  };
}

export type FlightHubGeospatialWorkspace = ReturnType<typeof presentFlightHubGeospatial>;

export function presentFlightHubGeospatial(
  projectId: number,
  access: AccessRow,
  connectors: ConnectorRow[],
  syncStates: SyncRow[],
  resourceRows: ResourceRow[],
  now = new Date()
) {
  const scopedConnectors = connectors.filter((row) => Number(row.projectId) === projectId);
  const scopedSyncStates = syncStates.filter((row) => Number(row.projectId) === projectId);
  const resources = resourceRows.filter((row) => Number(row.projectId) === projectId);
  return {
    projectId,
    access: { role: access.role, mode: "read-only" as const },
    source,
    connectors: scopedConnectors.map((row) => ({
      id: String(row.id), name: safeLabel(row.name, "司空连接器"), status: safeCode(row.status),
      lastCheckedAt: safeTimestamp(row.lastCheckedAt),
    })),
    syncStates: scopedSyncStates.map((row) => ({
      connectorId: String(row.connectorId), status: safeCode(row.status), attemptCount: safeInteger(row.attemptCount) ?? 0,
      lastErrorCode: safeOptionalCode(row.lastErrorCode), lastStartedAt: safeTimestamp(row.lastStartedAt),
      lastSucceededAt: safeTimestamp(row.lastSucceededAt), nextAttemptAt: safeTimestamp(row.nextAttemptAt),
    })),
    mapElements: resources.filter((row) => row.kind === "map-element").map((row) => ({
      ...commonResource(row, now, "司空地图标注"), stateCode: safeOptionalCode(row.stateCode), display: safeBoolean(row.display),
    })),
    flightAreas: resources.filter((row) => row.kind === "flight-area").map((row) => ({
      ...commonResource(row, now, "司空飞行区"), areaType: safeOptionalCode(row.areaType), stateCode: safeOptionalCode(row.stateCode),
    })),
    offlineMaps: resources.filter((row) => row.kind === "offline-map").map((row) => ({
      ...commonResource(row, now, "司空离线地图"), progress: safeNumber(row.progress), resultCode: safeOptionalCode(row.resultCode),
      modelCount: safeInteger(row.modelCount) ?? 0, modelNames: safeStringArray(row.modelNames), stateCode: safeOptionalCode(row.stateCode),
    })),
    airSenseWarnings: resources.filter((row) => row.kind === "air-sense-warning").map((row) => ({
      ...commonResource(row, now, "AirSense 空域目标"), warningLevel: safeInteger(row.warningLevel),
      deviceId: safeInteger(row.deviceId), issueId: safeInteger(row.issueId), expiresAt: safeTimestamp(row.expiresAt),
    })),
  };
}

async function query<T extends QueryResultRow>(client: GeospatialClient, text: string, values: unknown[] = []) {
  return client.query<T>(text, values) as Promise<QueryResult<T>>;
}

export async function readFlightHubGeospatialCore(
  userId: number,
  projectId: number,
  connect: () => Promise<GeospatialClient>
): Promise<FlightHubGeospatialWorkspace | null> {
  if (!Number.isSafeInteger(userId) || userId <= 0 || !Number.isSafeInteger(projectId) || projectId <= 0) return null;
  const client = await connect();
  try {
    await client.query("begin isolation level repeatable read read only");
    const access = (await query<AccessRow>(client, `/* flighthub-geospatial:access */
      select project.id::int as "projectId",project.team_id::int as "teamId",membership.role
        from projects project
        join team_members membership on membership.team_id=project.team_id and membership.user_id=$1
       where project.id=$2`, [userId, projectId])).rows[0];
    if (!access) {
      await client.query("commit");
      return null;
    }

    const connectors = await query<ConnectorRow>(client, `/* flighthub-geospatial:connectors */
      select adapter.project_id::int as "projectId",adapter.id::text,adapter.name,adapter.status,
             adapter.last_checked_at as "lastCheckedAt"
        from device_adapters adapter
        join connector_definitions definition on definition.id=adapter.connector_definition_id
       where adapter.project_id=$1 and adapter.team_id=$2
         and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
       order by adapter.updated_at desc limit 50`, [projectId, access.teamId]);

    const syncStates = await query<SyncRow>(client, `/* flighthub-geospatial:sync-states */
      select state.project_id::int as "projectId",state.connector_instance_id::text as "connectorId",state.status,
             state.attempt_count::int as "attemptCount",state.last_error_code as "lastErrorCode",
             state.last_started_at as "lastStartedAt",state.last_succeeded_at as "lastSucceededAt",
             state.next_attempt_at as "nextAttemptAt"
        from connector_resource_sync_states state
        join device_adapters adapter on adapter.id=state.connector_instance_id and adapter.project_id=state.project_id
       where state.project_id=$1 and state.team_id=$2 and state.resource_kind='geospatial'
       order by state.updated_at desc limit 50`, [projectId, access.teamId]);

    const resources = await query<ResourceRow>(client, `/* flighthub-geospatial:resources */
      select resource.project_id::int as "projectId",resource.id::text,
             resource.connector_instance_id::text as "connectorId",resource.resource_kind as kind,resource.status,
             resource.remote_version as "remoteVersion",resource.remote_updated_at as "remoteUpdatedAt",
             resource.last_seen_at as "lastSeenAt",resource.missing_at as "missingAt",
             resource.summary_json->>'name' as name,
             case when resource.resource_kind='air-sense-warning'
                    and jsonb_typeof(resource.summary_json->'longitude')='number'
                    and jsonb_typeof(resource.summary_json->'latitude')='number'
                  then jsonb_build_object('type','Point','coordinates',jsonb_build_array(
                    resource.summary_json->'longitude',resource.summary_json->'latitude'))
                  else resource.summary_json->'geometry' end as geometry,
             resource.summary_json->>'coordinateReference' as "coordinateReference",
             resource.summary_json->>'status' as "stateCode",resource.summary_json->>'display' as display,
             resource.summary_json->>'areaType' as "areaType",resource.summary_json->>'percent' as progress,
             resource.summary_json->>'result' as "resultCode",resource.summary_json->>'modelCount' as "modelCount",
             resource.summary_json->'modelNames' as "modelNames",resource.summary_json->>'warningLevel' as "warningLevel",
             resource.summary_json->>'deviceId' as "deviceId",resource.summary_json->>'expiresAt' as "expiresAt",
             resource.summary_json->>'expired' as expired,issue.id::int as "issueId"
        from connector_remote_resources resource
        join device_adapters adapter on adapter.id=resource.connector_instance_id and adapter.project_id=resource.project_id
        join connector_definitions definition on definition.id=adapter.connector_definition_id
        left join perception_events event on resource.resource_kind='air-sense-warning'
          and resource.canonical_target_type='perception_event' and resource.canonical_target_id=event.id::text
          and event.project_id=resource.project_id
        left join issue_links issue_link on issue_link.project_id=resource.project_id
          and issue_link.link_type='perception_event' and issue_link.target_id=event.id::text
        left join issues issue on issue.id=issue_link.issue_id and issue.project_id=resource.project_id
       where resource.project_id=$1 and resource.team_id=$2
         and resource.resource_kind in('map-element','flight-area','offline-map','air-sense-warning')
         and definition.connector_key='dji.flighthub2'
       order by coalesce(resource.remote_updated_at,resource.last_seen_at) desc,resource.id desc limit 1000`,
      [projectId, access.teamId]);

    await client.query("commit");
    return presentFlightHubGeospatial(projectId, access, connectors.rows, syncStates.rows, resources.rows);
  } catch (error) {
    await client.query("rollback").catch(() => undefined);
    throw error;
  } finally {
    client.release();
  }
}
