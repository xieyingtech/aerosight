import { db, schema } from "@nuxthub/db";
import { desc, eq } from "drizzle-orm";

export default defineEventHandler(async (event) => {
  await requireUserSession(event);

  return db
    .select({
      id: schema.projects.id,
      name: schema.projects.name,
      description: schema.projects.description,
      teamId: schema.projects.teamId,
      teamName: schema.teams.name,
      createdByUserId: schema.projects.createdByUserId,
      createdByUserName: schema.users.name,
      createdAt: schema.projects.createdAt,
      updatedAt: schema.projects.updatedAt,
    })
    .from(schema.projects)
    .innerJoin(schema.teams, eq(schema.teams.id, schema.projects.teamId))
    .leftJoin(schema.users, eq(schema.users.id, schema.projects.createdByUserId))
    .orderBy(desc(schema.projects.createdAt));
});
