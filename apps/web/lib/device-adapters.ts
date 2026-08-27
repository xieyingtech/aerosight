import "server-only";

import { correlationId } from "@/lib/observability";
import { withAuditedProjectWrite } from "@/lib/audit";
import { query } from "@/lib/db";
import {
  assertNoInlineSecrets,
  assertSupportedDeviceAdapterType,
  canManageDeviceAdapters,
  deviceAdapterInputSchema,
  publicDeviceAdapter,
  type DeviceAdapterInput
} from "@/lib/device-adapter-policy";
import { requireCurrentProjectPermission } from "@/lib/data";

type DeviceAdapterRow = {
  id: string;
  projectId: number;
  name: string;
  adapterType: "simulator" | "dji";
  vendor: string | null;
  protocolVersion: string;
  status: string;
  secretRef: string | null;
  config: Record<string, unknown>;
  lastHealth: Record<string, unknown>;
  lastCheckedAt: Date | null;
  updatedAt: Date;
};

async function requireAdapterManager(projectId: number) {
  const scope = await requireCurrentProjectPermission(projectId, "device:configure");
  if (!canManageDeviceAdapters(scope.access.role)) throw new Error("PROJECT_ACCESS_DENIED");
  return scope;
}

export async function listDeviceAdapters(projectId: number) {
  await requireAdapterManager(projectId);
  const result = await query<DeviceAdapterRow>(
    `select id, project_id as "projectId", name, adapter_type as "adapterType", vendor,
            protocol_version as "protocolVersion", status, secret_ref as "secretRef",
            config_json as config, last_health_json as "lastHealth",
            last_checked_at as "lastCheckedAt", updated_at as "updatedAt"
       from device_adapters where project_id = $1 order by name`,
    [projectId]
  );
  return result.rows.map(publicDeviceAdapter);
}

export async function createDeviceAdapter(
  projectId: number,
  rawInput: DeviceAdapterInput,
  requestId?: string | null
) {
  const { user, access } = await requireAdapterManager(projectId);
  const input = deviceAdapterInputSchema.parse(rawInput);
  assertSupportedDeviceAdapterType(input.adapterType);
  assertNoInlineSecrets(input.config);
  return withAuditedProjectWrite(
    {
      projectId,
      teamId: access.teamId,
      requestId: correlationId(requestId),
      actorUserId: user.id,
      action: "device_adapter.create",
      resourceType: "device_adapter",
      input: { ...input, secretRef: input.secretRef ? "[SECRET_REF]" : undefined },
      policyResult: { permission: "device:configure", role: access.role }
    },
    async (client) => {
      const result = await client.query<DeviceAdapterRow>(
        `insert into device_adapters (
           project_id, team_id, name, adapter_type, vendor, protocol_version, secret_ref, config_json
         ) values ($1, $2, $3, $4, $5, $6, $7, $8)
         returning id, project_id as "projectId", name, adapter_type as "adapterType", vendor,
                   protocol_version as "protocolVersion", status, secret_ref as "secretRef",
                   config_json as config, last_health_json as "lastHealth",
                   last_checked_at as "lastCheckedAt", updated_at as "updatedAt"`,
        [
          projectId, access.teamId, input.name, input.adapterType, input.vendor ?? null,
          input.protocolVersion, input.secretRef ?? null, input.config
        ]
      );
      return publicDeviceAdapter(result.rows[0]);
    }
  );
}

