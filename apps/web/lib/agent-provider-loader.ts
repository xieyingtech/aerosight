import "server-only";

import { createOpenAIAgentProvider, resolveStoredAIProvider, type StoredAIProvider } from "@/lib/agent-provider-registry";
import { query } from "@/lib/db";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

export async function loadAgentProviderRegistry() {
  const provider = (await query<StoredAIProvider>(
    `select id, provider_type as "providerType", base_url as "baseUrl", model_id as "modelId",
            credential_envelope_json as envelope
       from ai_providers where enabled and is_default limit 2`
  )).rows;
  const configured = resolveStoredAIProvider(provider, getWebRuntimeConfig().authSecret);
  return createOpenAIAgentProvider(configured.apiKey, configured.modelId, configured.baseUrl);
}
