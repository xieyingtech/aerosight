import { createHash } from "node:crypto";
import { z } from "zod";

const referencePattern = /^(?:inputs(?:\.[A-Za-z][A-Za-z0-9_-]*)*|steps\.[A-Za-z][A-Za-z0-9_-]*\.outputs(?:\.[A-Za-z][A-Za-z0-9_-]*)*)$/;
const forbiddenSegments = new Set(["__proto__", "prototype", "constructor"]);

export type TaskConditionOperand = { ref: string } | { value: unknown };
export type TaskCondition =
  | { op: "all"; conditions: TaskCondition[] }
  | { op: "any"; conditions: TaskCondition[] }
  | { op: "not"; condition: TaskCondition }
  | { op: "exists"; target: TaskConditionOperand }
  | { op: "eq" | "ne" | "gt" | "gte" | "lt" | "lte" | "in" | "contains"; left: TaskConditionOperand; right: TaskConditionOperand };

const taskConditionReferenceSchema = z.string().min(1).superRefine((reference, context) => {
  const segments = reference.split(".");
  if (!referencePattern.test(reference) || segments.some((segment) => forbiddenSegments.has(segment))) {
    context.addIssue({ code: "custom", message: "condition references may only read inputs or steps.<key>.outputs" });
  }
});

const taskConditionOperandSchema: z.ZodType<TaskConditionOperand> = z.union([
  z.object({ ref: taskConditionReferenceSchema }).strict(),
  z.object({ value: z.unknown() }).strict()
]);

export const taskConditionSchema: z.ZodType<TaskCondition> = z.lazy(() => z.union([
  z.object({ op: z.enum(["all", "any"]), conditions: z.array(taskConditionSchema).min(1) }).strict(),
  z.object({ op: z.literal("not"), condition: taskConditionSchema }).strict(),
  z.object({ op: z.literal("exists"), target: taskConditionOperandSchema }).strict(),
  z.object({
    op: z.enum(["eq", "ne", "gt", "gte", "lt", "lte", "in", "contains"]),
    left: taskConditionOperandSchema,
    right: taskConditionOperandSchema
  }).strict()
])) as z.ZodType<TaskCondition>;

export type TaskConditionContext = {
  inputs: Record<string, unknown>;
  steps: Record<string, { outputs: Record<string, unknown> }>;
};

export type TaskConditionAudit = {
  conditionHash: string;
  references: string[];
  result: boolean;
};

function canonicalJson(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(",")}]`;
  if (value && typeof value === "object") {
    return `{${Object.entries(value as Record<string, unknown>).sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJson(item)}`).join(",")}}`;
  }
  return JSON.stringify(value);
}

function readReference(context: TaskConditionContext, reference: string): unknown {
  const segments = reference.split(".");
  let value: unknown = context;
  for (const segment of segments) {
    if (forbiddenSegments.has(segment) || !value || typeof value !== "object"
      || !Object.prototype.hasOwnProperty.call(value, segment)) {
      throw new Error(`TASK_CONDITION_REFERENCE_MISSING:${reference}`);
    }
    value = (value as Record<string, unknown>)[segment];
  }
  return value;
}

function resolveOperand(context: TaskConditionContext, operand: TaskConditionOperand, references: Set<string>) {
  if ("value" in operand) return operand.value;
  references.add(operand.ref);
  return readReference(context, operand.ref);
}

function comparableNumbers(left: unknown, right: unknown): [number, number] {
  if (typeof left !== "number" || !Number.isFinite(left) || typeof right !== "number" || !Number.isFinite(right)) {
    throw new Error("TASK_CONDITION_TYPE_MISMATCH");
  }
  return [left, right];
}

function evaluate(condition: TaskCondition, context: TaskConditionContext, references: Set<string>): boolean {
  if (condition.op === "all") return condition.conditions.every((item) => evaluate(item, context, references));
  if (condition.op === "any") return condition.conditions.some((item) => evaluate(item, context, references));
  if (condition.op === "not") return !evaluate(condition.condition, context, references);
  if (condition.op === "exists") {
    try {
      return resolveOperand(context, condition.target, references) !== undefined;
    } catch (error) {
      if (error instanceof Error && error.message.startsWith("TASK_CONDITION_REFERENCE_MISSING:")) return false;
      throw error;
    }
  }

  const left = resolveOperand(context, condition.left, references);
  const right = resolveOperand(context, condition.right, references);
  if (condition.op === "eq" || condition.op === "ne") {
    if (left !== null && right !== null && typeof left !== typeof right) throw new Error("TASK_CONDITION_TYPE_MISMATCH");
    const equal = canonicalJson(left) === canonicalJson(right);
    return condition.op === "eq" ? equal : !equal;
  }
  if (condition.op === "in") {
    if (!Array.isArray(right)) throw new Error("TASK_CONDITION_TYPE_MISMATCH");
    return right.some((item) => canonicalJson(item) === canonicalJson(left));
  }
  if (condition.op === "contains") {
    if (Array.isArray(left)) return left.some((item) => canonicalJson(item) === canonicalJson(right));
    if (typeof left === "string" && typeof right === "string") return left.includes(right);
    throw new Error("TASK_CONDITION_TYPE_MISMATCH");
  }
  const [leftNumber, rightNumber] = comparableNumbers(left, right);
  if (condition.op === "gt") return leftNumber > rightNumber;
  if (condition.op === "gte") return leftNumber >= rightNumber;
  if (condition.op === "lt") return leftNumber < rightNumber;
  return leftNumber <= rightNumber;
}

export function evaluateTaskCondition(input: unknown, context: TaskConditionContext): TaskConditionAudit {
  const condition = taskConditionSchema.parse(input);
  const references = new Set<string>();
  const result = evaluate(condition, context, references);
  return {
    conditionHash: createHash("sha256").update(canonicalJson(condition)).digest("hex"),
    references: [...references].sort(),
    result
  };
}
