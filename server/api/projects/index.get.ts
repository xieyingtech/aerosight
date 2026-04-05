import { db, schema } from "@nuxthub/db";
import { and, desc, eq, ilike, inArray, or } from "drizzle-orm";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  const session = await requireUserSession(event);

  const { scope, search } = z
    .object({
      scope: z.enum(["all", "joined", "managed"]).optional(),
      search: z.string().optional(),
    })
    .parse(getQuery(event));

  const conditions = [eq(schema.teamMembers.userId, session.user.id)];

  if (scope === "joined") {
    conditions.push(eq(schema.teamMembers.role, "member"));
  }

  if (scope === "managed") {
    conditions.push(inArray(schema.teamMembers.role, ["owner", "admin"]));
  }

  const searchTerm = search?.trim();
  if (searchTerm) {
    const keyword = `%${searchTerm}%`;

    conditions.push(
      or(
        ilike(schema.projects.name, keyword),
        ilike(schema.projects.description, keyword),
        ilike(schema.teams.name, keyword),
      )!,
    );
  }

  return db
    .select({
      id: schema.projects.id,
      name: schema.projects.name,
      description: schema.projects.description,
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
        eq(schema.teamMembers.userId, session.user.id),
      ),
    )
    .where(and(...conditions))
    .orderBy(desc(schema.projects.updatedAt));
});
