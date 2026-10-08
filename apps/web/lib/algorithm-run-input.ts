import { z } from "./zod.ts";

export const startAlgorithmRunInputSchema = z.object({
  configurationSnapshotId: z.coerce.number().int().positive().optional(),
  definitionVersionId: z.coerce.number().int().positive().optional(),
  assetId: z.coerce.number().int().positive(),
  videoFps: z.number().min(0.2).max(5).optional(),
  parameters: z.record(z.string(), z.unknown()).default({})
}).strict().refine(
  (input) => input.configurationSnapshotId !== undefined || input.definitionVersionId !== undefined,
  { message: "algorithm configuration snapshot is required" }
).transform((input) => ({
  configurationSnapshotId: input.configurationSnapshotId ?? input.definitionVersionId!,
  assetId: input.assetId,
  parameters: input.parameters,
  ...(input.videoFps === undefined ? {} : {videoFps: input.videoFps})
}));

export function coerceSchemaParameters(schema: Record<string, unknown>, values: Record<string, FormDataEntryValue>) {
  const properties = schema.properties && typeof schema.properties === "object" && !Array.isArray(schema.properties)
    ? schema.properties as Record<string, Record<string, unknown>> : {};
  const result: Record<string, unknown> = {};
  for (const [key, definition] of Object.entries(properties)) {
    const raw = values[key];
    if (typeof raw !== "string" || raw === "") continue;
    if (definition.type === "number" || definition.type === "integer") result[key] = Number(raw);
    else if (definition.type === "boolean") result[key] = raw === "true";
    else if (definition.type === "array") {
      const trimmed = raw.trim();
      const parsed: unknown = trimmed.startsWith("[") ? JSON.parse(trimmed) : trimmed.split(",").map(value => value.trim());
      if (!Array.isArray(parsed)) throw new Error(`${key} 必须是数组`);
      const itemType = (definition.items as Record<string, unknown> | undefined)?.type;
      result[key] = itemType === "number" || itemType === "integer" ? parsed.map(Number) : parsed;
    }
    else result[key] = raw;
  }
  return result;
}
