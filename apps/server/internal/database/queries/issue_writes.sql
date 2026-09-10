-- name: LockIssueMutation :one
SELECT state_version FROM issues WHERE project_id=$1 AND id=$2 FOR UPDATE;

-- name: IssueMutationReplayed :one
SELECT EXISTS(SELECT 1 FROM issue_events WHERE project_id=$1 AND issue_id=$2 AND client_key=$3)::boolean;

-- name: LockIssueAssigneeUser :one
SELECT user_id FROM team_members WHERE team_id=$1 AND user_id=$2 FOR SHARE;

-- name: LockIssueAssigneeAgent :one
SELECT id,name,coalesce(config_json->>'kind','')::text AS kind FROM agents WHERE project_id=$1 AND id=$2 AND status='active' FOR SHARE;

-- name: IssueAssigneeActive :one
SELECT EXISTS(SELECT 1 FROM issue_assignees WHERE project_id=$1 AND issue_id=$2
 AND (user_id=sqlc.narg(user_id) OR agent_id=sqlc.narg(agent_id)) AND active)::boolean;

-- name: AddIssueAssignee :exec
INSERT INTO issue_assignees(project_id,team_id,issue_id,assignee_type,user_id,agent_id,assigned_by_user_id)
VALUES($1,$2,$3,$4,sqlc.narg(user_id),sqlc.narg(agent_id),sqlc.arg(actor_user_id));

-- name: RemoveIssueAssignee :exec
UPDATE issue_assignees SET active=false,removed_at=now() WHERE project_id=$1 AND issue_id=$2
 AND (user_id=sqlc.narg(user_id) OR agent_id=sqlc.narg(agent_id)) AND active;

-- name: UpdateIssueMutation :one
UPDATE issues SET status=coalesce(sqlc.narg(status)::text,status),labels_json=coalesce(sqlc.narg(labels)::jsonb,labels_json),
 state_version=sqlc.arg(next_version),updated_at=now(),
 closed_at=CASE WHEN sqlc.narg(status)::text='closed' THEN now() WHEN sqlc.narg(status)::text='open' THEN null ELSE closed_at END
WHERE project_id=sqlc.arg(project_id) AND id=sqlc.arg(issue_id) AND state_version=sqlc.arg(expected_version)
RETURNING state_version;

-- name: InsertIssueActivity :one
INSERT INTO issue_events(project_id,issue_id,event_type,body,metadata_json,actor_user_id,client_key)
VALUES($1,$2,$3,sqlc.narg(body),sqlc.arg(metadata),sqlc.arg(actor_user_id),sqlc.narg(client_key)) RETURNING id;

-- name: EnsureIssueCopilot :one
INSERT INTO agents(project_id,name,description,status,config_json)
VALUES($1,'Copilot','项目级 AI 助手，可通过案件评论提及或负责人指派触发。','active','{"kind":"copilot","builtIn":true}'::jsonb)
ON CONFLICT(project_id,(config_json->>'kind')) WHERE config_json->>'kind'='copilot'
DO UPDATE SET status='active' RETURNING id;

-- name: CreateIssueCopilotSession :one
INSERT INTO agent_sessions(project_id,agent_id,issue_id,started_by_user_id,summary)
VALUES($1,$2,$3,$4,$5) RETURNING id;

-- name: QueueIssueCopilot :one
INSERT INTO agent_tool_jobs(project_id,team_id,session_id,requested_by_user_id,issue_id,trigger_issue_event_id,trigger_type,idempotency_key,
 tool_name,required_permission,args_json,context_expires_at)
VALUES(sqlc.arg(project_id),sqlc.arg(team_id),sqlc.arg(session_id),sqlc.arg(actor_user_id),sqlc.arg(issue_id),sqlc.arg(activity_id),sqlc.arg(trigger_type),sqlc.arg(idempotency_key),
 'issue_copilot','agent:use',jsonb_build_object('issueId',sqlc.arg(issue_id)::int,'triggerEventId',sqlc.arg(activity_id)::int),now()+interval '24 hours')
ON CONFLICT(project_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET idempotency_key=excluded.idempotency_key RETURNING id;
