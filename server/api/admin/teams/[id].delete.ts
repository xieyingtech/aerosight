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

  return db.delete(schema.teams).where(eq(schema.teams.id, id)).returning();
});
