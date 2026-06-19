-- name: AdminOverview :one
select
  (select count(*) from users) as users,
  (select count(*) from teams) as teams,
  (select count(*) from projects) as projects;

-- name: ListAdminUsers :many
select id, name, email, phone, role, created_at, updated_at
from users
order by created_at desc;

-- name: ListAdminProjects :many
select p.id, p.name, p.description, p.team_id, t.name as team_name, p.created_by_user_id, u.name as created_by_user_name, p.created_at, p.updated_at
from projects p
inner join teams t on t.id = p.team_id
left join users u on u.id = p.created_by_user_id
order by p.created_at desc;
