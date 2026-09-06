-- name: InsertProjectAudit :one
INSERT INTO audit_events(project_id,team_id,request_id,idempotency_key,actor_user_id,actor_agent_id,action,resource_type,resource_id,input_hash,policy_result_json)
VALUES(sqlc.arg(project_id),sqlc.arg(team_id),sqlc.arg(request_id),sqlc.narg(idempotency_key),sqlc.narg(actor_user_id),sqlc.narg(actor_agent_id),sqlc.arg(action),sqlc.arg(resource_type),sqlc.narg(resource_id),sqlc.arg(input_hash),sqlc.arg(policy_result)) RETURNING id;

-- name: CompleteProjectAudit :execrows
UPDATE audit_events SET status='completed',result_hash=$2,completed_at=now() WHERE id=$1 AND project_id=$3;

-- name: InsertPlatformAudit :one
INSERT INTO platform_audit_events(actor_user_id,request_id,action,resource_type,resource_id,input_hash)
VALUES($1,$2,$3,$4,$5,$6) RETURNING id;

-- name: CompletePlatformAudit :execrows
UPDATE platform_audit_events SET status='completed',result_hash=$2,completed_at=now() WHERE id=$1;

-- name: PublishProjectEvent :one
INSERT INTO project_events(project_id,team_id,event_id,event_type,payload_json,occurred_at)
VALUES($1,$2,$3,$4,$5,coalesce(sqlc.narg(occurred_at)::timestamptz,now()))
ON CONFLICT(event_id) DO UPDATE SET event_id=excluded.event_id RETURNING cursor;

-- name: EnqueueProjectEvent :exec
INSERT INTO outbox_events(project_id,team_id,event_id,event_type,payload_json,max_attempts)
VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(event_id) DO NOTHING;

-- name: ReserveIdempotency :one
INSERT INTO idempotency_records(project_id,team_id,actor_key,operation,idempotency_key,request_hash)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(project_id,actor_key,operation,idempotency_key) DO NOTHING RETURNING id;

-- name: ReadIdempotency :one
SELECT request_hash,status,response_json,error_code FROM idempotency_records
WHERE project_id=$1 AND actor_key=$2 AND operation=$3 AND idempotency_key=$4;

-- name: CompleteIdempotency :execrows
UPDATE idempotency_records SET status='completed',response_json=$2,completed_at=now() WHERE id=$1;

-- name: LockProjectMembership :one
SELECT p.id AS project_id,p.team_id,tm.role FROM projects p
JOIN team_members tm ON tm.team_id=p.team_id AND tm.user_id=sqlc.arg(user_id)
WHERE p.id=sqlc.arg(project_id) FOR SHARE OF p,tm;

-- name: LockProjectPermissions :many
SELECT permission FROM project_permissions
WHERE project_id=$1 AND team_id=$2 AND user_id=$3 FOR SHARE;
