-- name: TaskWorkbenchTask :many
SELECT to_jsonb(r) FROM (
select id,name,description,status,current_published_version_id as "currentPublishedVersionId"
    from tasks where project_id=sqlc.arg(p1) and id=sqlc.arg(p2)
) r;

-- name: TaskWorkbenchVersions :many
SELECT to_jsonb(r) FROM (
select id::int,version,status,definition_json as definition,
    input_schema_json as "inputSchema",trigger_json as trigger,concurrency_limit as "concurrencyLimit",
    created_at as "createdAt",published_at as "publishedAt"
    from task_versions where project_id=sqlc.arg(p1) and task_id=sqlc.arg(p2) order by version desc
) r;

-- name: TaskWorkbenchSteps :many
SELECT to_jsonb(r) FROM (
select id::int,position,step_key as key,name,uses,
    capability_code as "capabilityCode",action,parameters_json as "with",input_schema_json as "inputSchema",
    output_schema_json as "outputSchema",condition_json as condition,depends_on_json as "dependsOn",
    timeout_seconds as "timeoutSeconds",retry_policy_json as retry,failure_policy_json->>'onFailure' as "onFailure"
    from task_steps where project_id=sqlc.arg(p1) and task_version_id=sqlc.arg(p2) order by position
) r;
