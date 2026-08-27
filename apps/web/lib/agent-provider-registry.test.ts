import assert from "node:assert/strict";
import test from "node:test";
import { customProvider,simulateReadableStream } from "ai";
import { MockLanguageModelV3 } from "ai/test";
import { collectAgentTextStream,createAgentProviderRegistryFromProviders } from "./agent-provider-registry.ts";

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
