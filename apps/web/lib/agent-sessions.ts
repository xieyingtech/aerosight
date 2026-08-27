import "server-only";

import { withAuditedProjectWrite } from "@/lib/audit";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { sanitizeAgentMessageForStorage } from "@/lib/agent-message-retention";
import { correlationId } from "@/lib/observability";

export type AgentSessionView = {
  id: number;
  status: string;
  summary: string | null;
  createdAt: Date;
  messages: Array<{ id: number; role: string; content: string; toolCalls: unknown; createdAt: Date }>;
};

export async function listAgentSessions(projectId: number): Promise<AgentSessionView[]> {
  const { user } = await requireCurrentProjectPermission(projectId, "agent:use");
  const sessions = (await query<Omit<AgentSessionView, "messages">>(
    `select id,status,summary,created_at as "createdAt" from agent_sessions
      where project_id=$1 and started_by_user_id=$2 order by created_at desc limit 50`, [projectId,user.id]
  )).rows;
  if (!sessions.length) return [];
  const messages = (await query<{ id: number; sessionId: number; role: string; content: string; toolCalls: unknown; createdAt: Date }>(
    `select message.id,message.session_id as "sessionId",message.role,message.content,message.tool_calls_json as "toolCalls",message.created_at as "createdAt"
       from agent_messages message join agent_sessions session on session.id=message.session_id
      where session.project_id=$1 and session.started_by_user_id=$2 and session.id=any($3::int[])
      order by message.created_at`, [projectId,user.id,sessions.map((session)=>session.id)]
  )).rows;
  return sessions.map((session) => ({ ...session, messages: messages.filter((message) => message.sessionId === session.id) }));
}

export async function createAgentSession(projectId: number, requestId?: string | null) {
  const { user,access } = await requireCurrentProjectPermission(projectId,"agent:use");
  return withAuditedProjectWrite({projectId,teamId:access.teamId,actorUserId:user.id,requestId:correlationId(requestId),
    action:"agent_session.create",resourceType:"agent_session",input:{},policyResult:{permission:"agent:use"}},async(client)=>
    (await client.query<{id:number}>(`insert into agent_sessions(project_id,status,started_by_user_id) values($1,'open',$2) returning id`,[projectId,user.id])).rows[0]);
}

export async function appendAgentMessage(input:{projectId:number;sessionId:number;role:"user"|"assistant";content:string;toolCalls?:unknown;requestId?:string|null}){
  const {user,access}=await requireCurrentProjectPermission(input.projectId,"agent:use");
  const stored=sanitizeAgentMessageForStorage(input);
  return withAuditedProjectWrite({projectId:input.projectId,teamId:access.teamId,actorUserId:user.id,requestId:correlationId(input.requestId),
    action:"agent_message.append",resourceType:"agent_session",resourceId:String(input.sessionId),input:{role:input.role},policyResult:{permission:"agent:use",retention:"minimal"}},async(client)=>{
    const session=(await client.query(`select id from agent_sessions where id=$1 and project_id=$2 and started_by_user_id=$3 and status='open' for update`,[input.sessionId,input.projectId,user.id])).rows[0];
    if(!session)throw new Error("AGENT_SESSION_NOT_FOUND");
    return (await client.query(`insert into agent_messages(session_id,role,content,tool_calls_json) values($1,$2,$3,$4) returning id,created_at as "createdAt"`,[input.sessionId,input.role,stored.content,stored.toolCalls])).rows[0];
  });
}
