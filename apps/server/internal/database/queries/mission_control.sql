-- name: LockMissionControlRun :one
SELECT status,state_version,approval_request_id FROM task_runs WHERE project_id=$1 AND id=$2 FOR UPDATE;

-- name: ApproveMissionControlRun :exec
INSERT INTO approvals(project_id,team_id,approval_request_id,approver_user_id,decision,reason)
VALUES($1,$2,$3,$4,'approved',$5);

-- name: UpdateMissionControlRun :one
UPDATE task_runs SET status=$4,state_version=state_version+1,state_reason=$5
WHERE project_id=$1 AND id=$2 AND state_version=$3 RETURNING id,status,state_version;
