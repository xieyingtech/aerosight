import "server-only";

import { randomUUID } from "node:crypto";

import { correlationId } from "@/lib/observability";
import { withAuditedProjectWrite } from "@/lib/audit";
import { credentialAAD, decryptCredentialObject, encryptCredentialObject, type CredentialEnvelope } from "@/lib/credential-encryption";
import { query } from "@/lib/db";
import {
  assertNoInlineSecrets,
  assertSupportedDeviceAdapterType,
  buildDjiConfigurationSummary,
  canManageDeviceAdapters,
  djiCredentialUpdateSchema,
  djiAdapterSetupInputSchema,
  deviceAdapterInputSchema,
  nonEmptyDjiCredentialUpdates,
  type DeviceAdapterInput,
  type DjiAdapterSetupInput
} from "@/lib/device-adapter-policy";
import { requireCurrentProjectPermission } from "@/lib/data";
import { checkDeviceNetworkConnection } from "@/lib/device-connection-check-core";
import { createDeviceEndpointProbe } from "@/lib/device-connection-probe";
import { validateDeviceNetworkProfile } from "@/lib/device-network-profile";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

type DeviceAdapterRow = {
  id: string;
  projectId: number;
  name: string;
  adapterType: "simulator" | "dji";
  vendor: string | null;
  protocolVersion: string;
  status: string;
  config: Record<string, unknown>;
  lastHealth: Record<string, unknown>;
  lastCheckedAt: Date | null;
  updatedAt: Date;
};

const adapterProjection = `id, project_id as "projectId", name, adapter_type as "adapterType", vendor,
  protocol_version as "protocolVersion", status, config_json as config,
  last_health_json as "lastHealth", last_checked_at as "lastCheckedAt", updated_at as "updatedAt"`;

async function requireAdapterManager(projectId: number) {
  const scope = await requireCurrentProjectPermission(projectId, "device:configure");
  if (!canManageDeviceAdapters(scope.access.role)) throw new Error("PROJECT_ACCESS_DENIED");
  return scope;
}

export async function listDeviceAdapters(projectId: number) {
  await requireAdapterManager(projectId);
  const result = await query<DeviceAdapterRow>(
    `select adapter.id, adapter.project_id as "projectId", adapter.name,
            adapter.adapter_type as "adapterType", adapter.vendor,
            adapter.protocol_version as "protocolVersion", adapter.status, adapter.config_json as config,
            adapter.last_health_json as "lastHealth",
            adapter.last_checked_at as "lastCheckedAt", adapter.updated_at as "updatedAt"
       from device_adapters adapter where adapter.project_id = $1 order by adapter.name`,
    [projectId]
  );
  return result.rows;
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
      input: { ...input, credentials: undefined },
      policyResult: { permission: "device:configure", role: access.role }
    },
    async (client) => {
      const result = await client.query<DeviceAdapterRow>(
        `insert into device_adapters (
           project_id, team_id, name, adapter_type, vendor, protocol_version, config_json
         ) values ($1, $2, $3, $4, $5, $6, $7)
         returning ${adapterProjection}`,
        [
          projectId, access.teamId, input.name, input.adapterType, input.vendor ?? null,
          input.protocolVersion, input.config
        ]
      );
      if (input.credentials && Object.keys(input.credentials).length > 0) {
        const envelope = encryptCredentialObject(input.credentials, getWebRuntimeConfig().authSecret,
          credentialAAD("device-adapter", result.rows[0].id, projectId));
        await client.query(`update device_adapters set credential_envelope_json=$2::jsonb where id=$1`, [result.rows[0].id, envelope]);
      }
      return result.rows[0];
    }
  );
}

