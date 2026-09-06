-- name: ReadAlgorithmAccessAsset :one
SELECT storage_key, mime_type FROM assets
WHERE id=$1 AND project_id=$2 AND version=$3 AND status='available' AND deleted_at IS NULL;
