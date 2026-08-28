import assert from "node:assert/strict";
import test from "node:test";
import { customProvider,simulateReadableStream } from "ai";
import { MockLanguageModelV3 } from "ai/test";
import { collectAgentTextStream,createAgentProviderRegistryFromProviders,resolveStoredAIProvider } from "./agent-provider-registry.ts";
import { credentialAAD, encryptCredentialObject } from "./credential-encryption.ts";

test("provider registry streams through a replaceable mock provider",async()=>{
  const model=new MockLanguageModelV3({provider:"mock",modelId:"inspection-test",doStream:async()=>({
    stream:simulateReadableStream({chunks:[
      {type:"stream-start",warnings:[]},
      {type:"text-start",id:"text-1"},
      {type:"text-delta",id:"text-1",delta:"巡检"},
      {type:"text-delta",id:"text-1",delta:"完成"},
      {type:"text-end",id:"text-1"},
      {type:"finish",finishReason:{unified:"stop",raw:undefined},usage:{inputTokens:{total:1,noCache:1,cacheRead:0,cacheWrite:0},outputTokens:{total:2,text:2,reasoning:0}}}
    ]})
  })});
  const registry=createAgentProviderRegistryFromProviders({mock:customProvider({languageModels:{inspection:model}})});
  const text=await collectAgentTextStream(registry.languageModel("mock:inspection"),"汇总巡检");
  assert.equal(text,"巡检完成");assert.equal(model.doStreamCalls.length,1);
});

test("empty provider registry is rejected",()=>assert.throws(()=>createAgentProviderRegistryFromProviders({}),/AI_PROVIDER_REGISTRY_EMPTY/));

test("stored provider selection fails closed without exactly one valid default",()=>{
  const secret="0123456789abcdef0123456789abcdef";
  assert.throws(()=>resolveStoredAIProvider([],secret),/AI_PROVIDER_UNAVAILABLE/);
  const provider={id:"7",providerType:"openai",baseUrl:null,modelId:"gpt-test",
    envelope:encryptCredentialObject({apiKey:"database-key"},secret,credentialAAD("ai-provider",7))};
  assert.deepEqual(resolveStoredAIProvider([provider],secret),{apiKey:"database-key",modelId:"gpt-test",baseUrl:null});
  const replacement={id:"8",providerType:"openai",baseUrl:"https://api.example.test/v1",modelId:"gpt-next",
    envelope:encryptCredentialObject({apiKey:"replacement-key"},secret,credentialAAD("ai-provider",8))};
  assert.deepEqual(resolveStoredAIProvider([replacement],secret),{
    apiKey:"replacement-key",modelId:"gpt-next",baseUrl:"https://api.example.test/v1"
  });
  assert.throws(()=>resolveStoredAIProvider([provider,{...provider,id:"8"}],secret),/CONFIGURATION_INVALID/);
  assert.throws(()=>resolveStoredAIProvider([{...provider,id:"8"}],secret),/DECRYPTION_FAILED/);
});
