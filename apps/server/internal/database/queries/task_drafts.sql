-- name: TaskDraftExisting :many
SELECT to_jsonb(r) FROM (
select id, project_id as "projectId", task_id as "taskId", version, status,
  definition_json as definition, script, input_schema_json as "inputSchema", trigger_json as trigger,
  concurrency_limit as "concurrencyLimit" from task_versions where project_id = sqlc.arg(p1) and task_id = sqlc.arg(p2) and status = 'draft'
) r;

-- name: TaskDraftTask :many
SELECT to_jsonb(r) FROM (
select task.team_id as "teamId", task.current_published_version_id as "currentVersionId",
                task.script, jsonb_build_object('name', task.name, 'description', task.description,
                  'triggerType', task.trigger_type, 'targetSelector', task.target_selector_json,
                  'schedule', task.schedule, 'eventRule', task.event_rule_json) as definition
           from tasks task where task.project_id = sqlc.arg(p1) and task.id = sqlc.arg(p2) for update
) r;

-- name: TaskDraftSource :many
SELECT to_jsonb(r) FROM (
select definition_json as definition, script, input_schema_json as "inputSchema", trigger_json as trigger,
                  concurrency_limit as "concurrencyLimit" from task_versions where project_id = sqlc.arg(p1) and id = sqlc.arg(p2)
) r;

-- name: TaskDraftCreate :one
insert into task_versions (
           project_id, team_id, task_id, version, status, definition_json, script, input_schema_json,
           trigger_json, concurrency_limit, created_by_user_id
         ) values (sqlc.arg(p1), sqlc.arg(p2), sqlc.arg(p3), sqlc.arg(p4), 'draft', sqlc.arg(p5), sqlc.arg(p6), sqlc.arg(p7), sqlc.arg(p8), sqlc.arg(p9), sqlc.arg(p10)) returning id, project_id as "projectId", task_id as "taskId", version, status,
  definition_json as definition, script, input_schema_json as "inputSchema", trigger_json as trigger,
  concurrency_limit as "concurrencyLimit";

-- name: TaskDraftCopySteps :exec
insert into task_steps (
             project_id, team_id, task_version_id, position, step_key, name, capability_code,
             action, parameters_json, failure_policy_json, media_requirements_json, uses,
             input_schema_json, output_schema_json, condition_json, depends_on_json, timeout_seconds, retry_policy_json
           ) select project_id, team_id, sqlc.arg(p3), position, step_key, name, capability_code,
                    action, parameters_json, failure_policy_json, media_requirements_json, uses,
                    input_schema_json, output_schema_json, condition_json, depends_on_json, timeout_seconds, retry_policy_json
               from task_steps where task_steps.project_id = sqlc.arg(p1) and task_steps.task_version_id = sqlc.arg(p2);

-- name: TaskDraftLockVersion :many
SELECT to_jsonb(r) FROM (
select id, project_id as "projectId", task_id as "taskId", version, status,
  definition_json as definition, script, input_schema_json as "inputSchema", trigger_json as trigger,
  concurrency_limit as "concurrencyLimit" from task_versions where project_id = sqlc.arg(p1) and id = sqlc.arg(p2) for update
) r;

-- name: TaskDraftSteps :many
SELECT to_jsonb(r) FROM (
select position, step_key as "stepKey", name, capability_code as "capabilityCode", action,
                parameters_json as parameters, failure_policy_json as "failurePolicy",
                media_requirements_json as "mediaRequirements" from task_steps
          where project_id = sqlc.arg(p1) and task_version_id = sqlc.arg(p2) order by position
) r;

-- name: TaskDraftPublish :one
update task_versions set status = 'published', published_by_user_id = sqlc.arg(p3), published_at = now()
          where project_id = sqlc.arg(p1) and id = sqlc.arg(p2) and status = 'draft' returning id, project_id as "projectId", task_id as "taskId", version, status,
  definition_json as definition, script, input_schema_json as "inputSchema", trigger_json as trigger,
  concurrency_limit as "concurrencyLimit";

-- name: TaskDraftUpdateTask :exec
update tasks set current_published_version_id=sqlc.arg(p3),name=coalesce(nullif(sqlc.arg(p4),''),name),
          description=coalesce(sqlc.arg(p5),description),trigger_type=sqlc.arg(p6),updated_at=now() where project_id=sqlc.arg(p1) and id=sqlc.arg(p2);

-- name: TaskDraftLockDraft :many
SELECT to_jsonb(r) FROM (
select id from task_versions where project_id=sqlc.arg(p1) and task_id=sqlc.arg(p2) and id=sqlc.arg(p3) and status='draft' for update
) r;

-- name: TaskDraftSave :exec
update task_versions set definition_json=sqlc.arg(p4),script='typed-task-v1',input_schema_json=sqlc.arg(p5),
      trigger_json=sqlc.arg(p6),concurrency_limit=sqlc.arg(p7) where project_id=sqlc.arg(p1) and task_id=sqlc.arg(p2) and id=sqlc.arg(p3);

-- name: TaskDraftDeleteSteps :exec
delete from task_steps where project_id=sqlc.arg(p1) and task_version_id=sqlc.arg(p2);

-- name: TaskDraftInsertStep :exec
insert into task_steps(
        project_id,team_id,task_version_id,position,step_key,name,capability_code,action,parameters_json,
        failure_policy_json,media_requirements_json,uses,input_schema_json,output_schema_json,condition_json,
        depends_on_json,timeout_seconds,retry_policy_json)
        values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),sqlc.arg(p4),sqlc.arg(p5),sqlc.arg(p6),sqlc.arg(p7),sqlc.arg(p8),sqlc.arg(p9),sqlc.arg(p10),sqlc.arg(p11),sqlc.arg(p12),sqlc.arg(p13),sqlc.arg(p14),sqlc.arg(p15),sqlc.arg(p16),sqlc.arg(p17),sqlc.arg(p18));

-- name: TaskDraftList :many
SELECT to_jsonb(r) FROM (
select id, project_id as "projectId", task_id as "taskId", version, status,
  definition_json as definition, script, input_schema_json as "inputSchema", trigger_json as trigger,
  concurrency_limit as "concurrencyLimit" from task_versions where project_id = sqlc.arg(p1) and task_id = sqlc.arg(p2) order by version desc
) r;

-- name: TaskDraftNextVersion :one
select coalesce(max(version),0)::int+1 as version from task_versions where task_id=$1;
