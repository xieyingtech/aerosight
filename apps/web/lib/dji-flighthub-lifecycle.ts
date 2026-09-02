import "server-only";

import { randomUUID } from "node:crypto";

import type { PoolClient } from "pg";

import { withAuditedProjectWrite } from "@/lib/audit";
import { credentialAAD, encryptCredentialObject } from "@/lib/credential-encryption";
import { requireCurrentProjectPermission } from "@/lib/data";
import { canManageDeviceAdapters } from "@/lib/device-adapter-policy";
import { createFlightHubProjectClient } from "@/lib/dji-flighthub-client";
import {
  FlightHubConnectionError,
  flightHubScopeFingerprint,
  revalidateSelectedFlightHubProject,
} from "@/lib/dji-flighthub-connection-core";
import {
  assertFlightHubConnectorEnabled,
  buildFlightHubSyncRequest,
  flightHubTokenUpdateSchema,
} from "@/lib/dji-flighthub-lifecycle-core";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";
import { publishProjectEvent } from "@/lib/project-events";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

export type FlightHubConnectorRow = {
  id: string;
  name: string;
  status: string;
  projectUuid: string;
  projectName: string;
  lastErrorCode: string | null;
  lastValidatedAt: Date | null;
  lastSyncAt: Date | null;
  lastSyncStatus: string | null;
  discoveredCount: number;
  managedCount: number;
  missingCount: number;
  createdAt: Date;
  updatedAt: Date;
};

export type FlightHubDiscoveryIdentityRow = {
  id: string;
  connectorId: string;
  connectorName: string;
  externalDeviceId: string;
  externalDeviceType: string | null;
  serialNumber: string | null;
  callsign: string | null;
  parentExternalId: string | null;
  discoveryStatus: "discovered" | "managed" | "ignored" | "conflicted" | "missing";
  sourceVersion: string | null;
  deviceId: number | null;
  lastSeenAt: Date;
};

export type FlightHubSyncRunRow = {
  id: string;
  connectorId: string;
  connectorName: string;
  status: "pending" | "running" | "succeeded" | "failed" | "cancelled";
  discoveredCount: number;
  managedCount: number;
  missingCount: number;
  errorCode: string | null;
  startedAt: Date | null;
  finishedAt: Date | null;
  createdAt: Date;
};

type LockedConnector = {
  id: string;
  status: string;
  projectUuid: string;
  projectName: string;
};

async function requireFlightHubManager(projectId: number) {
  if (!Number.isSafeInteger(projectId) || projectId <= 0) {
    throw new FlightHubConnectionError("access_denied");
  }
  const scope = await requireCurrentProjectPermission(projectId, "device:configure");
  if (!canManageDeviceAdapters(scope.access.role) || scope.access.projectId !== projectId) {
    throw new FlightHubConnectionError("access_denied");
  }
  return scope;
}

async function lockFlightHubConnector(client: PoolClient, projectId: number, connectorId: string) {
  const result = await client.query<LockedConnector>(
    `select adapter.id, adapter.status,
            adapter.discovery_scope_json->>'projectUuid' as "projectUuid",
            adapter.discovery_scope_json->>'projectName' as "projectName"
       from device_adapters adapter
       join connector_definitions definition on definition.id=adapter.connector_definition_id
      where adapter.id=$1 and adapter.project_id=$2
        and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
      for update of adapter`,
    [connectorId, projectId]
  );
  if (result.rowCount !== 1) throw new FlightHubConnectionError("connector_not_found");
  return result.rows[0];
}

async function queueSync(
  client: PoolClient,
  input: { projectId: number; teamId: number; connectorId: string; trigger: "manual" | "credential-update" | "capability-probe" | "reconnect" }
) {
  await client.query("select pg_advisory_xact_lock(hashtext($1))", [`flightHub-sync:${input.connectorId}`]);
  const queued = await client.query<{ eventId: string }>(
    `select event_id as "eventId" from outbox_events
      where project_id=$1 and event_type='connector.sync.requested'
        and payload_json->>'connectorInstanceId'=$2
        and status in ('pending','processing')
      order by id limit 1`,
    [input.projectId, input.connectorId]
  );
  if (queued.rowCount === 1) {
    return { eventId: queued.rows[0].eventId, deduplicated: true };
  }

  const eventId = `connector-sync:${input.connectorId}:${randomUUID()}`;
  await publishProjectEvent(client, {
    projectId: input.projectId,
    teamId: input.teamId,
    eventId,
    eventType: "connector.sync.requested",
    payload: buildFlightHubSyncRequest(input.connectorId, input.trigger),
  });
  return { eventId, deduplicated: false };
}