export async function createDjiAdapterSetup(
  projectId: number,
  rawInput: DjiAdapterSetupInput,
  requestId?: string | null
) {
  const { user, access } = await requireAdapterManager(projectId);
  const input = djiAdapterSetupInputSchema.parse(rawInput);
  const profileInput = {
    mode: input.mode,
    mqttEndpoint: input.mqttEndpoint,
    apiPublicBaseUrl: input.apiPublicBaseUrl,
    websocketPublicUrl: input.websocketPublicUrl,
    mediaIngestBaseUrl: input.mediaIngestBaseUrl,
    mediaPlaybackBaseUrl: input.mediaPlaybackBaseUrl,
    tlsRequired: input.tlsRequired,
    mqttAnonymous: input.mqttAnonymous,
    credentialProvided: true
  };
  const validation = await validateDeviceNetworkProfile(profileInput);
  if (!validation.valid) {
    const error = new Error("NETWORK_PROFILE_INVALID");
    Object.assign(error, { issues: validation.issues });
    throw error;
  }
  const topics = input.gatewaySerials.flatMap((serial) => [
    `sys/product/${serial}/status`,
    `thing/product/${serial}/state`,
    `thing/product/${serial}/osd`,
    `thing/product/${serial}/events`,
    `thing/product/${serial}/requests`,
    `thing/product/${serial}/services_reply`
  ]);
  const clientId = `aerosight-${randomUUID()}`;
  return withAuditedProjectWrite(
    {
      projectId,
      teamId: access.teamId,
      requestId: correlationId(requestId),
      actorUserId: user.id,
      action: "device_adapter.dji_setup",
      resourceType: "device_adapter",
      input: {
        ...input, mqttUsername: undefined, mqttPassword: undefined, appId: undefined,
        appKey: undefined, appLicense: undefined, mediaPublishUser: undefined,
        mediaPublishPassword: undefined
      },
      policyResult: { permission: "device:configure", role: access.role, networkPolicy: "valid" }
    },
    async (client) => {
      const profile = await client.query<{ id: string }>(
        `insert into device_network_profiles (
           project_id, team_id, name, mode, mqtt_endpoint, api_public_base_url,
           websocket_public_url, media_ingest_base_url, media_playback_base_url,
           tls_required, status, config_json, last_validation_json
         ) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'unverified',$11,$12)
         returning id`,
        [
          projectId, access.teamId, `${input.name} Network`, input.mode, input.mqttEndpoint,
          input.apiPublicBaseUrl, input.websocketPublicUrl, input.mediaIngestBaseUrl,
          input.mediaPlaybackBaseUrl, input.tlsRequired,
          { mqttAnonymous: input.mqttAnonymous },
          { status: "unverified", policyIssues: [] }
        ]
      );
      const adapter = await client.query<DeviceAdapterRow>(
        `insert into device_adapters (
           project_id, team_id, name, adapter_type, vendor, protocol_version,
           status, config_json, network_profile_id
         ) values ($1,$2,$3,'dji','dji','cloud-api-mqtt5','connecting',$4,$5)
         returning ${adapterProjection}`,
        [
          projectId, access.teamId, input.name,
          {
            clientId, gatewaySerials: input.gatewaySerials, topics,
            djiConfiguration: { ntpServerHost: input.ntpServerHost, ntpServerPort: input.ntpServerPort }
          },
          profile.rows[0].id
        ]
      );
      const envelope = encryptCredentialObject({
        mqttUsername: input.mqttUsername, mqttPassword: input.mqttPassword,
        appId: input.appId, appKey: input.appKey, appLicense: input.appLicense,
        mediaPublishUser: input.mediaPublishUser, mediaPublishPassword: input.mediaPublishPassword
      }, getWebRuntimeConfig().authSecret, credentialAAD("device-adapter", adapter.rows[0].id, projectId));
      await client.query(`update device_adapters set credential_envelope_json=$2::jsonb where id=$1`, [adapter.rows[0].id, envelope]);
      return {
        ...adapter.rows[0],
        network: { mode: input.mode, status: "unverified" },
        configurationSummary: buildDjiConfigurationSummary(input, clientId)
      };
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

      const requestedTypeKey = typeof identity.identity.deviceTypeKey === "string"
        ? identity.identity.deviceTypeKey
        : "legacy.device";

      const device = await client.query<{ id: number }>(
        `with selected_type as (
           select id from device_types
            where status = 'active' and type_key in ($6, 'legacy.device')
            order by case when type_key = $6 then 0 else 1 end, version desc
            limit 1
         )
         insert into devices (project_id, adapter_id, device_type_id, name, type, status, metadata_json)
         select $1, $2, selected_type.id, $3, $4, 'unknown',
                jsonb_build_object('identityId', $5::bigint, 'requestedDeviceTypeKey', $6::text)
           from selected_type
         returning id`,
        [projectId, identity.adapterId, name, deviceType, identityId, requestedTypeKey]
      );
      if (!device.rows[0]) throw new Error("DEVICE_TYPE_NOT_AVAILABLE");
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
  const adapter = await query<DeviceAdapterRow & {
    networkProfileId: string | null;
    networkMode: "lan" | "public" | null;
    mqttEndpoint: string | null;
    apiPublicBaseUrl: string | null;
    websocketPublicUrl: string | null;
    mediaIngestBaseUrl: string | null;
    mediaPlaybackBaseUrl: string | null;
    tlsRequired: boolean | null;
    hasCredential: boolean;
    networkConfig: Record<string, unknown> | null;
  }>(
    `select adapter.id, adapter.project_id as "projectId", adapter.name,
            adapter.adapter_type as "adapterType", adapter.vendor,
            adapter.protocol_version as "protocolVersion", adapter.status,
            adapter.config_json as config,
            adapter.last_health_json as "lastHealth", adapter.last_checked_at as "lastCheckedAt",
            adapter.updated_at as "updatedAt", profile.id as "networkProfileId",
            profile.mode as "networkMode", profile.mqtt_endpoint as "mqttEndpoint",
            profile.api_public_base_url as "apiPublicBaseUrl",
            profile.websocket_public_url as "websocketPublicUrl",
            profile.media_ingest_base_url as "mediaIngestBaseUrl",
            profile.media_playback_base_url as "mediaPlaybackBaseUrl",
            profile.tls_required as "tlsRequired", adapter.credential_envelope_json is not null as "hasCredential",
            profile.config_json as "networkConfig"
       from device_adapters adapter
       left join device_network_profiles profile
         on profile.project_id = adapter.project_id and profile.id = adapter.network_profile_id
      where adapter.project_id = $1 and adapter.id = $2`,
    [projectId, adapterId]
  );
  const row = adapter.rows[0];
  if (!row) throw new Error("DEVICE_ADAPTER_NOT_FOUND");
  const hasCompleteProfile = row.networkProfileId && row.networkMode && row.mqttEndpoint
    && row.apiPublicBaseUrl && row.websocketPublicUrl && row.mediaIngestBaseUrl && row.mediaPlaybackBaseUrl;
  const health = row.adapterType === "simulator" && !hasCompleteProfile
    ? { ok: true, code: "SIMULATOR_READY", serverVerification: "not_applicable", deviceVerification: "pending" }
    : !hasCompleteProfile
      ? { ok: false, code: "NETWORK_PROFILE_REQUIRED", serverVerification: "failed", deviceVerification: "pending" }
      : await checkDeviceNetworkConnection({
          mode: row.networkMode!,
          mqttEndpoint: row.mqttEndpoint!,
          apiPublicBaseUrl: row.apiPublicBaseUrl!,
          websocketPublicUrl: row.websocketPublicUrl!,
          mediaIngestBaseUrl: row.mediaIngestBaseUrl!,
          mediaPlaybackBaseUrl: row.mediaPlaybackBaseUrl!,
          tlsRequired: Boolean(row.tlsRequired),
          mqttAnonymous: row.networkConfig?.mqttAnonymous === true,
          credentialProvided: row.hasCredential
        }, { probe: createDeviceEndpointProbe() });

  return withAuditedProjectWrite(
    {
      projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
      action: "device_adapter.test_connection", resourceType: "device_adapter",
      resourceId: String(adapterId), input: {}, policyResult: { permission: "device:configure" }
    },
    async (client) => {
      if (row.networkProfileId && "status" in health) {
        await client.query(
          `update device_network_profiles
              set status = $3, last_validation_json = $4,
                  last_validated_at = now(), updated_at = now()
            where project_id = $1 and id = $2`,
          [projectId, row.networkProfileId, health.status, health]
        );
      }
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

export async function updateDjiAdapterCredentials(
  projectId: number,
  adapterId: number,
  rawInput: unknown,
  requestId?: string | null
) {
  const { user, access } = await requireAdapterManager(projectId);
  const input = djiCredentialUpdateSchema.parse(rawInput);
  const updates = nonEmptyDjiCredentialUpdates(input);
  if (Object.keys(updates).length === 0) return { id: adapterId, updated: false };
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
    action: "device_adapter.credentials.update", resourceType: "device_adapter", resourceId: String(adapterId),
    input: { fieldsUpdated: Object.keys(updates).sort() }, policyResult: { permission: "device:configure" }
  }, async (client) => {
    const adapter = (await client.query<{ id: string; envelope: CredentialEnvelope | null }>(
      `select id, credential_envelope_json as envelope from device_adapters
        where project_id=$1 and id=$2 and adapter_type='dji' for update`, [projectId, adapterId]
    )).rows[0];
    if (!adapter) throw new Error("DEVICE_ADAPTER_NOT_FOUND");
    const aad = credentialAAD("device-adapter", adapter.id, projectId);
    const current = adapter.envelope
      ? decryptCredentialObject<Record<string, string>>(adapter.envelope, getWebRuntimeConfig().authSecret, aad)
      : {};
    const envelope = encryptCredentialObject({ ...current, ...updates }, getWebRuntimeConfig().authSecret, aad);
    await client.query(
      `update device_adapters set credential_envelope_json=$3::jsonb, status='connecting', updated_at=now()
        where project_id=$1 and id=$2`, [projectId, adapterId, envelope]
    );
    return { id: adapterId, updated: true };
  });
}
