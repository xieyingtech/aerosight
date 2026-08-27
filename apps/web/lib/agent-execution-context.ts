import "server-only";

import { query } from "@/lib/db";
import { requireProjectPermissionForUser } from "@/lib/project-access";
import {
  createAgentExecutionContext,
  type AgentExecutionContext
} from "@/lib/agent-execution-context-core";

type SessionBinding = {
  sessionId: number;
  projectId: number;
  startedByUserId: number;
};

export async function bindAgentExecutionContext(input: {
  authenticatedUserId: number;
  projectId: number;
  sessionId: number;
}): Promise<AgentExecutionContext> {
  const access = await requireProjectPermissionForUser(input.authenticatedUserId, input.projectId, "agent:use");
  const binding = (await query<SessionBinding>(
    `select id as "sessionId", project_id as "projectId", started_by_user_id as "startedByUserId"
       from agent_sessions
      where id = $1 and project_id = $2 and started_by_user_id = $3 and status = 'open'`,
    [input.sessionId, access.projectId, input.authenticatedUserId]
  )).rows[0];
  if (!binding) throw new Error("AGENT_SESSION_SCOPE_MISMATCH");

  return createAgentExecutionContext({
    userId: binding.startedByUserId,
    teamId: access.teamId,
    projectId: binding.projectId,
    sessionId: binding.sessionId
  });
}
