import { db, schema } from "@nuxthub/db";
import { aliasedTable, and, asc, count, eq } from "drizzle-orm";

export default defineEventHandler(async (event) => {
  const { user: _user } = await requireUserSession(event);

  const owners = aliasedTable(schema.teamMembers, "owners");
  const ownerUsers = aliasedTable(schema.users, "owner_users");

  return db
    .select({
      id: schema.teams.id,
      name: schema.teams.name,
      createdAt: schema.teams.createdAt,
      updatedAt: schema.teams.updatedAt,
      memberCount: count(schema.teamMembers.id),
      ownerUserId: ownerUsers.id,
      ownerName: ownerUsers.name,
    })
    .from(schema.teams)
    .leftJoin(schema.teamMembers, eq(schema.teamMembers.teamId, schema.teams.id))
    .leftJoin(
      owners,
      and(
        eq(owners.teamId, schema.teams.id),
        eq(owners.role, "owner"),
      ),
    )
    .leftJoin(ownerUsers, eq(ownerUsers.id, owners.userId))
    .groupBy(
      schema.teams.id,
      schema.teams.name,
      schema.teams.createdAt,
      schema.teams.updatedAt,
      ownerUsers.id,
      ownerUsers.name,
    )
    .orderBy(asc(schema.teams.name));
});
