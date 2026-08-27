import assert from "node:assert/strict";
import test from "node:test";
import { planPerceptionEventAction } from "./perception-event-actions-core.ts";

test("event handling permission and optimistic version are both required",()=>{
  const base={action:"investigate" as const,currentStatus:"open",actualVersion:2,expectedVersion:2,actorUserId:7};
  assert.throws(()=>planPerceptionEventAction({...base,permissions:new Set(["project:view"])}),/PROJECT_ACCESS_DENIED/);
  assert.throws(()=>planPerceptionEventAction({...base,expectedVersion:1,permissions:new Set(["event:handle"])}),/VERSION_CONFLICT/);
  assert.equal(planPerceptionEventAction({...base,permissions:new Set(["event:handle"])}).stateVersion,3);
});

test("feedback actions produce event patches without mutating original algorithm evidence",()=>{
  const originalAlgorithmResult=Object.freeze({runId:"run-1",detections:Object.freeze([{label:"suspected-construction",confidence:.9}])});
  const before=JSON.stringify(originalAlgorithmResult);
  const correction=planPerceptionEventAction({action:"category_correction",category:"extension",currentStatus:"open",actualVersion:0,expectedVersion:0,actorUserId:7,permissions:new Set(["event:handle"])});
  assert.deepEqual(correction.feedbackValue,{category:"extension"});
  assert.equal(correction.status,"investigating");
  assert.equal(JSON.stringify(originalAlgorithmResult),before);
});

test("claim, false-positive, dismiss and resolve map to explicit outcomes",()=>{
  const base={currentStatus:"open",actualVersion:0,expectedVersion:0,actorUserId:7,permissions:new Set(["event:handle"])};
  assert.equal(planPerceptionEventAction({...base,action:"assign"}).assignedUserId,7);
  assert.equal(planPerceptionEventAction({...base,action:"false_positive"}).status,"dismissed");
  assert.equal(planPerceptionEventAction({...base,action:"dismiss"}).status,"dismissed");
  assert.equal(planPerceptionEventAction({...base,action:"resolve"}).status,"resolved");
});
