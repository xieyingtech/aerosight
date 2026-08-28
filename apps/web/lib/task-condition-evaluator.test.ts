import assert from "node:assert/strict";
import test from "node:test";
import { evaluateTaskCondition, taskConditionSchema } from "./task-condition-evaluator.ts";

const context = {
  inputs: { minimumConfidence: 0.8, acceptedCategories: ["suspected-construction"] },
  steps: {
    detect: { outputs: { category: "suspected-construction", confidence: 0.91, count: 3, status: "succeeded", spatialRelation: "inside" } }
  }
};

test("task condition reads typed inputs and prior step outputs deterministically", () => {
  const condition = { op: "all", conditions: [
    { op: "gte", left: { ref: "steps.detect.outputs.confidence" }, right: { ref: "inputs.minimumConfidence" } },
    { op: "in", left: { ref: "steps.detect.outputs.category" }, right: { ref: "inputs.acceptedCategories" } },
    { op: "gte", left: { ref: "steps.detect.outputs.count" }, right: { value: 1 } },
    { op: "eq", left: { ref: "steps.detect.outputs.status" }, right: { value: "succeeded" } },
    { op: "eq", left: { ref: "steps.detect.outputs.spatialRelation" }, right: { value: "inside" } }
  ] };
  const first = evaluateTaskCondition(condition, context);
  const second = evaluateTaskCondition(condition, context);
  assert.equal(first.result, true);
  assert.deepEqual(first, second);
  assert.deepEqual(first.references, [
    "inputs.acceptedCategories", "inputs.minimumConfidence", "steps.detect.outputs.category",
    "steps.detect.outputs.confidence", "steps.detect.outputs.count", "steps.detect.outputs.spatialRelation",
    "steps.detect.outputs.status"
  ]);
});

test("missing outputs and type errors fail explicitly", () => {
  assert.throws(() => evaluateTaskCondition({
    op: "eq", left: { ref: "steps.detect.outputs.missing" }, right: { value: "x" }
  }, context), /TASK_CONDITION_REFERENCE_MISSING/);
  assert.throws(() => evaluateTaskCondition({
    op: "gte", left: { ref: "steps.detect.outputs.status" }, right: { value: 1 }
  }, context), /TASK_CONDITION_TYPE_MISMATCH/);
  assert.equal(evaluateTaskCondition({ op: "exists", target: { ref: "steps.detect.outputs.missing" } }, context).result, false);
});

test("condition schema rejects code injection and arbitrary object traversal", () => {
  for (const ref of ["process.env.AUTH_SECRET", "steps.detect.outputs.constructor", "steps.detect.result", "inputs.x;globalThis.pwned=true"]) {
    assert.equal(taskConditionSchema.safeParse({ op: "exists", target: { ref } }).success, false, ref);
  }
  assert.equal(taskConditionSchema.safeParse({ op: "script", source: "return true" }).success, false);
});
