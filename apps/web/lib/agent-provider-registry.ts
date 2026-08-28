import { createOpenAI } from "@ai-sdk/openai";
import { createProviderRegistry,streamText,type LanguageModel } from "ai";
import { credentialAAD, decryptCredentialObject, type CredentialEnvelope } from "./credential-encryption.ts";

type RegistryProviders=Parameters<typeof createProviderRegistry>[0];

export function createAgentProviderRegistryFromProviders<PROVIDERS extends RegistryProviders>(providers:PROVIDERS){
  if(Object.keys(providers).length===0)throw new Error("AI_PROVIDER_REGISTRY_EMPTY");
  return createProviderRegistry(providers);
}

export function createOpenAIAgentProvider(apiKey:string,modelId:string,baseURL?:string|null){
  return {
    registry:createAgentProviderRegistryFromProviders({openai:createOpenAI({apiKey,baseURL:baseURL||undefined})}),
    modelId:`openai:${modelId}` as `openai:${string}`
  };
}

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

export async function collectAgentTextStream(model:LanguageModel,prompt:string){
  const result=streamText({model,prompt});let text="";
  for await(const chunk of result.textStream)text+=chunk;
  return text;
}