export async function bindDiscoveredDevice(
  projectId: number,
  identityId: number,
  input: { name: string; deviceType: string },
  requestId?: string | null
) {
  const { user, access } = await requireAdapterManager(projectId);
  const name = input.name.trim();
  const deviceType = input.deviceType.trim();
  if (!name || !["drone", "dock", "ground_robot", "fixed_sensor", "other"].includes(deviceType)) {
    throw new Error("INVALID_DEVICE_BINDING");
  }

  return withAuditedProjectWrite(
    {
      projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
      action: "device_identity.bind", resourceType: "device_external_identity",
      resourceId: String(identityId), input: { name, deviceType },
      policyResult: { permission: "device:configure", role: access.role }
    },
    async (client) => {
      const identityResult = await client.query<{
        id: string; adapterId: string; deviceId: number | null; identity: Record<string, unknown>;
      }>(
        `select id, adapter_id as "adapterId", device_id as "deviceId", identity_json as identity
           from device_external_identities
          where project_id = $1 and id = $2 for update`,
        [projectId, identityId]
      );
      const identity = identityResult.rows[0];
      if (!identity) throw new Error("DEVICE_IDENTITY_NOT_FOUND");
      if (identity.deviceId) return { deviceId: identity.deviceId, replayed: true };

      const device = await client.query<{ id: number }>(
        `insert into devices (project_id, adapter_id, name, type, status, metadata_json)
         values ($1, $2, $3, $4, 'unknown', jsonb_build_object('identityId', $5::bigint))
         returning id`,
        [projectId, identity.adapterId, name, deviceType, identityId]
      );
      await client.query(
        `update device_external_identities set device_id = $3, bound_at = now()
          where project_id = $1 and id = $2`,
        [projectId, identityId, device.rows[0].id]
      );

      const capabilities = Array.isArray(identity.identity.capabilities)
        ? identity.identity.capabilities.filter((value): value is string => typeof value === "string")
        : [];
      for (const capability of capabilities) {
        await client.query(
          `insert into device_capabilities (
             device_id, project_id, capability_code, declared_by_adapter_id
           ) values ($1, $2, $3, $4)
           on conflict (device_id, capability_code) do update
             set version_number = device_capabilities.version_number + 1,
                 declared_by_adapter_id = excluded.declared_by_adapter_id,
                 updated_at = now()`,
          [device.rows[0].id, projectId, capability, identity.adapterId]
        );
      }
      return { deviceId: device.rows[0].id, replayed: false };
    }
  );
}

export async function testDeviceAdapterConnection(
  projectId: number,
  adapterId: number,
  requestId?: string | null
) {
  const { user, access } = await requireAdapterManager(projectId);
  const adapter = await query<DeviceAdapterRow>(
    `select id, project_id as "projectId", name, adapter_type as "adapterType", vendor,
            protocol_version as "protocolVersion", status, secret_ref as "secretRef",
            config_json as config, last_health_json as "lastHealth",
            last_checked_at as "lastCheckedAt", updated_at as "updatedAt"
       from device_adapters where project_id = $1 and id = $2`,
    [projectId, adapterId]
  );
  const row = adapter.rows[0];
  if (!row) throw new Error("DEVICE_ADAPTER_NOT_FOUND");
  const health = row.adapterType === "simulator"
    ? { ok: true, code: "SIMULATOR_READY" }
    : { ok: false, code: "CONNECTION_TEST_NOT_IMPLEMENTED" };

  return withAuditedProjectWrite(
    {
      projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
      action: "device_adapter.test_connection", resourceType: "device_adapter",
      resourceId: String(adapterId), input: {}, policyResult: { permission: "device:configure" }
    },
    async (client) => {
      await client.query(
        `update device_adapters
            set last_health_json = $3, last_checked_at = now(), updated_at = now()
          where project_id = $1 and id = $2`,
        [projectId, adapterId, health]
      );
      return health;
    }
  );
}

export async function setDeviceAdapterEnabled(
  projectId: number,
  adapterId: number,
  enabled: boolean,
  requestId?: string | null
) {
  const { user, access } = await requireAdapterManager(projectId);
  return withAuditedProjectWrite(
    {
      projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
      action: "device_adapter.set_enabled", resourceType: "device_adapter",
      resourceId: String(adapterId), input: { enabled }, policyResult: { permission: "device:configure" }
    },
    async (client) => {
      const result = await client.query<{ id: string; status: string }>(
        `update device_adapters set status = $3, updated_at = now()
          where project_id = $1 and id = $2 returning id, status`,
        [projectId, adapterId, enabled ? "connecting" : "disabled"]
      );
      if (!result.rows[0]) throw new Error("DEVICE_ADAPTER_NOT_FOUND");
      return result.rows[0];
    }
  );
}
