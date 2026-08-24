import "server-only";

import { cache } from "react";
import { notFound, redirect } from "next/navigation";
import { auth } from "@/auth";
import { db, query } from "@/lib/db";

export type TeamRole = "owner" | "admin" | "member";

export type Project = {
  id: number;
  teamId: number;
  name: string;
  description: string | null;
  teamName: string;
  role: TeamRole;
  updatedAt: Date;
};

export type TeamListItem = {
  id: number;
  name: string;
  role: TeamRole;
  memberCount: number;
  createdAt: Date;
  updatedAt: Date;
};

export const requireUser = cache(async () => {
  const session = await auth();
  if (!session?.user?.userId) redirect("/login");

  const result = await query<{ id: number; name: string; email: string | null; phone: string | null; role: "user" | "admin" }>(
    "select id, name, email, phone, role from users where id = $1",
    [session.user.userId]
  );
  const user = result.rows[0];
  if (!user) redirect("/login");
  return user;
});

export async function requireAdmin() {
  const user = await requireUser();
  if (user.role !== "admin") redirect("/projects");
  return user;
}

export async function listTeams(search = "") {
  const user = await requireUser();
  const result = await query<TeamListItem>(
    `select t.id, t.name, current_members.role,
            count(all_members.id)::int as "memberCount",
            t.created_at as "createdAt", t.updated_at as "updatedAt"
     from teams t
     join team_members current_members on current_members.team_id = t.id and current_members.user_id = $1
     left join team_members all_members on all_members.team_id = t.id
     where ($2 = '' or t.name ilike '%' || $2 || '%')
     group by t.id, current_members.role
     order by t.name`,
    [user.id, search.trim()]
  );
  return result.rows;
}

export async function listManagedTeams() {
  const user = await requireUser();
  const result = await query<{ id: number; name: string }>(
    `select t.id, t.name from teams t
     join team_members tm on tm.team_id = t.id
     where tm.user_id = $1 and tm.role in ('owner', 'admin')
     order by t.name`,
    [user.id]
  );
  return result.rows;
}

export async function getTeam(id: number) {
  const user = await requireUser();
  const teamResult = await query<TeamListItem>(
    `select t.id, t.name, current_members.role,
            count(all_members.id)::int as "memberCount",
            t.created_at as "createdAt", t.updated_at as "updatedAt"
     from teams t
     join team_members current_members on current_members.team_id = t.id and current_members.user_id = $1
     left join team_members all_members on all_members.team_id = t.id
     where t.id = $2
     group by t.id, current_members.role`,
    [user.id, id]
  );
  const team = teamResult.rows[0];
  if (!team) notFound();
  const projects = await query<{ id: number; name: string; description: string | null; updatedAt: Date }>(
    `select id, name, description, updated_at as "updatedAt"
     from projects where team_id = $1 order by updated_at desc`,
    [id]
  );
  return { team, projects: projects.rows };
}

export async function listProjects(scope = "", search = "") {
  const user = await requireUser();
  const result = await query<Project>(
    `select p.id, p.team_id as "teamId", p.name, p.description,
            t.name as "teamName", tm.role, p.updated_at as "updatedAt"
     from projects p
     join teams t on t.id = p.team_id
     join team_members tm on tm.team_id = t.id and tm.user_id = $1
     where ($2 <> 'joined' or tm.role = 'member')
       and ($2 <> 'managed' or tm.role in ('owner', 'admin'))
       and ($3 = '' or p.name ilike '%' || $3 || '%' or coalesce(p.description, '') ilike '%' || $3 || '%' or t.name ilike '%' || $3 || '%')
     order by p.updated_at desc`,
    [user.id, scope, search.trim()]
  );
  return result.rows;
}

export async function getProject(id: number) {
  const user = await requireUser();
  const result = await query<Project>(
    `select p.id, p.team_id as "teamId", p.name, p.description,
            t.name as "teamName", tm.role, p.updated_at as "updatedAt"
     from projects p
     join teams t on t.id = p.team_id
     join team_members tm on tm.team_id = t.id and tm.user_id = $1
     where p.id = $2`,
    [user.id, id]
  );
  const project = result.rows[0];
  if (!project) notFound();
  return project;
}

