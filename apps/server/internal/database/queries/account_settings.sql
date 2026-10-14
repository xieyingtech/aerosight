-- name: LockAccount :one
SELECT password, credential_version FROM users WHERE id = $1 FOR UPDATE;

-- name: UpdateProfileName :exec
UPDATE users SET name=$2, updated_at=now() WHERE id=$1;

-- name: ReadAccountPassword :one
SELECT password, credential_version FROM users WHERE id=$1;

-- name: UpdateAccountPassword :execrows
UPDATE users SET password=sqlc.arg(new_password), credential_version=credential_version+1, updated_at=now()
WHERE id=sqlc.arg(user_id) AND password=sqlc.arg(old_password) AND credential_version=sqlc.arg(credential_version);

-- name: UpdateProjectName :one
UPDATE projects SET name=$3, updated_at=now() WHERE id=$1 AND team_id=$2 RETURNING id, name;
