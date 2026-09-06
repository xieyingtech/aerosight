-- name: ListAlgorithmProviders :many
SELECT to_jsonb(result) FROM (
 SELECT id::text,project_id AS "projectId",name,provider_type AS "providerType",base_url AS "baseUrl",auth_type AS "authType",
 allowed_headers_json AS "allowedHeaders",timeout_seconds AS "timeoutSeconds",concurrency_limit AS "concurrencyLimit",rate_limit_per_minute AS "rateLimitPerMinute",status,health_json AS health,updated_at AS "updatedAt"
 FROM algorithm_providers WHERE project_id=$1 ORDER BY name
) result;

-- name: ReadAlgorithmProviderPublic :one
SELECT to_jsonb(result) FROM (
 SELECT id::text,project_id AS "projectId",name,provider_type AS "providerType",base_url AS "baseUrl",auth_type AS "authType",
 allowed_headers_json AS "allowedHeaders",timeout_seconds AS "timeoutSeconds",concurrency_limit AS "concurrencyLimit",rate_limit_per_minute AS "rateLimitPerMinute",status,health_json AS health,updated_at AS "updatedAt"
 FROM algorithm_providers WHERE project_id=$1 AND id=$2
) result;

-- name: CreateAlgorithmProvider :one
INSERT INTO algorithm_providers(project_id,team_id,name,provider_type,base_url,auth_type,allowed_headers_json,timeout_seconds,concurrency_limit,rate_limit_per_minute,created_by_user_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id;

-- name: LockAlgorithmProviderCredential :one
SELECT auth_type,(credential_envelope_json IS NOT NULL)::boolean AS has_credential FROM algorithm_providers WHERE project_id=$1 AND id=$2 FOR UPDATE;

-- name: UpdateAlgorithmProvider :exec
UPDATE algorithm_providers SET name=$3,provider_type=$4,base_url=$5,auth_type=$6,allowed_headers_json=$7,timeout_seconds=$8,concurrency_limit=$9,rate_limit_per_minute=$10,updated_at=now() WHERE project_id=$1 AND id=$2;

-- name: SetAlgorithmProviderCredential :exec
UPDATE algorithm_providers SET credential_envelope_json=$3 WHERE project_id=$1 AND id=$2;

-- name: ReadAlgorithmProviderEndpoint :one
SELECT base_url,provider_type FROM algorithm_providers WHERE project_id=$1 AND id=$2;