const projectItemQueries = {
  devices: `select id, name, type, status, last_seen_at as "lastSeenAt", updated_at as "updatedAt" from devices where project_id = $1 order by updated_at desc`,
  agents: `select id, name, description, status, updated_at as "updatedAt" from agents where project_id = $1 order by updated_at desc`,
  tasks: `select id, name, description, trigger_type as "triggerType", status, updated_at as "updatedAt" from tasks where project_id = $1 order by updated_at desc`,
  issues: `select id, number, title, status, priority, updated_at as "updatedAt" from issues where project_id = $1 order by updated_at desc`,
  assets: `select id, kind, mime_type as "mimeType", captured_at as "capturedAt", created_at as "createdAt" from assets where project_id = $1 order by created_at desc`
} as const;

export async function listProjectItems(projectId: number, kind: keyof typeof projectItemQueries) {
  await getProject(projectId);
  const result = await query<Record<string, unknown>>(projectItemQueries[kind], [projectId]);
  return result.rows;
}

export async function getProfileData() {
  const user = await requireUser();
  const teams = await query<Record<string, unknown>>(
    `select t.id, t.name, tm.role, tm.created_at as "joinedAt"
     from teams t join team_members tm on tm.team_id = t.id
     where tm.user_id = $1 order by t.name`,
    [user.id]
  );
  return { profile: [user], teams: teams.rows };
}

export async function getAdminOverview() {
  await requireAdmin();
  const result = await query<{ users: number; teams: number; projects: number }>(
    `select (select count(*)::int from users) as users,
            (select count(*)::int from teams) as teams,
            (select count(*)::int from projects) as projects`
  );
  return result.rows[0];
}

export async function listAdminUsers() {
  await requireAdmin();
  return (await query<Record<string, unknown>>(`select id, name, email, phone, role, created_at as "createdAt" from users order by created_at desc`)).rows;
}

export async function listAdminTeams() {
  await requireAdmin();
  return (await query<Record<string, unknown>>(
    `select t.id, t.name, count(distinct tm.id)::int as "memberCount",
            owner_users.id as "ownerUserId", owner_users.name as "ownerName"
     from teams t
     left join team_members tm on tm.team_id = t.id
     left join team_members owners on owners.team_id = t.id and owners.role = 'owner'
     left join users owner_users on owner_users.id = owners.user_id
     group by t.id, owner_users.id, owner_users.name order by t.name`
  )).rows;
}

export async function listAdminProjects() {
  await requireAdmin();
  return (await query<Record<string, unknown>>(
    `select p.id, p.name, p.description, t.name as "teamName", u.name as "createdByName", p.created_at as "createdAt"
     from projects p join teams t on t.id = p.team_id
     left join users u on u.id = p.created_by_user_id order by p.created_at desc`
  )).rows;
}

export async function createTeam(name: string) {
  const user = await requireUser();
  const client = await db.connect();
  try {
    await client.query("begin");
    const team = await client.query<{ id: number }>("insert into teams (name) values ($1) returning id", [name]);
    await client.query("insert into team_members (team_id, user_id, role) values ($1, $2, 'owner')", [team.rows[0].id, user.id]);
    await client.query("commit");
  } catch (error) {
    await client.query("rollback");
    throw error;
  } finally {
    client.release();
  }
}

export async function createProject(teamId: number, name: string) {
  const user = await requireUser();
  const allowed = await query(
    `select 1 from team_members where team_id = $1 and user_id = $2 and role in ('owner', 'admin')`,
    [teamId, user.id]
  );
  if (!allowed.rowCount) throw new Error("FORBIDDEN");
  const result = await query<{ id: number }>(
    `insert into projects (team_id, name, created_by_user_id) values ($1, $2, $3) returning id`,
    [teamId, name, user.id]
  );
  return result.rows[0];
}
