import { z } from "zod";

const jsonObjectSchema = z.record(z.string(), z.unknown());
const jsonSchema = jsonObjectSchema.superRefine((schema, context) => {
  if (schema.type !== undefined && typeof schema.type !== "string" && !Array.isArray(schema.type)) {
    context.addIssue({ code: "custom", message: "JSON Schema type must be a string or array" });
  }
});

export const algorithmDefinitionInputSchema = z.object({
  providerId: z.coerce.number().int().positive(),
  name: z.string().trim().min(1).max(160),
  capabilityCode: z.string().regex(/^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$/),
  description: z.string().trim().max(2000).nullable().optional()
}).strict();

export const algorithmDefinitionConfigurationInputSchema = z.object({
  executionMode: z.enum(["synchronous", "asynchronous", "callback"]),
  modelOrProcess: z.string().trim().min(1).max(240),
  inputSchema: jsonSchema,
  parametersSchema: jsonSchema,
  outputSchema: jsonSchema,
  protocolConfig: jsonObjectSchema,
  outputMapping: jsonObjectSchema,
  labelMapping: jsonObjectSchema.default({}),
  displayMetadata: jsonObjectSchema.default({}),
  publishThreshold: z.number().min(0).max(1).default(0)
}).strict();

export type AlgorithmDefinitionInput = z.infer<typeof algorithmDefinitionInputSchema>;
export type AlgorithmDefinitionConfigurationInput = z.infer<typeof algorithmDefinitionConfigurationInputSchema>;
