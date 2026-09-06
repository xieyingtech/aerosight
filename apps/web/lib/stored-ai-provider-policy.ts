import { credentialAAD, decryptCredentialObject, type CredentialEnvelope } from "./credential-encryption.ts";

export type StoredAIProvider = {
  id: string; providerType: string; baseUrl: string | null; modelId: string; envelope: CredentialEnvelope;
};

export function resolveStoredAIProvider(rows: StoredAIProvider[], authSecret: string) {
  if (rows.length === 0) throw new Error("AI_PROVIDER_UNAVAILABLE");
  if (rows.length !== 1 || rows[0].providerType !== "openai") throw new Error("AI_PROVIDER_CONFIGURATION_INVALID");
  const { apiKey } = decryptCredentialObject<{ apiKey: string }>(rows[0].envelope, authSecret,
    credentialAAD("ai-provider", rows[0].id));
  if (!apiKey) throw new Error("AI_PROVIDER_CREDENTIAL_UNAVAILABLE");
  return { apiKey, modelId: rows[0].modelId, baseUrl: rows[0].baseUrl };
}

