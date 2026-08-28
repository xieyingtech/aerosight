import "server-only";

import { createOpenAIAgentProvider } from "@/lib/agent-provider-registry";
import { credentialAAD, decryptCredentialObject, type CredentialEnvelope } from "@/lib/credential-encryption";
import { query } from "@/lib/db";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

export async function loadAgentProviderRegistry() {
  const provider = (await query<{ id: string; providerType: string; baseUrl: string | null; modelId: string; envelope: CredentialEnvelope }>(
    `select id, provider_type as "providerType", base_url as "baseUrl", model_id as "modelId",
            credential_envelope_json as envelope
       from ai_providers where enabled and is_default limit 2`
  )).rows;
  if (provider.length === 0) throw new Error("AI_PROVIDER_UNAVAILABLE");
  if (provider.length !== 1 || provider[0].providerType !== "openai") throw new Error("AI_PROVIDER_CONFIGURATION_INVALID");
  const { apiKey } = decryptCredentialObject<{ apiKey: string }>(provider[0].envelope,
    getWebRuntimeConfig().authSecret, credentialAAD("ai-provider", provider[0].id));
  if (!apiKey) throw new Error("AI_PROVIDER_CREDENTIAL_UNAVAILABLE");
  return createOpenAIAgentProvider(apiKey, provider[0].modelId, provider[0].baseUrl);
}
