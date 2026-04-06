import { db, schema } from "@nuxthub/db";
import { and, asc, eq } from "drizzle-orm";

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);

  return db
    .select({
      id: schema.teams.id,
      name: schema.teams.name,
      role: schema.teamMembers.role,
      joinedAt: schema.teamMembers.createdAt,
    })
    .from(schema.teams)
    .innerJoin(
      schema.teamMembers,
      and(
        eq(schema.teamMembers.teamId, schema.teams.id),
        eq(schema.teamMembers.userId, user.id),
      ),
    )
    .orderBy(asc(schema.teams.name));
});