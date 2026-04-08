import { db, schema } from "@nuxthub/db";
import { and, eq } from "drizzle-orm";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);

  const id = z.coerce
    .number()
    .int()
    .positive("errors.validation.id.required")
    .parse(getRouterParam(event, "id"));

  const project = await db
    .select({
      id: schema.projects.id,
      name: schema.projects.name,
      description: schema.projects.description,
      teamId: schema.projects.teamId,
      teamName: schema.teams.name,
      role: schema.teamMembers.role,
      updatedAt: schema.projects.updatedAt,
    })
    .from(schema.projects)
    .innerJoin(schema.teams, eq(schema.projects.teamId, schema.teams.id))
    .innerJoin(
      schema.teamMembers,
      and(
        eq(schema.teamMembers.teamId, schema.teams.id),
        eq(schema.teamMembers.userId, user.id),
      ),
    )
    .where(eq(schema.projects.id, id))
    .limit(1);

  if (!project.length) {
    throw createError({ statusCode: 404, message: "errors.generic" });
  }

  return project[0];
});
