import { createOpenAI } from "@ai-sdk/openai";
import { createProviderRegistry,streamText,type LanguageModel } from "ai";

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

export async function collectAgentTextStream(model:LanguageModel,prompt:string){
  const result=streamText({model,prompt});let text="";
  for await(const chunk of result.textStream)text+=chunk;
  return text;
}
