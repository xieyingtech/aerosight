-- name: LockTaskTrigger :exec
SELECT pg_advisory_xact_lock($1::integer,$2::integer);

-- name: ReadTaskTriggerVersion :one
SELECT task.project_id,task.team_id,task.id AS task_id,task.status AS task_status,
 version.id AS task_version_id,version.status AS task_version_status,version.trigger_json,
 version.input_schema_json,version.concurrency_limit
FROM tasks task JOIN task_versions version ON version.id=task.current_published_version_id AND version.project_id=task.project_id
WHERE task.project_id=$1 AND task.id=$2 FOR UPDATE OF task,version;

-- name: ReadTriggeredRun :one
SELECT id,status FROM task_runs WHERE project_id=$1 AND task_version_id=$2 AND trigger_key=$3;

-- name: CountActiveTriggeredRuns :one
SELECT count(*) FROM task_runs WHERE project_id=$1 AND task_version_id=$2
AND status IN ('queued','blocked','ready','dispatching','running','paused','canceling');

-- name: InsertTriggeredRun :one
INSERT INTO task_runs(project_id,team_id,task_id,task_version_id,trigger_source,trigger_key,status,input_snapshot_json,created_by_user_id,state_reason)
VALUES($1,$2,$3,$4,$5,$6,'queued',$7,$8,'trigger-accepted') RETURNING id,status;

-- name: InsertTriggeredSteps :exec
INSERT INTO task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status,execution_key)
SELECT step.project_id,step.team_id,$3,step.id,step.position,'pending',$4::text||':step:'||step.step_key
FROM task_steps step WHERE step.project_id=$1 AND step.task_version_id=$2 ORDER BY step.position;
