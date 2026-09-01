import type { PoolClient, QueryResult, QueryResultRow } from "pg";

const capabilityStatuses = new Set(["supported", "empty", "forbidden", "not_applicable", "unverified", "degraded", "failed"]);
const safeCodePattern = /^[A-Za-z0-9_.:-]{1,128}$/;

type DiagnosticClient = Pick<PoolClient, "query" | "release">;

type ConnectorAccessRow = QueryResultRow & {
  id: string;
  name: string;
  status: string;
  lastErrorCode: string | null;
  lastCheckedAt: Date | string | null;
};

type ResourceWatermarkRow = QueryResultRow & {
  resourceKind: string;
  status: string;
  attemptCount: number;
  lastErrorCode: string | null;
  lastStartedAt: Date | string | null;
  lastSucceededAt: Date | string | null;
  nextAttemptAt: Date | string | null;
};

type CapabilityDiagnosticRow = QueryResultRow & {
  capabilityCode: string;
  status: string;
  evidenceLevel: string;
  region: string;
  deployment: string;
  deviceModel: string | null;
  firmwareVersion: string | null;
  reason: string | null;
  endpointId: string | null;
  layers: unknown;
  verifiedAt: Date | string;
  expiresAt: Date | string | null;
  expired: boolean;
};

export type FlightHubConnectorDiagnostics = {
  connector: ConnectorAccessRow;
  resourceWatermarks: ResourceWatermarkRow[];
  capabilities: Array<Omit<CapabilityDiagnosticRow, "layers" | "expired"> & {
    layers: Record<string, string>;
  }>;
};

function safeCode(value: unknown, fallback: string | null = null) {
  const normalized = typeof value === "string" ? value.trim() : "";
  return safeCodePattern.test(normalized) ? normalized : fallback;
}

function safeCapabilityStatus(value: unknown) {
  const normalized = typeof value === "string" ? value : "";
  return capabilityStatuses.has(normalized) ? normalized : "failed";
}

function safeLayers(value: unknown) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  const source = value as Record<string, unknown>;
  const result: Record<string, string> = {};
  for (const key of ["contract", "deployment", "account", "implementation", "acceptance"]) {
    if (key in source) result[key] = safeCapabilityStatus(source[key]);
  }
  return result;
}

async function query<T extends QueryResultRow>(client: DiagnosticClient, text: string, values: unknown[] = []) {
  return client.query<T>(text, values) as Promise<QueryResult<T>>;
}

export async function readFlightHubConnectorDiagnostics(
  userId: number,
  projectId: number,
  connectorId: string,
  connect: () => Promise<DiagnosticClient>
): Promise<FlightHubConnectorDiagnostics | null> {
  if (!Number.isSafeInteger(userId) || userId <= 0 || !Number.isSafeInteger(projectId) || projectId <= 0 || !/^\d+$/.test(connectorId)) {
    return null;
  }
  const client = await connect();
  try {
    await client.query("begin isolation level repeatable read read only");
    const connector = (await query<ConnectorAccessRow>(client, `/* flighthub-diagnostics:access */
      select adapter.id::text,adapter.name,adapter.status,
             adapter.last_health_json->>'code' as "lastErrorCode",adapter.last_checked_at as "lastCheckedAt"
        from device_adapters adapter
        join connector_definitions definition on definition.id=adapter.connector_definition_id
        join projects project on project.id=adapter.project_id and project.team_id=adapter.team_id
        join team_members membership on membership.team_id=project.team_id and membership.user_id=$1
       where adapter.project_id=$2 and adapter.id=$3
         and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'`,
      [userId, projectId, connectorId])).rows[0];
    if (!connector) {
      await client.query("commit");
      return null;
    }
    const watermarks = await query<ResourceWatermarkRow>(client, `/* flighthub-diagnostics:watermarks */
        select resource_kind as "resourceKind",status,attempt_count::int as "attemptCount",
               last_error_code as "lastErrorCode",last_started_at as "lastStartedAt",
               last_succeeded_at as "lastSucceededAt",next_attempt_at as "nextAttemptAt"
          from connector_resource_sync_states
         where project_id=$1 and connector_instance_id=$2
         order by resource_kind`, [projectId, connectorId]);
    const capabilities = await query<CapabilityDiagnosticRow>(client, `/* flighthub-diagnostics:capabilities */
        select capability_code as "capabilityCode",status,evidence_level as "evidenceLevel",region,deployment,
               device_model as "deviceModel",firmware_version as "firmwareVersion",
               details_json->>'reason' as reason,details_json->>'endpointId' as "endpointId",
               details_json->'layers' as layers,verified_at as "verifiedAt",expires_at as "expiresAt",
               (expires_at is not null and expires_at<=now()) as expired
          from connector_capability_snapshots
         where project_id=$1 and connector_instance_id=$2
         order by capability_code,verified_at desc
         limit 500`, [projectId, connectorId]);
    await client.query("commit");
    return {
      connector: {
        id: connector.id,
        name: connector.name,
        status: connector.status,
        lastErrorCode: safeCode(connector.lastErrorCode),
        lastCheckedAt: connector.lastCheckedAt,
      },
      resourceWatermarks: watermarks.rows.map((row) => ({
        resourceKind: safeCode(row.resourceKind, "unknown")!,
        status: safeCode(row.status, "failed")!,
        attemptCount: row.attemptCount,
        lastErrorCode: safeCode(row.lastErrorCode),
        lastStartedAt: row.lastStartedAt,
        lastSucceededAt: row.lastSucceededAt,
        nextAttemptAt: row.nextAttemptAt,
      })),
      capabilities: capabilities.rows.map((row) => ({
        capabilityCode: safeCode(row.capabilityCode, "unknown")!,
        status: row.expired ? "unverified" : safeCapabilityStatus(row.status),
        evidenceLevel: safeCode(row.evidenceLevel, "documented")!,
        region: safeCode(row.region, "unknown")!,
        deployment: safeCode(row.deployment, "unknown")!,
        deviceModel: safeCode(row.deviceModel),
        firmwareVersion: safeCode(row.firmwareVersion),
        reason: row.expired ? "evidence_expired" : safeCode(row.reason, "diagnostic_unavailable"),
        endpointId: safeCode(row.endpointId),
        layers: safeLayers(row.layers),
        verifiedAt: row.verifiedAt,
        expiresAt: row.expiresAt,
      })),
    };
  } catch (error) {
    await client.query("rollback").catch(() => undefined);
    throw error;
  } finally {
    client.release();
  }
}
