import assert from "node:assert/strict";
import test from "node:test";
import { resolveStoredAIProvider } from "./stored-ai-provider-policy.ts";
import { credentialAAD, encryptCredentialObject } from "./credential-encryption.ts";

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
