import "server-only";

import { randomUUID } from "node:crypto";

import type { PoolClient } from "pg";

import { withAuditedProjectWrite } from "@/lib/audit";
import { credentialAAD, encryptCredentialObject } from "@/lib/credential-encryption";
import { requireCurrentProjectPermission } from "@/lib/data";
import { canManageDeviceAdapters } from "@/lib/device-adapter-policy";
import { createFlightHubProjectClient } from "@/lib/dji-flighthub-client";
import {
  buildFlightHubConnectionPlan,
  FlightHubConnectionError,
  flightHubConnectionInputSchema,
  flightHubScopeFingerprint,
  revalidateSelectedFlightHubProject,
} from "@/lib/dji-flighthub-connection-core";
import { correlationId } from "@/lib/observability";
import { publishProjectEvent } from "@/lib/project-events";
import { getWebRuntimeConfig } from "@/lib/runtime-config";
import { buildFlightHubSyncRequest } from "@/lib/dji-flighthub-lifecycle-core";

type ConnectorDefinitionRow = { id: string };
type ConnectorInstanceRow = { id: string; status: string; createdAt: Date };

function isPostgresError(error: unknown, code: string) {
  return error !== null && typeof error === "object" && "code" in error && error.code === code;
}

async function persistFlightHubConnection(
  client: PoolClient,
  input: {
    projectId: number;
    teamId: number;
    token: string;
    plan: ReturnType<typeof buildFlightHubConnectionPlan>;
  }
) {
  const definition = await client.query<ConnectorDefinitionRow>(
    `select id from connector_definitions
      where connector_key=$1 and version=$2 and status='active'`,
    [input.plan.connectorKey, input.plan.connectorVersion]
  );
  if (definition.rowCount !== 1) {
    throw new FlightHubConnectionError("configuration_unavailable");
  }

  const inserted = await client.query<ConnectorInstanceRow>(
    `insert into device_adapters (
       project_id, team_id, name, adapter_type, connector_definition_id,
       vendor, protocol_version, status, config_json, capabilities_json,
       onboarding_policy, discovery_scope_json, external_scope_key
     ) values ($1,$2,$3,$4,$5,$6,$7,'connecting',$8,$9,'review',$10,$11)
     returning id, status, created_at as "createdAt"`,
    [
      input.projectId,
      input.teamId,
      input.plan.name,
      input.plan.adapterType,
      definition.rows[0].id,
      input.plan.vendor,
      input.plan.protocolVersion,
      input.plan.config,
      input.plan.capabilities,
      input.plan.discoveryScope,
      input.plan.externalScopeKey,
    ]
  );
  const connector = inserted.rows[0];
  const envelope = encryptCredentialObject(
    { token: input.token },
    getWebRuntimeConfig().authSecret,
    credentialAAD("device-adapter", connector.id, input.projectId)
  );
  await client.query(
    `update device_adapters set credential_envelope_json=$3::jsonb, last_checked_at=now(), updated_at=now()
      where id=$1 and project_id=$2`,
    [connector.id, input.projectId, envelope]
  );

  const eventId = `connector-sync:${connector.id}:${randomUUID()}`;
  await publishProjectEvent(client, {
    projectId: input.projectId,
    teamId: input.teamId,
    eventId,
    eventType: "connector.sync.requested",
    payload: buildFlightHubSyncRequest(connector.id, "initial"),
  });

  return {
    id: connector.id,
    connectorKey: input.plan.connectorKey,
    connectorVersion: input.plan.connectorVersion,
    status: connector.status,
    project: input.plan.discoveryScope,
    createdAt: connector.createdAt,
    initialSyncQueued: true,
  };
}

export async function createFlightHubConnection(
  projectId: number,
  rawInput: unknown,
  requestId?: string | null
) {
  if (!Number.isSafeInteger(projectId) || projectId <= 0) {
    throw new FlightHubConnectionError("access_denied");
  }
  const { user, access } = await requireCurrentProjectPermission(projectId, "device:configure");
  if (!canManageDeviceAdapters(access.role) || access.projectId !== projectId) {
    throw new FlightHubConnectionError("access_denied");
  }
  const input = flightHubConnectionInputSchema.parse(rawInput);
  const selectedProject = await revalidateSelectedFlightHubProject(
    createFlightHubProjectClient(),
    input.token,
    input.projectUuid
  );
  const plan = buildFlightHubConnectionPlan(selectedProject);

  try {
    return await withAuditedProjectWrite(
      {
        projectId,
        teamId: access.teamId,
        requestId: correlationId(requestId),
        actorUserId: user.id,
        action: "connector.flighthub.create",
        resourceType: "connector",
        input: {
          connectorKey: plan.connectorKey,
          externalScopeFingerprint: flightHubScopeFingerprint(plan.externalScopeKey),
        },
        policyResult: {
          permission: "device:configure",
          role: access.role,
          upstreamProjectRevalidated: true,
        },
      },
      (client) => persistFlightHubConnection(client, {
        projectId,
        teamId: access.teamId,
        token: input.token,
        plan,
      })
    );
  } catch (error) {
    if (error instanceof FlightHubConnectionError) throw error;
    if (isPostgresError(error, "23505")) {
      throw new FlightHubConnectionError("duplicate_connection");
    }
    throw new FlightHubConnectionError("configuration_unavailable");
  }
}
