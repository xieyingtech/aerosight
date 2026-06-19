-- name: GetProfile :many
select id, name, email, phone, role, created_at
from users
where id = $1;

-- name: ListProfileTeams :many
select t.id, t.name, tm.role, tm.created_at as joined_at
from teams t
inner join team_members tm on tm.team_id = t.id and tm.user_id = $1
order by t.name asc;

-- name: ListManagedTeams :many
select t.id, t.name
from teams t
inner join team_members tm on tm.team_id = t.id and tm.user_id = $1 and tm.role in ('owner', 'admin')
order by t.name asc;
