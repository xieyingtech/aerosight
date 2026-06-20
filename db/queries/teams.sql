-- name: ListTeams :many
select t.id, t.name, current_members.role, count(all_members.id) as member_count, t.created_at, t.updated_at
from teams t
inner join team_members current_members on current_members.team_id = t.id and current_members.user_id = $1
left join team_members all_members on all_members.team_id = t.id
where current_members.user_id = $1
group by t.id, t.name, t.created_at, t.updated_at, current_members.role
order by t.name asc;

-- name: CreateTeam :one
insert into teams (name)
values ($1)
returning id, name, created_at, updated_at;

-- name: CreateTeamOwner :exec
insert into team_members (team_id, user_id, role)
values ($1, $2, 'owner');

-- name: GetTeamForUser :one
select t.id, t.name, tm.role, count(all_members.id) as member_count, t.created_at, t.updated_at
from teams t
inner join team_members tm on tm.team_id = t.id and tm.user_id = $1
left join team_members all_members on all_members.team_id = t.id
where t.id = $2
group by t.id, t.name, t.created_at, t.updated_at, tm.role
limit 1;

-- name: ListTeamProjects :many
select id, name, description, updated_at
from projects
where team_id = $1
order by updated_at desc;
