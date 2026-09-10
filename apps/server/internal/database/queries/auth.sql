-- name: FindLoginUser :one
SELECT id, name, email, phone, password, role FROM users
WHERE lower(email) = lower(sqlc.arg(username)::text) OR phone = sqlc.arg(username)::text
LIMIT 1;

-- name: GetUser :one
SELECT id, name, email, phone, role FROM users WHERE id = $1;

-- name: HasUsers :one
SELECT EXISTS(SELECT 1 FROM users);

-- name: CreateDefaultAdmin :exec
INSERT INTO users (name, email, password, role) VALUES ('admin', 'admin@example.com', $1, 'admin');

-- name: GetProjectAccess :one
SELECT p.id AS project_id, p.team_id, tm.role,
 coalesce(array_agg(pp.permission) FILTER (WHERE pp.permission IS NOT NULL), '{}'::text[])::text[] AS permissions
FROM projects p
JOIN team_members tm ON tm.team_id = p.team_id AND tm.user_id = sqlc.arg(user_id)
LEFT JOIN project_permissions pp ON pp.project_id = p.id AND pp.team_id = p.team_id AND pp.user_id = tm.user_id
WHERE p.id = sqlc.arg(project_id)
GROUP BY p.id, p.team_id, tm.role;
