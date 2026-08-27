import assert from "node:assert/strict";
import test from "node:test";
import { createAlertAutomationPolicyVersion, publishAlertAutomationPolicyVersion, type AlertAutomationPolicyVersion } from "./alert-automation-policy-core.ts";

test("new alert automation policy defaults to manual",()=>{
  assert.deepEqual(createAlertAutomationPolicyVersion([],{}),{version:1,status:"draft",mode:"manual",eventRuleVersionId:null,config:{}});
});

test("publishing a new policy version preserves immutable history",()=>{
  const history:AlertAutomationPolicyVersion[]=[{version:1,status:"published",mode:"manual",eventRuleVersionId:3,config:{notify:true}},{version:2,status:"draft",mode:"agent-auto-draft",eventRuleVersionId:4,config:{prompt:"v2"}}];
  const before=structuredClone(history);const published=publishAlertAutomationPolicyVersion(history,2);
  assert.deepEqual(history,before);
  assert.equal(published[0].status,"retired");assert.equal(published[0].mode,"manual");
  assert.equal(published[1].status,"published");assert.equal(published[1].mode,"agent-auto-draft");
});

test("all supported automation modes are accepted and unknown modes fail",()=>{
  for(const mode of ["manual","agent-on-demand","agent-auto-draft","follow-up-draft"] as const)assert.equal(createAlertAutomationPolicyVersion([],{mode}).mode,mode);
  assert.throws(()=>createAlertAutomationPolicyVersion([],{mode:"auto-execute"}));
});
