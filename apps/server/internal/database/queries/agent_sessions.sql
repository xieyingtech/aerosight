-- name: ListChatSessions :many
SELECT id,status,summary,created_at FROM agent_sessions
WHERE project_id=$1 AND started_by_user_id=$2 ORDER BY created_at DESC,id DESC LIMIT 50;

-- name: ListChatMessages :many
SELECT message.id,message.session_id,message.role,message.content,message.tool_calls_json,message.created_at
FROM agent_messages message JOIN agent_sessions session ON session.id=message.session_id
WHERE session.project_id=$1 AND session.started_by_user_id=$2 AND session.id=ANY(sqlc.arg(session_ids)::int[])
ORDER BY message.created_at,message.id;

-- name: CreateChatSession :one
INSERT INTO agent_sessions(project_id,status,started_by_user_id) VALUES($1,'open',$2) RETURNING id;

-- name: LockChatSession :one
SELECT id FROM agent_sessions WHERE id=$1 AND project_id=$2 AND started_by_user_id=$3 AND status='open' FOR UPDATE;

-- name: AppendChatMessage :one
INSERT INTO agent_messages(session_id,role,content,tool_calls_json) VALUES($1,$2,$3,$4) RETURNING id,created_at;

-- name: RecentChatHistory :many
SELECT recent.role,recent.content FROM (
 SELECT message.id,message.role,message.content FROM agent_messages message
 JOIN agent_sessions session ON session.id=message.session_id
 WHERE session.id=$1 AND session.project_id=$2 AND session.started_by_user_id=$3 AND session.status='open'
 AND message.role IN ('user','assistant') ORDER BY message.id DESC LIMIT 20
) recent ORDER BY recent.id;

-- name: ReadOpenChatSession :one
SELECT id FROM agent_sessions WHERE id=$1 AND project_id=$2 AND started_by_user_id=$3 AND status='open';
