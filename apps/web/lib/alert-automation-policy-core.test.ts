import assert from "node:assert/strict";
import test from "node:test";
import { alertAutomationPolicyInputSchema, isAutomaticAlertMode } from "./alert-automation-policy-core.ts";

test("project alert automation mode defaults to manual",()=>{
  assert.deepEqual(alertAutomationPolicyInputSchema.parse({}),{mode:"manual"});
});

test("project mode input exposes no user-managed version fields",()=>{
  assert.throws(()=>alertAutomationPolicyInputSchema.parse({mode:"manual",eventRuleVersionId:3}));
  assert.throws(()=>alertAutomationPolicyInputSchema.parse({mode:"manual",name:"policy-v2"}));
});

test("all supported automation modes are accepted and unknown modes fail",()=>{
  for(const mode of ["manual","agent-on-demand","agent-auto-draft","follow-up-draft"] as const)assert.equal(alertAutomationPolicyInputSchema.parse({mode}).mode,mode);
  assert.throws(()=>alertAutomationPolicyInputSchema.parse({mode:"auto-execute"}));
});

test("only automatic draft modes run without an explicit user request",()=>{
  assert.equal(isAutomaticAlertMode("manual"),false);
  assert.equal(isAutomaticAlertMode("agent-on-demand"),false);
  assert.equal(isAutomaticAlertMode("agent-auto-draft"),true);
  assert.equal(isAutomaticAlertMode("follow-up-draft"),true);
});
