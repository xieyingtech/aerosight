-- name: GetProjectForUser :one
select p.id, p.name, p.description, p.team_id, t.name as team_name, tm.role, p.updated_at
from projects p
inner join teams t on p.team_id = t.id
inner join team_members tm on tm.team_id = t.id and tm.user_id = $1
where p.id = $2
limit 1;

-- name: ListProjectDevices :many
select id, name, type, status, last_seen_at, updated_at
from devices
where project_id = $1
order by updated_at desc;

-- name: ListProjectAgents :many
select id, name, description, status, updated_at
from agents
where project_id = $1
order by updated_at desc;

-- name: ListProjectTasks :many
select id, name, description, trigger_type, status, updated_at
from tasks
where project_id = $1
order by updated_at desc;

-- name: ListProjectIssues :many
select id, number, title, status, priority, updated_at
from issues
where project_id = $1
order by updated_at desc;

-- name: ListProjectAssets :many
select id, kind, mime_type, captured_at, created_at
from assets
where project_id = $1
order by created_at desc;
