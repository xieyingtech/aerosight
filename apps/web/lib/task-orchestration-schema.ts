import { z } from "zod";
import { inspectionMissionDefinitionSchema } from "./inspection-mission-schema.ts";
import { taskConditionSchema } from "./task-condition-evaluator.ts";

const jsonObjectSchema = z.object({
  type: z.literal("object"),
  properties: z.record(z.string(), z.unknown()).default({}),
  required: z.array(z.string().min(1)).default([]),
  additionalProperties: z.boolean().default(false)
}).passthrough();

export const taskTriggerSchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("manual") }).strict(),
  z.object({ type: z.literal("schedule"), cron: z.string().min(1), timezone: z.string().min(1), enabled: z.boolean().default(true) }).strict(),
  z.object({ type: z.literal("api"), key: z.string().min(1) }).strict(),
  z.object({ type: z.literal("webhook"), source: z.string().min(1) }).strict(),
  z.object({ type: z.literal("upstream"), taskId: z.number().int().positive(), statuses: z.array(z.enum(["succeeded", "failed"])).min(1) }).strict(),
  z.object({ type: z.literal("copilot"), delegation: z.enum(["chat", "issue-mention", "issue-assignment"]) }).strict()
]);

export const taskStepUsesSchema = z.enum([
  "device.command", "device.collect", "algorithm.run", "issue.create-or-update", "copilot.run", "report.generate"
]);

export const taskOrchestrationStepSchema = z.object({
  key: z.string().regex(/^[A-Za-z][A-Za-z0-9_-]*$/),
  name: z.string().min(1),
  uses: taskStepUsesSchema,
  requires: z.array(z.string().min(1)).default([]),
  with: z.record(z.string(), z.unknown()).default({}),
  inputSchema: jsonObjectSchema,
  outputSchema: jsonObjectSchema,
  condition: taskConditionSchema.optional(),
  dependsOn: z.array(z.string().regex(/^[A-Za-z][A-Za-z0-9_-]*$/)).default([]),
  timeoutSeconds: z.number().int().positive().max(86400),
  retry: z.object({ maxAttempts: z.number().int().min(1).max(11), backoffSeconds: z.number().int().min(0).max(3600) }).strict(),
  onFailure: z.enum(["abort", "pause", "continue"])
}).strict();

export const taskOrchestrationDefinitionSchema = z.object({
  name: z.string().min(1),
  description: z.string().default(""),
  inputSchema: jsonObjectSchema,
  trigger: taskTriggerSchema,
  concurrencyLimit: z.number().int().positive().max(100),
  steps: z.array(taskOrchestrationStepSchema).min(1)
}).strict().superRefine((definition, context) => {
  const keys = new Set<string>();
  definition.steps.forEach((step, index) => {
    if (keys.has(step.key)) context.addIssue({ code: "custom", message: "step keys must be unique", path: ["steps", index, "key"] });
    keys.add(step.key);
  });
  definition.steps.forEach((step, index) => {
    for (const dependency of step.dependsOn) {
      if (dependency === step.key || !keys.has(dependency)) {
        context.addIssue({ code: "custom", message: "step dependency must reference another step", path: ["steps", index, "dependsOn"] });
      }
    }
  });
});

export type TaskOrchestrationDefinition = z.infer<typeof taskOrchestrationDefinitionSchema>;

export function migrateLegacyInspectionTask(input: unknown): TaskOrchestrationDefinition {
  const legacy = inspectionMissionDefinitionSchema.parse(input);
  const trigger = legacy.trigger.type === "event"
    ? { type: "webhook" as const, source: `legacy-event:${legacy.trigger.ruleId}` }
    : legacy.trigger;
  return taskOrchestrationDefinitionSchema.parse({
    name: legacy.name,
    description: legacy.objective,
    inputSchema: { type: "object", properties: {}, required: [], additionalProperties: false },
    trigger,
    concurrencyLimit: legacy.concurrencyLimit,
    steps: legacy.steps.map((step, index) => ({
      key: step.stepKey,
      name: step.name,
      uses: step.mediaRequirements.required ? "device.collect" : "device.command",
      requires: [step.capabilityCode],
      with: step.parameters,
      inputSchema: { type: "object", properties: {}, required: [], additionalProperties: true },
      outputSchema: { type: "object", properties: {}, required: [], additionalProperties: true },
      dependsOn: index === 0 ? [] : [legacy.steps[index - 1].stepKey],
      timeoutSeconds: 300,
      retry: { maxAttempts: step.failurePolicy.maxRetries + 1, backoffSeconds: step.failurePolicy.retryBackoffSeconds },
      onFailure: step.failurePolicy.onFailure
    }))
  });
}

export function assertTaskResourcesCompatible(
  definition: TaskOrchestrationDefinition,
  resources: Record<string, readonly string[]>
) {
  for (const step of definition.steps) {
    if (!step.uses.startsWith("device.")) continue;
    const capabilities = new Set(resources[step.key] ?? []);
    const missing = step.requires.filter((capability) => !capabilities.has(capability));
    if (missing.length) throw new Error(`TASK_RESOURCE_INCOMPATIBLE:${step.key}:${missing.join(",")}`);
  }
}
