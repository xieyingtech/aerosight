import { createOpenAI } from "@ai-sdk/openai";
import { createProviderRegistry,streamText,type LanguageModel } from "ai";
import type { WebRuntimeConfig } from "@/lib/runtime-config";

type RegistryProviders=Parameters<typeof createProviderRegistry>[0];

export function createAgentProviderRegistryFromProviders<PROVIDERS extends RegistryProviders>(providers:PROVIDERS){
  if(Object.keys(providers).length===0)throw new Error("AI_PROVIDER_REGISTRY_EMPTY");
  return createProviderRegistry(providers);
}

export function createAgentProviderRegistry(config:WebRuntimeConfig){
  if(config.aiProvider==="disabled")throw new Error("AI_PROVIDER_DISABLED");
  if(config.aiProvider==="openai"&&config.aiModel&&config.openaiApiKey){
    return {registry:createAgentProviderRegistryFromProviders({openai:createOpenAI({apiKey:config.openaiApiKey})}),modelId:`openai:${config.aiModel}` as `openai:${string}`};
  }
  throw new Error("AI_PROVIDER_CONFIGURATION_INVALID");
}

export async function collectAgentTextStream(model:LanguageModel,prompt:string){
  const result=streamText({model,prompt});let text="";
  for await(const chunk of result.textStream)text+=chunk;
  return text;
}
