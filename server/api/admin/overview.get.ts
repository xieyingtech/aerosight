import { db, schema } from "@nuxthub/db";
import { count } from "drizzle-orm";

export default defineEventHandler(async (event) => {
  await requireUserSession(event);

  const [usersResult, teamsResult, projectsResult] = await Promise.all([
    db.select({ total: count(schema.users.id) }).from(schema.users),
    db.select({ total: count(schema.teams.id) }).from(schema.teams),
    db.select({ total: count(schema.projects.id) }).from(schema.projects),
  ]);

  return {
    users: usersResult[0]?.total ?? 0,
    teams: teamsResult[0]?.total ?? 0,
    projects: projectsResult[0]?.total ?? 0,
  };
});