export async function listFlightHubConnections(projectId: number) {
  await requireFlightHubManager(projectId);
  const result = await query<FlightHubConnectorRow>(
    `select instance.id, instance.name, instance.status,
            instance.discovery_scope_json->>'projectUuid' as "projectUuid",
            instance.discovery_scope_json->>'projectName' as "projectName",
            instance.last_health_json->>'code' as "lastErrorCode",
            instance.last_checked_at as "lastValidatedAt",
            latest.finished_at as "lastSyncAt", latest.status as "lastSyncStatus",
            coalesce(inventory.discovered_count,0)::int as "discoveredCount",
            coalesce(inventory.managed_count,0)::int as "managedCount",
            coalesce(inventory.missing_count,0)::int as "missingCount",
            instance.created_at as "createdAt", instance.updated_at as "updatedAt"
       from connector_instances instance
       left join lateral (
         select run.status, run.finished_at
           from connector_sync_runs run
          where run.project_id=instance.project_id and run.connector_instance_id=instance.id
          order by run.created_at desc limit 1
       ) latest on true
       left join lateral (
         select count(*) filter (where identity.discovery_status='discovered') as discovered_count,
                count(*) filter (where identity.discovery_status='managed') as managed_count,
                count(*) filter (where identity.discovery_status='missing') as missing_count
           from device_external_identities identity
          where identity.project_id=instance.project_id and identity.adapter_id=instance.id
       ) inventory on true
      where instance.project_id=$1 and instance.connector_key='dji.flighthub2'
      order by instance.created_at`,
    [projectId]
  );
  return result.rows;
}

export async function listFlightHubDiscoveryActivity(projectId: number) {
  await requireFlightHubManager(projectId);
  const [identities, syncRuns] = await Promise.all([
    query<FlightHubDiscoveryIdentityRow>(
      `select identity.id, identity.adapter_id as "connectorId", instance.name as "connectorName",
              identity.external_device_id as "externalDeviceId",
              identity.external_device_type as "externalDeviceType",
              identity.identity_json#>>'{attributes,serialNumber}' as "serialNumber",
              nullif(identity.identity_json#>>'{attributes,callsign}','') as callsign,
              nullif(identity.identity_json->>'parentExternalId','') as "parentExternalId",
              identity.discovery_status as "discoveryStatus", identity.source_version as "sourceVersion",
              identity.device_id as "deviceId", identity.last_seen_at as "lastSeenAt"
         from device_external_identities identity
         join connector_instances instance on instance.id=identity.adapter_id and instance.project_id=identity.project_id
        where identity.project_id=$1 and instance.connector_key='dji.flighthub2'
        order by identity.last_seen_at desc, identity.id desc limit 200`,
      [projectId]
    ),
    query<FlightHubSyncRunRow>(
      `select run.id, run.connector_instance_id as "connectorId", instance.name as "connectorName",
              run.status, run.discovered_count::int as "discoveredCount",
              run.managed_count::int as "managedCount", run.missing_count::int as "missingCount",
              run.error_code as "errorCode", run.started_at as "startedAt",
              run.finished_at as "finishedAt", run.created_at as "createdAt"
         from connector_sync_runs run
         join connector_instances instance on instance.id=run.connector_instance_id and instance.project_id=run.project_id
        where run.project_id=$1 and instance.connector_key='dji.flighthub2'
        order by run.created_at desc limit 100`,
      [projectId]
    ),
  ]);
  return { identities: identities.rows, syncRuns: syncRuns.rows };
}

