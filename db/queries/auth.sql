-- name: FindUserByEmail :one
select id, name, email, phone, password, role, created_at, updated_at
from users
where email = $1
limit 1;

-- name: FindUserByPhone :one
select id, name, email, phone, password, role, created_at, updated_at
from users
where phone = $1
limit 1;

-- name: FindUserByEmailOrPhone :one
select id, name, email, phone, password, role, created_at, updated_at
from users
where email = $1 or phone = $2
limit 1;
