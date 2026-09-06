-- name: ListTeams :many
SELECT t.id, t.name, current_members.role,
 count(all_members.id)::int AS member_count, t.created_at, t.updated_at
FROM teams t
JOIN team_members current_members ON current_members.team_id=t.id AND current_members.user_id=sqlc.arg(user_id)
LEFT JOIN team_members all_members ON all_members.team_id=t.id
WHERE (sqlc.arg(search)::text='' OR t.name ILIKE '%'||sqlc.arg(search)::text||'%')
 AND (sqlc.arg(scope)::text<>'managed' OR current_members.role IN ('owner','admin'))
GROUP BY t.id,current_members.role ORDER BY t.name;

-- name: GetTeam :one
SELECT t.id, t.name, current_members.role,
 count(all_members.id)::int AS member_count, t.created_at, t.updated_at
FROM teams t
JOIN team_members current_members ON current_members.team_id=t.id AND current_members.user_id=sqlc.arg(user_id)
LEFT JOIN team_members all_members ON all_members.team_id=t.id
WHERE t.id=sqlc.arg(team_id) GROUP BY t.id,current_members.role;

-- name: ListTeamProjects :many
SELECT id,name,description,updated_at FROM projects WHERE team_id=$1 ORDER BY updated_at DESC;

-- name: ListProjects :many
SELECT p.id,p.team_id,p.name,p.description,t.name AS team_name,tm.role,
 coalesce(array_agg(DISTINCT pp.permission) FILTER(WHERE pp.permission IS NOT NULL),'{}'::text[])::text[] AS permissions,
 p.updated_at
FROM projects p JOIN teams t ON t.id=p.team_id
JOIN team_members tm ON tm.team_id=t.id AND tm.user_id=sqlc.arg(user_id)
LEFT JOIN project_permissions pp ON pp.project_id=p.id AND pp.team_id=p.team_id AND pp.user_id=sqlc.arg(user_id)
WHERE (sqlc.arg(scope)::text<>'joined' OR tm.role='member')
 AND (sqlc.arg(scope)::text<>'managed' OR tm.role IN ('owner','admin'))
 AND (sqlc.arg(search)::text='' OR p.name ILIKE '%'||sqlc.arg(search)::text||'%' OR coalesce(p.description,'') ILIKE '%'||sqlc.arg(search)::text||'%' OR t.name ILIKE '%'||sqlc.arg(search)::text||'%')
GROUP BY p.id,t.name,tm.role ORDER BY p.updated_at DESC;

-- name: GetProject :one
SELECT p.id,p.team_id,p.name,p.description,t.name AS team_name,tm.role,
 coalesce(array_agg(DISTINCT pp.permission) FILTER(WHERE pp.permission IS NOT NULL),'{}'::text[])::text[] AS permissions,
 p.updated_at
FROM projects p JOIN teams t ON t.id=p.team_id
JOIN team_members tm ON tm.team_id=t.id AND tm.user_id=sqlc.arg(user_id)
LEFT JOIN project_permissions pp ON pp.project_id=p.id AND pp.team_id=p.team_id AND pp.user_id=sqlc.arg(user_id)
WHERE p.id=sqlc.arg(project_id) GROUP BY p.id,t.name,tm.role;

-- name: ListProfileTeams :many
SELECT t.id,t.name,tm.role,tm.created_at AS joined_at FROM teams t
JOIN team_members tm ON tm.team_id=t.id WHERE tm.user_id=$1 ORDER BY t.name;

-- name: AdminOverview :one
SELECT (SELECT count(*)::int FROM users) AS users,
 (SELECT count(*)::int FROM teams) AS teams,(SELECT count(*)::int FROM projects) AS projects;

-- name: ListAdminUsers :many
SELECT id,name,email,phone,role,created_at FROM users ORDER BY created_at DESC;

-- name: ListAdminTeams :many
SELECT t.id,t.name,count(DISTINCT tm.id)::int AS member_count,u.id AS owner_user_id,u.name AS owner_name
FROM teams t LEFT JOIN team_members tm ON tm.team_id=t.id
LEFT JOIN team_members owners ON owners.team_id=t.id AND owners.role='owner'
LEFT JOIN users u ON u.id=owners.user_id GROUP BY t.id,u.id,u.name ORDER BY t.name;

-- name: ListAdminProjects :many
SELECT p.id,p.name,p.description,t.name AS team_name,u.name AS created_by_name,p.created_at
FROM projects p JOIN teams t ON t.id=p.team_id LEFT JOIN users u ON u.id=p.created_by_user_id ORDER BY p.created_at DESC;

-- name: CreateTeam :one
INSERT INTO teams(name) VALUES($1) RETURNING id;

-- name: CreateTeamOwner :exec
INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner');

-- name: GetTeamManager :one
SELECT id FROM team_members WHERE team_id=$1 AND user_id=$2 AND role IN ('owner','admin') FOR UPDATE;

-- name: CreateProject :one
INSERT INTO projects(team_id,name,created_by_user_id) VALUES($1,$2,$3) RETURNING id;