export async function updateFlightHubToken(
  projectId: number,
  connectorId: string,
  rawInput: unknown,
  requestId?: string | null
) {
  const { user, access } = await requireFlightHubManager(projectId);
  const input = flightHubTokenUpdateSchema.parse(rawInput);
  const current = await query<LockedConnector>(
    `select instance.id, instance.status,
            instance.discovery_scope_json->>'projectUuid' as "projectUuid",
            instance.discovery_scope_json->>'projectName' as "projectName"
       from connector_instances instance
      where instance.id=$1 and instance.project_id=$2 and instance.connector_key='dji.flighthub2'`,
    [connectorId, projectId]
  );
  if (current.rowCount !== 1) throw new FlightHubConnectionError("connector_not_found");
  let selected;
  try {
    selected = await revalidateSelectedFlightHubProject(
      createFlightHubProjectClient(),
      input.token,
      current.rows[0].projectUuid
    );
  } catch (error) {
    const safeError = error instanceof FlightHubConnectionError
      ? error
      : new FlightHubConnectionError("upstream_error");
    await withAuditedProjectWrite(
      {
        projectId,
        teamId: access.teamId,
        requestId: correlationId(requestId),
        actorUserId: user.id,
        action: "connector.flighthub.credential.update",
        resourceType: "connector",
        resourceId: connectorId,
        input: { externalScopeFingerprint: flightHubScopeFingerprint(current.rows[0].projectUuid) },
        policyResult: {
          permission: "device:configure",
          role: access.role,
          projectRevalidated: false,
          errorCode: safeError.safeCode,
        },
      },
      async () => ({ tokenUpdated: false, errorCode: safeError.safeCode })
    );
    throw safeError;
  }

  return withAuditedProjectWrite(
    {
      projectId,
      teamId: access.teamId,
      requestId: correlationId(requestId),
      actorUserId: user.id,
      action: "connector.flighthub.credential.update",
      resourceType: "connector",
      resourceId: connectorId,
      input: { externalScopeFingerprint: flightHubScopeFingerprint(selected.uuid) },
      policyResult: { permission: "device:configure", role: access.role, projectRevalidated: true },
    },
    async (client) => {
      const locked = await lockFlightHubConnector(client, projectId, connectorId);
      if (locked.projectUuid !== selected.uuid) {
        throw new FlightHubConnectionError("project_access_changed");
      }
      const envelope = encryptCredentialObject(
        { token: input.token },
        getWebRuntimeConfig().authSecret,
        credentialAAD("device-adapter", connectorId, projectId)
      );
      await client.query(
        `update device_adapters
            set credential_envelope_json=$3::jsonb, status='connecting',
                discovery_scope_json=discovery_scope_json-'accountFingerprint',
				last_health_json='{}'::jsonb, last_checked_at=now(), updated_at=now()
          where id=$1 and project_id=$2`,
        [connectorId, projectId, envelope]
      );
      const sync = await queueSync(client, {
        projectId,
        teamId: access.teamId,
        connectorId,
        trigger: "credential-update",
      });
      return { id: connectorId, status: "connecting", tokenUpdated: true, syncQueued: true, ...sync };
    }
  );
}

export async function requestFlightHubSync(
  projectId: number,
  connectorId: string,
  requestId?: string | null
) {
  const { user, access } = await requireFlightHubManager(projectId);
  return withAuditedProjectWrite(
    {
      projectId,
      teamId: access.teamId,
      requestId: correlationId(requestId),
      actorUserId: user.id,
      action: "connector.flighthub.sync.request",
      resourceType: "connector",
      resourceId: connectorId,
      input: { trigger: "manual" },
      policyResult: { permission: "device:configure", role: access.role },
    },
    async (client) => {
      const connector = await lockFlightHubConnector(client, projectId, connectorId);
      try {
        assertFlightHubConnectorEnabled(connector.status);
      } catch {
        throw new FlightHubConnectionError("connector_disabled");
      }
      const sync = await queueSync(client, {
        projectId,
        teamId: access.teamId,
        connectorId,
        trigger: "manual",
      });
      return { id: connectorId, syncQueued: true, ...sync };
    }
  );
}

export async function requestFlightHubCapabilityProbe(
  projectId: number,
  connectorId: string,
  requestId?: string | null
) {
  const { user, access } = await requireFlightHubManager(projectId);
  return withAuditedProjectWrite(
    {
      projectId,
      teamId: access.teamId,
      requestId: correlationId(requestId),
      actorUserId: user.id,
      action: "connector.flighthub.capability.probe",
      resourceType: "connector",
      resourceId: connectorId,
      input: { upstreamMethods: ["GET"] },
      policyResult: { permission: "device:configure", role: access.role, readOnlyUpstream: true },
    },
    async (client) => {
      const connector = await lockFlightHubConnector(client, projectId, connectorId);
      try {
        assertFlightHubConnectorEnabled(connector.status);
      } catch {
        throw new FlightHubConnectionError("connector_disabled");
      }
      await client.query(
        `update connector_capability_snapshots
            set expires_at=case when verified_at<now() then now() else verified_at+interval '1 microsecond' end,
                updated_at=now()
          where project_id=$1 and connector_instance_id=$2 and evidence_level='live-read'`,
        [projectId, connectorId]
      );
      const sync = await queueSync(client, {
        projectId,
        teamId: access.teamId,
        connectorId,
        trigger: "capability-probe",
      });
      return { id: connectorId, probeQueued: true, readOnlyUpstream: true, ...sync };
    }
  );
}

