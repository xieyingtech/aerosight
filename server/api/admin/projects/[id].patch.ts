import { db, schema } from "@nuxthub/db";
import { eq } from "drizzle-orm";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  await requireUserSession(event);

  const id = z.coerce
    .number()
    .int()
    .positive("errors.validation.id.required")
    .parse(getRouterParam(event, "id"));

  const body = await readBody(event);
  const payload = z
    .object({
      teamId: z.coerce
        .number()
        .int()
        .positive("errors.validation.teamId.required")
        .optional(),
      name: z
        .string()
        .trim()
        .min(1, "errors.validation.projectName.required")
        .optional(),
      description: z.string().trim().optional(),
    })
    .parse(body);

  return db
    .update(schema.projects)
    .set({
      ...(payload.teamId !== undefined ? { teamId: payload.teamId } : {}),
      ...(payload.name !== undefined ? { name: payload.name } : {}),
      ...(payload.description !== undefined
        ? { description: payload.description || null }
        : {}),
      updatedAt: new Date(),
    })
    .where(eq(schema.projects.id, id))
    .returning();
});
