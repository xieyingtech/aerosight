import { z } from "zod";

const coordinateSchema = z.tuple([
  z.number().min(-180).max(180),
  z.number().min(-90).max(90),
  z.number().min(-500).max(10_000).optional()
]);

const routeSchema = z.object({
  type: z.literal("route"),
  coordinates: z.array(coordinateSchema).min(2),
  maxAltitudeMeters: z.number().positive().max(500),
  maxSpeedMetersPerSecond: z.number().positive().max(50)
});

const areaSchema = z.object({
  type: z.literal("area"),
  rings: z.array(z.array(coordinateSchema).min(4)).min(1),
  maxAltitudeMeters: z.number().positive().max(500),
  maxSpeedMetersPerSecond: z.number().positive().max(50)
}).superRefine((area, context) => {
  area.rings.forEach((ring, ringIndex) => {
    const first = ring[0];
    const last = ring.at(-1);
    if (first?.[0] !== last?.[0] || first?.[1] !== last?.[1] || first?.[2] !== last?.[2]) {
      context.addIssue({
        code: "custom",
        message: "area ring must be closed",
        path: ["rings", ringIndex]
      });
    }
  });
});

export const missionSpatialScopeSchema = z.union([routeSchema, areaSchema]);

export const missionTriggerSchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("manual") }),
  z.object({ type: z.literal("schedule"), cron: z.string().min(1), timezone: z.string().min(1) }),
  z.object({ type: z.literal("event"), ruleId: z.string().min(1), eventTypes: z.array(z.string().min(1)).min(1) })
]);

export const missionFailurePolicySchema = z.object({
  onFailure: z.enum(["abort", "pause", "continue"]),
  maxRetries: z.number().int().min(0).max(10),
  retryBackoffSeconds: z.number().int().min(0).max(3600),
  idempotency: z.enum(["safe", "unsafe"])
}).superRefine((policy, context) => {
  if (policy.idempotency === "unsafe" && policy.maxRetries > 0) {
    context.addIssue({ code: "custom", message: "unsafe actions cannot be retried", path: ["maxRetries"] });
  }
});

export const missionMediaRequirementsSchema = z.object({
  required: z.boolean(),
  modes: z.array(z.enum(["photo", "video", "thermal"])).min(1),
  minimumCount: z.number().int().min(0).max(10_000)
}).superRefine((media, context) => {
  if (media.required && media.minimumCount < 1) {
    context.addIssue({ code: "custom", message: "required media needs a positive minimum count", path: ["minimumCount"] });
  }
});

export const inspectionMissionStepSchema = z.object({
  position: z.number().int().positive(),
  stepKey: z.string().min(1),
  name: z.string().min(1),
  capabilityCode: z.string().min(1),
  action: z.string().min(1),
  parameters: z.record(z.string(), z.unknown()),
  failurePolicy: missionFailurePolicySchema,
  mediaRequirements: missionMediaRequirementsSchema
});

export const inspectionMissionDefinitionSchema = z.object({
  name: z.string().min(1),
  objective: z.string().min(1),
  spatialScope: missionSpatialScopeSchema,
  requiredCapabilities: z.array(z.object({
    code: z.string().min(1),
    constraints: z.record(z.string(), z.unknown()).default({})
  })).min(1),
  trigger: missionTriggerSchema,
  concurrencyLimit: z.number().int().positive().max(100).default(1),
  reportTemplate: z.object({ templateKey: z.string().min(1) }),
  steps: z.array(inspectionMissionStepSchema).min(1)
});

export type InspectionMissionDefinition = z.infer<typeof inspectionMissionDefinitionSchema>;

export function validateInspectionMissionDefinition(input: unknown): InspectionMissionDefinition {
  return inspectionMissionDefinitionSchema.parse(input);
}
