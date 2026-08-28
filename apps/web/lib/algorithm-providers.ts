import "server-only";

import { requireEnabledAlgorithmAdapter } from "@/lib/algorithm-adapter-registry";
import { withAuditedProjectWrite } from "@/lib/audit";
import { algorithmCredentialPayload, algorithmProviderInputSchema, type AlgorithmProviderInput } from "@/lib/algorithm-provider-policy";
import { credentialAAD, encryptCredentialObject } from "@/lib/credential-encryption";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";
import { assertSafeOutboundUrl } from "@/lib/outbound-url-policy";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

type ProviderRow = {
  id: string; projectId: number; name: string; providerType: AlgorithmProviderInput["providerType"];
  baseUrl: string; authType: AlgorithmProviderInput["authType"];
  allowedHeaders: string[]; timeoutSeconds: number; concurrencyLimit: number; rateLimitPerMinute: number;
  status: string; health: Record<string, unknown>; updatedAt: Date;
};

const projection = `id, project_id as "projectId", name, provider_type as "providerType", base_url as "baseUrl",
  auth_type as "authType", allowed_headers_json as "allowedHeaders",
  timeout_seconds as "timeoutSeconds", concurrency_limit as "concurrencyLimit",
  rate_limit_per_minute as "rateLimitPerMinute", status, health_json as health, updated_at as "updatedAt"`;

export async function listAlgorithmProviders(projectId: number) {
  await requireCurrentProjectPermission(projectId, "algorithm:manage");
  return (await query<ProviderRow>(`select ${projection} from algorithm_providers where project_id = $1 order by name`, [projectId])).rows;
}

export async function createAlgorithmProvider(projectId: number, rawInput: unknown, requestId?: string | null) {
  const input = algorithmProviderInputSchema.parse(rawInput);
  const credential = algorithmCredentialPayload(input);
  if (input.authType !== "none" && !credential) throw new Error("ALGORITHM_PROVIDER_CREDENTIAL_REQUIRED");
  const { user, access } = await requireCurrentProjectPermission(projectId, "algorithm:manage");
  await assertSafeOutboundUrl(input.baseUrl, { allowedHosts: getWebRuntimeConfig().algorithmAllowedHosts });
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
    action: "algorithm_provider.create", resourceType: "algorithm_provider", input: { ...input, credential: undefined },
    policyResult: { permission: "algorithm:manage", encryptedCredential: Boolean(credential) }
  }, async (client) => {
    const provider = (await client.query<ProviderRow>(
    `insert into algorithm_providers (
       project_id, team_id, name, provider_type, base_url, auth_type, allowed_headers_json,
       timeout_seconds, concurrency_limit, rate_limit_per_minute, created_by_user_id
     ) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) returning ${projection}`,
    [projectId, access.teamId, input.name, input.providerType, input.baseUrl, input.authType,
      input.allowedHeaders, input.timeoutSeconds, input.concurrencyLimit, input.rateLimitPerMinute, user.id]
    )).rows[0];
    if (credential) {
      const envelope = encryptCredentialObject(credential, getWebRuntimeConfig().authSecret,
        credentialAAD("algorithm-provider", provider.id, projectId));
      await client.query(`update algorithm_providers set credential_envelope_json=$2::jsonb where id=$1`, [provider.id, envelope]);
    }
    return provider;
  });
}

export async function updateAlgorithmProvider(projectId: number, providerId: number, rawInput: unknown, requestId?: string | null) {
  const input = algorithmProviderInputSchema.parse(rawInput);
  const credential = algorithmCredentialPayload(input);
  const { user, access } = await requireCurrentProjectPermission(projectId, "algorithm:manage");
  await assertSafeOutboundUrl(input.baseUrl, { allowedHosts: getWebRuntimeConfig().algorithmAllowedHosts });
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
    action: "algorithm_provider.update", resourceType: "algorithm_provider", resourceId: String(providerId), input: { ...input, credential: undefined },
    policyResult: { permission: "algorithm:manage", credentialUpdated: Boolean(credential) }
  }, async (client) => {
    const current = (await client.query<{ authType: AlgorithmProviderInput["authType"]; hasCredential: boolean }>(
      `select auth_type as "authType", credential_envelope_json is not null as "hasCredential"
         from algorithm_providers where project_id=$1 and id=$2 for update`, [projectId, providerId]
    )).rows[0];
    if (!current) throw new Error("ALGORITHM_PROVIDER_NOT_FOUND");
    if (input.authType !== "none" && !credential && (!current.hasCredential || current.authType !== input.authType)) {
      throw new Error("ALGORITHM_PROVIDER_CREDENTIAL_REQUIRED");
    }
    const envelope = credential ? encryptCredentialObject(credential, getWebRuntimeConfig().authSecret,
      credentialAAD("algorithm-provider", providerId, projectId)) : null;
    const result = await client.query<ProviderRow>(
      `update algorithm_providers set name=$3, provider_type=$4, base_url=$5,
          auth_type=$6, allowed_headers_json=$7, timeout_seconds=$8, concurrency_limit=$9,
          rate_limit_per_minute=$10,
          credential_envelope_json=coalesce($11::jsonb, credential_envelope_json), updated_at=now()
        where project_id=$1 and id=$2 returning ${projection}`,
      [projectId, providerId, input.name, input.providerType, input.baseUrl,
        input.authType, input.allowedHeaders, input.timeoutSeconds, input.concurrencyLimit, input.rateLimitPerMinute, envelope]
    );
    if (!result.rows[0]) throw new Error("ALGORITHM_PROVIDER_NOT_FOUND");
    return result.rows[0];
  });
}

export async function testAlgorithmProviderEndpoint(projectId: number, providerId: number) {
  await requireCurrentProjectPermission(projectId, "algorithm:manage");
  const provider = (await query<{ baseUrl: string; providerType: AlgorithmProviderInput["providerType"] }>(
    `select base_url as "baseUrl", provider_type as "providerType" from algorithm_providers where project_id = $1 and id = $2`, [projectId, providerId]
  )).rows[0];
  if (!provider) throw new Error("ALGORITHM_PROVIDER_NOT_FOUND");
  const capability = requireEnabledAlgorithmAdapter(provider.providerType);
  const target = await assertSafeOutboundUrl(provider.baseUrl, { allowedHosts: getWebRuntimeConfig().algorithmAllowedHosts });
  return { safe: true, providerType: provider.providerType, capability, protocol: target.url.protocol, host: target.url.hostname, resolvedAddressCount: target.addresses.length };
}
