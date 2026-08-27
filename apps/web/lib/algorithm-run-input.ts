import { z } from "zod";

export const startAlgorithmRunInputSchema = z.object({
  definitionVersionId: z.coerce.number().int().positive(),
  assetId: z.coerce.number().int().positive(),
  parameters: z.record(z.string(), z.unknown()).default({})
}).strict();

export function coerceSchemaParameters(schema: Record<string, unknown>, values: Record<string, FormDataEntryValue>) {
  const properties = schema.properties && typeof schema.properties === "object" && !Array.isArray(schema.properties)
    ? schema.properties as Record<string, Record<string, unknown>> : {};
  const result: Record<string, unknown> = {};
  for (const [key, definition] of Object.entries(properties)) {
    const raw = values[key];
    if (typeof raw !== "string" || raw === "") continue;
    if (definition.type === "number" || definition.type === "integer") result[key] = Number(raw);
    else if (definition.type === "boolean") result[key] = raw === "true";
    else result[key] = raw;
  }
  return result;
}