export async function disconnectFlightHubConnection(
  projectId: number,
  connectorId: string,
  requestId?: string | null
) {
  const { user, access } = await requireFlightHubManager(projectId);
  return withAuditedProjectWrite(
    {
      projectId,
      teamId: access.teamId,
      requestId: correlationId(requestId),
      actorUserId: user.id,
      action: "connector.flighthub.disconnect",
      resourceType: "connector",
      resourceId: connectorId,
      input: { operation: "disable" },
      policyResult: { permission: "device:configure", role: access.role, preservesHistory: true },
    },
    async (client) => {
      await lockFlightHubConnector(client, projectId, connectorId);
      await client.query(
        `update device_adapters
            set status='disabled', lease_owner=null, lease_expires_at=null,
                last_health_json='{"ok":false,"code":"CONNECTOR_DISCONNECTED"}'::jsonb,
                updated_at=now()
          where id=$1 and project_id=$2`,
        [connectorId, projectId]
      );
      await client.query(
        `update device_connector_bindings
            set status='disabled', unbound_at=coalesce(unbound_at,now())
          where project_id=$1 and connector_instance_id=$2 and status<>'disabled'`,
        [projectId, connectorId]
      );
      await client.query(
        `update connector_sync_runs
            set status='cancelled', error_code='CONNECTOR_DISCONNECTED', finished_at=coalesce(finished_at,now())
          where project_id=$1 and connector_instance_id=$2 and status in ('pending','running')`,
        [projectId, connectorId]
      );
      await client.query(
        `update outbox_events
            set status='dead', last_error='CONNECTOR_DISCONNECTED', completed_at=now(),
                locked_by=null, locked_until=null
          where project_id=$1 and event_type='connector.sync.requested'
            and payload_json->>'connectorInstanceId'=$2 and status in ('pending','processing')`,
        [projectId, connectorId]
      );
      return { id: connectorId, status: "disabled", disconnected: true, historyPreserved: true };
    }
  );
}

export async function reconnectFlightHubConnection(
  projectId: number,
  connectorId: string,
  requestId?: string | null
) {
  const { user, access } = await requireFlightHubManager(projectId);
  return withAuditedProjectWrite(
    {
      projectId,
      teamId: access.teamId,
      requestId: correlationId(requestId),
      actorUserId: user.id,
      action: "connector.flighthub.reconnect",
      resourceType: "connector",
      resourceId: connectorId,
      input: { operation: "enable" },
      policyResult: {
        permission: "device:configure",
        role: access.role,
        credentialsUnchanged: true,
        readOnlySync: true,
      },
    },
    async (client) => {
      const connector = await lockFlightHubConnector(client, projectId, connectorId);
      if (connector.status !== "disabled") {
        throw new FlightHubConnectionError("connector_not_disabled");
      }
      await client.query(
        `update device_adapters
            set status='connecting', lease_owner=null, lease_expires_at=null,
                last_health_json='{}'::jsonb, last_checked_at=null, updated_at=now()
          where id=$1 and project_id=$2 and status='disabled'`,
        [connectorId, projectId]
      );
      const bindings = await client.query(
        `update device_connector_bindings
            set status='active', unbound_at=null
          where project_id=$1 and connector_instance_id=$2 and status='disabled'
          returning id`,
        [projectId, connectorId]
      );
      const sync = await queueSync(client, {
        projectId,
        teamId: access.teamId,
        connectorId,
        trigger: "reconnect",
      });
      return {
        id: connectorId,
        status: "connecting",
        reconnected: true,
        restoredBindingCount: bindings.rowCount ?? 0,
        syncQueued: true,
        ...sync,
      };
    }
  );
}
