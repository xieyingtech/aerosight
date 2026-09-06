-- name: FindHTTPSession :one
SELECT data FROM sessions WHERE token = $1 AND current_timestamp < expiry;

-- name: CommitHTTPSession :exec
INSERT INTO sessions(token, data, expiry) VALUES ($1, $2, $3)
ON CONFLICT (token) DO UPDATE SET data = EXCLUDED.data, expiry = EXCLUDED.expiry;

-- name: DeleteHTTPSession :exec
DELETE FROM sessions WHERE token = $1;
