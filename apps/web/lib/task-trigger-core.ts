import { z } from "zod";
import { taskTriggerSchema } from "./task-orchestration-schema.ts";

const triggerInputsSchema = z.record(z.string(), z.unknown()).default({});
const common = {
  idempotencyKey: z.string().min(1).max(400),
  occurredAt: z.string().datetime(),
  inputs: triggerInputsSchema
};

export const taskTriggerInvocationSchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("manual"), ...common }).strict(),
  z.object({ type: z.literal("schedule"), scheduledFor: z.string().datetime(), ...common }).strict(),
  z.object({ type: z.literal("api"), key: z.string().min(1), ...common }).strict(),
  z.object({ type: z.literal("webhook"), source: z.string().min(1), deliveryId: z.string().min(1), ...common }).strict(),
  z.object({ type: z.literal("upstream"), upstreamProjectId: z.number().int().positive(), upstreamTaskId: z.number().int().positive(), upstreamRunId: z.number().int().positive(), status: z.enum(["succeeded", "failed"]), ...common }).strict(),
  z.object({ type: z.literal("copilot"), delegation: z.enum(["chat", "issue-mention", "issue-assignment"]), agentJobId: z.string().min(1), ...common }).strict()
]);

export type TaskTriggerInvocation = z.infer<typeof taskTriggerInvocationSchema>;

export function assertUserCallableTaskTrigger(invocation: TaskTriggerInvocation) {
  if (invocation.type !== "manual" && invocation.type !== "api" && invocation.type !== "webhook") {
    throw new Error("TASK_TRIGGER_INTERNAL_SOURCE_REQUIRED");
  }
}

export type TaskTriggerAuthorization = {
  projectId: number;
  taskProjectId: number;
  taskVersionStatus: string;
  taskStatus: string;
  concurrencyLimit: number;
  activeRunCount: number;
  actor: { type: "user" | "agent" | "service"; id: string };
};

function assertInputSnapshot(schema: unknown, inputs: Record<string, unknown>) {
  if (!schema || typeof schema !== "object") throw new Error("TASK_TRIGGER_INPUT_SCHEMA_INVALID");
  const inputSchema = schema as { required?: unknown; properties?: unknown; additionalProperties?: unknown };
  const properties = inputSchema.properties && typeof inputSchema.properties === "object"
    ? inputSchema.properties as Record<string, unknown> : {};
  const required = Array.isArray(inputSchema.required) ? inputSchema.required : [];
  for (const key of required) if (typeof key === "string" && !(key in inputs)) throw new Error(`TASK_TRIGGER_INPUT_REQUIRED:${key}`);
  if (inputSchema.additionalProperties === false) {
    for (const key of Object.keys(inputs)) if (!(key in properties)) throw new Error(`TASK_TRIGGER_INPUT_UNKNOWN:${key}`);
  }
}

export function planTaskTrigger(input: {
  trigger: unknown;
  inputSchema: unknown;
  invocation: unknown;
  authorization: TaskTriggerAuthorization;
}) {
  const trigger = taskTriggerSchema.parse(input.trigger);
  const invocation = taskTriggerInvocationSchema.parse(input.invocation);
  const auth = input.authorization;
  if (auth.taskProjectId !== auth.projectId) throw new Error("TASK_TRIGGER_SCOPE_MISMATCH");
  if (auth.taskVersionStatus !== "published") throw new Error("TASK_TRIGGER_VERSION_NOT_PUBLISHED");
  if (auth.taskStatus !== "active") throw new Error("TASK_TRIGGER_TASK_DISABLED");
  if (trigger.type !== invocation.type) throw new Error("TASK_TRIGGER_TYPE_MISMATCH");
  if (trigger.type === "schedule" && (!trigger.enabled || invocation.type !== "schedule")) throw new Error("TASK_TRIGGER_SCHEDULE_DISABLED");
  if (trigger.type === "api" && (invocation.type !== "api" || trigger.key !== invocation.key)) throw new Error("TASK_TRIGGER_API_KEY_MISMATCH");
  if (trigger.type === "webhook" && (invocation.type !== "webhook" || trigger.source !== invocation.source)) throw new Error("TASK_TRIGGER_WEBHOOK_SOURCE_MISMATCH");
  if (trigger.type === "upstream" && (invocation.type !== "upstream" || invocation.upstreamProjectId !== auth.projectId || trigger.taskId !== invocation.upstreamTaskId || !trigger.statuses.includes(invocation.status))) throw new Error("TASK_TRIGGER_UPSTREAM_MISMATCH");
  if (trigger.type === "copilot" && (invocation.type !== "copilot" || trigger.delegation !== invocation.delegation)) throw new Error("TASK_TRIGGER_COPILOT_MISMATCH");
  if (auth.activeRunCount >= auth.concurrencyLimit) throw new Error("TASK_TRIGGER_CONCURRENCY_LIMIT");
  assertInputSnapshot(input.inputSchema, invocation.inputs);
  const details = { ...invocation } as Record<string, unknown>;
  delete details.inputs;
  if (invocation.type === "api") delete details.key;
  return Object.freeze({
    triggerKey: `${invocation.type}:${invocation.idempotencyKey}`,
    triggerSource: invocation.type,
    inputSnapshot: {
      trigger: { ...details, actor: auth.actor },
      inputs: invocation.inputs
    }
  });
}
