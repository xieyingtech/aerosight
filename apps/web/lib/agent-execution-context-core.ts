const scopeArgumentNames = new Set([
  "userId",
  "teamId",
  "projectId",
  "sessionId",
  "user_id",
  "team_id",
  "project_id",
  "session_id"
]);

export type AgentExecutionContext = Readonly<{
  userId: number;
  teamId: number;
  projectId: number;
  sessionId: number;
}>;

export function createAgentExecutionContext(scope: AgentExecutionContext): AgentExecutionContext {
  for (const [name, value] of Object.entries(scope)) {
    if (!Number.isInteger(value) || value <= 0) throw new Error(`AGENT_CONTEXT_INVALID_${name.toUpperCase()}`);
  }
  return Object.freeze({ ...scope });
}

export function assertAgentToolArgsDoNotContainScope(args: unknown): void {
  inspect(args, new Set<object>());
}

function inspect(value: unknown, seen: Set<object>): void {
  if (!value || typeof value !== "object") return;
  if (seen.has(value)) return;
  seen.add(value);

  if (Array.isArray(value)) {
    for (const item of value) inspect(item, seen);
    return;
  }

  for (const [key, nested] of Object.entries(value)) {
    if (scopeArgumentNames.has(key)) throw new Error(`AGENT_TOOL_SCOPE_ARGUMENT_FORBIDDEN:${key}`);
    inspect(nested, seen);
  }
}
