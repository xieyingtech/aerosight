-- name: ReadAlgorithmRunSource :one
SELECT definition.team_id,version.id AS configuration_snapshot_id,provider.provider_type,version.model_or_process,
 version.execution_mode,coalesce(version.protocol_config_json->>'mappingVersion','v1')::text AS mapping_version,
 asset.id AS asset_id,asset.version AS asset_version,coalesce(asset.checksum_sha256,asset.checksum,'')::text AS checksum_sha256,
 coalesce(asset.mime_type,'application/octet-stream')::text AS mime_type
FROM algorithm_definition_versions version
JOIN algorithm_definitions definition ON definition.id=version.algorithm_definition_id AND definition.project_id=version.project_id AND definition.current_published_version_id=version.id
JOIN algorithm_providers provider ON provider.id=definition.provider_id AND provider.project_id=definition.project_id AND provider.status='active'
JOIN assets asset ON asset.project_id=version.project_id AND asset.id=sqlc.arg(asset_id) AND asset.status='available'
WHERE version.project_id=sqlc.arg(project_id) AND version.id=sqlc.arg(snapshot_id) AND version.status='published'
FOR SHARE OF version,definition,provider,asset;

-- name: InsertCatalogAlgorithmRun :exec
INSERT INTO algorithm_runs(id,project_id,team_id,algorithm_definition_version_id,input_asset_id,task_run_id,device_id,idempotency_key,parameters_json,input_snapshot_json)
VALUES($1,$2,$3,$4,$5,sqlc.narg(task_run_id),sqlc.narg(device_id),sqlc.arg(idempotency_key),sqlc.arg(parameters),sqlc.arg(snapshot));

-- name: LockAlgorithmRetrySource :one
SELECT team_id,algorithm_definition_version_id,input_asset_id,task_run_id,device_id,parameters_json,input_snapshot_json,status
FROM algorithm_runs WHERE project_id=$1 AND id=$2 FOR UPDATE;

-- name: ListAlgorithmRuns :many
SELECT to_jsonb(result) FROM (
 SELECT run.id,run.status,run.input_asset_id AS "inputAssetId",run.task_run_id AS "taskRunId",run.device_id AS "deviceId",
 run.canonical_result_json AS "canonicalResult",run.external_job_id AS "externalJobId",run.raw_result_object_key AS "rawResultObjectKey",
 run.raw_result_checksum_sha256 AS "rawResultChecksumSha256",run.error_code AS "errorCode",run.error_message AS "errorMessage",
 run.created_at AS "createdAt",run.started_at AS "startedAt",run.finished_at AS "finishedAt",definition.name AS "definitionName",
 provider.name AS "providerName",provider.provider_type AS "providerType"
 FROM algorithm_runs run
 JOIN algorithm_definition_versions version ON version.id=run.algorithm_definition_version_id AND version.project_id=run.project_id
 JOIN algorithm_definitions definition ON definition.id=version.algorithm_definition_id AND definition.project_id=run.project_id
 JOIN algorithm_providers provider ON provider.id=definition.provider_id AND provider.project_id=run.project_id
 WHERE run.project_id=$1 ORDER BY run.created_at DESC LIMIT 100
) result;

-- name: ReadAlgorithmRunDetail :one
SELECT to_jsonb(result) FROM (
 SELECT run.id,run.status,run.input_asset_id AS "inputAssetId",run.task_run_id AS "taskRunId",run.device_id AS "deviceId",
 run.input_snapshot_json AS "inputSnapshot",run.canonical_result_json AS "canonicalResult",run.external_job_id AS "externalJobId",run.raw_result_object_key AS "rawResultObjectKey",
 run.raw_result_checksum_sha256 AS "rawResultChecksumSha256",run.error_code AS "errorCode",run.error_message AS "errorMessage",
 run.created_at AS "createdAt",run.started_at AS "startedAt",run.finished_at AS "finishedAt",definition.name AS "definitionName",
 provider.name AS "providerName",provider.provider_type AS "providerType"
 FROM algorithm_runs run
 JOIN algorithm_definition_versions version ON version.id=run.algorithm_definition_version_id AND version.project_id=run.project_id
 JOIN algorithm_definitions definition ON definition.id=version.algorithm_definition_id AND definition.project_id=run.project_id
 JOIN algorithm_providers provider ON provider.id=definition.provider_id AND provider.project_id=run.project_id
 WHERE run.project_id=$1 AND run.id=$2
) result;

-- name: ReadAlgorithmRunAttempts :many
SELECT to_jsonb(result) FROM (
 SELECT attempt,status,response_status AS "responseStatus",duration_ms AS "durationMs",error_category AS "errorCategory",external_job_id AS "externalJobId",started_at AS "startedAt",finished_at AS "finishedAt"
 FROM algorithm_run_attempts WHERE project_id=$1 AND algorithm_run_id=$2 ORDER BY attempt
) result;
