import "server-only";

import { withAuditedPlatformWrite } from "@/lib/audit";
import { aiProviderInputSchema, normalizedAIAPIKey, type AIProviderInput } from "@/lib/ai-provider-policy";
import { credentialAAD, decryptCredentialObject, encryptCredentialObject, type CredentialEnvelope } from "@/lib/credential-encryption";
import { requireAdmin } from "@/lib/data";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";
import { assertSafeOutboundUrl } from "@/lib/outbound-url-policy";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

export type AIProviderView = {
  id: string; name: string; providerType: "openai"; baseUrl: string | null; modelId: string;
  enabled: boolean; isDefault: boolean; status: string; health: Record<string, unknown>;
  lastTestedAt: Date | null; updatedAt: Date;
};

const projection = `id, name, provider_type as "providerType", base_url as "baseUrl", model_id as "modelId",
  enabled, is_default as "isDefault", status, health_json as health,
  last_tested_at as "lastTestedAt", updated_at as "updatedAt"`;

async function validateBaseURL(baseUrl?: string) {
  if (!baseUrl) return;
  let hostname: string;
  try { hostname = new URL(baseUrl).hostname; } catch { throw new Error("AI_PROVIDER_BASE_URL_INVALID"); }
  await assertSafeOutboundUrl(baseUrl, { allowedHosts: [hostname] });
}

export async function listAIProviders() {
  await requireAdmin();
  return (await query<AIProviderView>(`select ${projection} from ai_providers order by name`)).rows;
}

export async function createAIProvider(rawInput: unknown, requestId?: string | null) {
  const input = aiProviderInputSchema.parse(rawInput);
  const apiKey = normalizedAIAPIKey(input);
  if (!apiKey) throw new Error("AI_PROVIDER_API_KEY_REQUIRED");
  await validateBaseURL(input.baseUrl);
  const user = await requireAdmin();
  return withAuditedPlatformWrite({
    requestId: correlationId(requestId), actorUserId: user.id, action: "ai_provider.create",
    resourceType: "ai_provider", input: { ...input, apiKey: undefined }
  }, async (client) => {
    if (input.isDefault) await client.query(`update ai_providers set is_default=false, updated_at=now() where is_default`);
    const created = (await client.query<AIProviderView>(
      `insert into ai_providers (
         name, provider_type, base_url, model_id, credential_envelope_json,
         enabled, is_default, created_by_user_id, updated_by_user_id
       ) values ($1,$2,nullif($3,''),$4,'{}'::jsonb,$5,$6,$7,$7) returning ${projection}`,
      [input.name, input.providerType, input.baseUrl ?? "", input.modelId, input.enabled, input.isDefault, user.id]
    )).rows[0];
    const envelope = encryptCredentialObject({ apiKey }, getWebRuntimeConfig().authSecret,
      credentialAAD("ai-provider", created.id));
    await client.query(`update ai_providers set credential_envelope_json=$2::jsonb where id=$1`, [created.id, envelope]);
    return created;
  });
}

export async function updateAIProvider(providerId: number, rawInput: unknown, requestId?: string | null) {
  const input = aiProviderInputSchema.parse(rawInput);
  const apiKey = normalizedAIAPIKey(input);
  await validateBaseURL(input.baseUrl);
  const user = await requireAdmin();
  return withAuditedPlatformWrite({
    requestId: correlationId(requestId), actorUserId: user.id, action: "ai_provider.update",
    resourceType: "ai_provider", resourceId: String(providerId), input: { ...input, apiKey: undefined }
  }, async (client) => {
    const exists = (await client.query<{ id: string }>(`select id from ai_providers where id=$1 for update`, [providerId])).rows[0];
    if (!exists) throw new Error("AI_PROVIDER_NOT_FOUND");
    if (input.isDefault) await client.query(`update ai_providers set is_default=false, updated_at=now() where is_default and id<>$1`, [providerId]);
    const envelope = apiKey ? encryptCredentialObject({ apiKey }, getWebRuntimeConfig().authSecret,
      credentialAAD("ai-provider", providerId)) : null;
    return (await client.query<AIProviderView>(
      `update ai_providers set name=$2, provider_type=$3, base_url=nullif($4,''), model_id=$5,
         enabled=$6, is_default=$7, credential_envelope_json=coalesce($8::jsonb, credential_envelope_json),
         status=case when $8::jsonb is null then status else 'untested' end,
         updated_by_user_id=$9, updated_at=now() where id=$1 returning ${projection}`,
      [providerId, input.name, input.providerType, input.baseUrl ?? "", input.modelId,
        input.enabled, input.isDefault, envelope, user.id]
    )).rows[0];
  });
}

export async function deleteAIProvider(providerId: number, requestId?: string | null) {
  const user = await requireAdmin();
  return withAuditedPlatformWrite({
    requestId: correlationId(requestId), actorUserId: user.id, action: "ai_provider.delete",
    resourceType: "ai_provider", resourceId: String(providerId), input: { providerId }
  }, async (client) => {
    const result = await client.query(`delete from ai_providers where id=$1`, [providerId]);
    if (result.rowCount !== 1) throw new Error("AI_PROVIDER_NOT_FOUND");
    return { id: providerId, deleted: true };
  });
}

export async function testAIProvider(providerId: number, requestId?: string | null) {
  const user = await requireAdmin();
  return withAuditedPlatformWrite({
    requestId: correlationId(requestId), actorUserId: user.id, action: "ai_provider.test",
    resourceType: "ai_provider", resourceId: String(providerId), input: { providerId }
  }, async (client) => {
    const provider = (await client.query<{ id: string; baseUrl: string | null; envelope: CredentialEnvelope }>(
      `select id, base_url as "baseUrl", credential_envelope_json as envelope from ai_providers where id=$1 for update`, [providerId]
    )).rows[0];
    if (!provider) throw new Error("AI_PROVIDER_NOT_FOUND");
    const { apiKey } = decryptCredentialObject<{ apiKey: string }>(provider.envelope, getWebRuntimeConfig().authSecret,
      credentialAAD("ai-provider", provider.id));
    const baseUrl = (provider.baseUrl || "https://api.openai.com/v1").replace(/\/$/, "");
    await validateBaseURL(baseUrl);
    let ok = false;
    let code = "AI_PROVIDER_CONNECTION_FAILED";
    try {
      const response = await fetch(`${baseUrl}/models`, {
        headers: { authorization: `Bearer ${apiKey}` }, redirect: "error", signal: AbortSignal.timeout(10_000)
      });
      ok = response.ok;
      code = response.ok ? "OK" : `HTTP_${response.status}`;
    } catch { code = "AI_PROVIDER_CONNECTION_FAILED"; }
    const health = { ok, code, checkedAt: new Date().toISOString() };
    await client.query(
      `update ai_providers set status=$2, health_json=$3, last_tested_at=now(), updated_by_user_id=$4, updated_at=now() where id=$1`,
      [providerId, ok ? "healthy" : "failed", health, user.id]
    );
    return health;
  });
}
