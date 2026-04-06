import { db, schema } from "@nuxthub/db";
import { and, eq, inArray } from "drizzle-orm";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);

  const body = await readBody(event);
  const { teamId, name } = z
    .object({
      teamId: z.coerce.number().int().positive("errors.validation.teamId.required"),
      name: z
        .string()
        .trim()
        .min(1, "errors.validation.projectName.required"),
    })
    .parse(body);

  const managedTeam = await db
    .select({ id: schema.teamMembers.teamId })
    .from(schema.teamMembers)
    .where(
      and(
        eq(schema.teamMembers.teamId, teamId),
        eq(schema.teamMembers.userId, user.id),
        inArray(schema.teamMembers.role, ["owner", "admin"]),
      ),
    )
    .limit(1);

  if (!managedTeam.length) {
    throw createError({ statusCode: 403, message: "errors.auth.forbidden" });
  }

  return db
    .insert(schema.projects)
    .values({
      teamId,
      name,
      createdByUserId: user.id,
    })
    .returning();
});
