-- name: LockPlatformUserRole :one
SELECT role FROM users WHERE id=$1 FOR SHARE;

-- name: LockAIProviderRegistry :exec
SELECT pg_advisory_xact_lock(hashtext('aerosight.ai-provider.registry'));

-- name: ListAIProviders :many
SELECT to_jsonb(result) FROM (
 SELECT id::text,name,provider_type AS "providerType",base_url AS "baseUrl",model_id AS "modelId",enabled,is_default AS "isDefault",status,health_json AS health,last_tested_at AS "lastTestedAt",updated_at AS "updatedAt"
 FROM ai_providers ORDER BY name
) result;

-- name: ReadAIProviderPublic :one
SELECT to_jsonb(result) FROM (
 SELECT id::text,name,provider_type AS "providerType",base_url AS "baseUrl",model_id AS "modelId",enabled,is_default AS "isDefault",status,health_json AS health,last_tested_at AS "lastTestedAt",updated_at AS "updatedAt"
 FROM ai_providers WHERE id=$1
) result;

-- name: ClearAIProviderDefault :exec
UPDATE ai_providers SET is_default=false,updated_at=now() WHERE is_default;

-- name: CreateAIProvider :one
INSERT INTO ai_providers(name,provider_type,base_url,model_id,credential_envelope_json,enabled,is_default,created_by_user_id,updated_by_user_id)
VALUES($1,'openai',$2,$3,'{}',$4,$5,$6,$6) RETURNING id;

-- name: LockAIProvider :one
SELECT id,base_url,model_id,credential_envelope_json,enabled FROM ai_providers WHERE id=$1 FOR UPDATE;

-- name: UpdateAIProvider :exec
UPDATE ai_providers SET name=$2,provider_type='openai',base_url=$3,model_id=$4,enabled=$5,is_default=$6,updated_by_user_id=$7,updated_at=now() WHERE id=$1;

-- name: SetAIProviderCredential :exec
UPDATE ai_providers SET credential_envelope_json=$2,status='untested' WHERE id=$1;

-- name: DeleteAIProvider :execrows
DELETE FROM ai_providers WHERE id=$1;

-- name: SetAIProviderHealth :exec
UPDATE ai_providers SET status=$2,health_json=$3,last_tested_at=now(),updated_by_user_id=$4,updated_at=now() WHERE id=$1;
