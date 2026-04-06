import { db, schema } from "@nuxthub/db";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  const session = await requireUserSession(event);

  const body = await readBody(event);
  const payload = z
    .object({
      teamId: z.coerce.number().int().positive("errors.validation.teamId.required"),
      name: z.string().trim().min(1, "errors.validation.projectName.required"),
      description: z.string().trim().optional(),
    })
    .parse(body);

  return db
    .insert(schema.projects)
    .values({
      teamId: payload.teamId,
      name: payload.name,
      description: payload.description || null,
      createdByUserId: session.user.id,
    })
    .returning();
});
