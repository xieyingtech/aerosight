import { assertAgentToolArgsDoNotContainScope, type AgentExecutionContext } from "./agent-execution-context-core.ts";
import { agentToolRegistry, parseAgentToolInput, type AgentToolName } from "./agent-tool-registry.ts";

export const AGENT_TOOL_RESULT_LIMIT = 100;
export const AGENT_TOOL_RESULT_BYTES = 64 * 1024;

export type AgentReadToolName = Extract<AgentToolName,
  "query_devices" | "query_missions" | "query_events" | "query_assets" | "query_tracks" | "query_map_context"
>;

const referenceType: Record<AgentReadToolName, string> = {
  query_devices: "device",
  query_missions: "task-run",
  query_events: "issue",
  query_assets: "asset",
  query_tracks: "track",
  query_map_context: "map-context"
};

export function prepareAgentReadToolCall(context: AgentExecutionContext, name: AgentReadToolName, rawInput: unknown) {
  if (agentToolRegistry[name].risk !== "read-only") throw new Error("AGENT_TOOL_NOT_READ_ONLY");
  assertAgentToolArgsDoNotContainScope(rawInput);
  return { projectId: context.projectId, input: parseAgentToolInput(name, rawInput) as Record<string, unknown> };
}

export function formatAgentReadToolResult(
  context: AgentExecutionContext,
  name: AgentReadToolName,
  rows: readonly Record<string, unknown>[],
  observedAt = new Date()
) {
  let truncated = rows.length > AGENT_TOOL_RESULT_LIMIT;
  const items: Record<string, unknown>[] = [];
  for (const row of rows.slice(0, AGENT_TOOL_RESULT_LIMIT)) {
    const id = String(row.id ?? row.deviceId ?? "current");
    const item = {
      ...row,
      reference: {
        type: referenceType[name],
        id,
        href: referenceHref(context.projectId, name, id)
      }
    };
    if (Buffer.byteLength(JSON.stringify([...items, item]), "utf8") > AGENT_TOOL_RESULT_BYTES) {
      truncated = true;
      break;
    }
    items.push(item);
  }

  const newest = newestTimestamp(items);
  return {
    projectId: context.projectId,
    observedAt: observedAt.toISOString(),
    quality: "authoritative-project-query",
    freshnessSeconds: newest === null ? null : Math.max(0, Math.floor((observedAt.getTime() - newest) / 1000)),
    truncated,
    items
  };
}

function referenceHref(projectId: number, name: AgentReadToolName, id: string) {
  const base = `/projects/${projectId}`;
  if (name === "query_devices") return `${base}/devices?selected=${encodeURIComponent(id)}`;
  if (name === "query_missions") return `${base}/tasks/runs/${encodeURIComponent(id)}`;
  if (name === "query_events") return `${base}/issues/${encodeURIComponent(id)}`;
  if (name === "query_assets") return `${base}/assets?selected=${encodeURIComponent(id)}`;
  return `${base}?selected=${encodeURIComponent(id)}`;
}

function newestTimestamp(items: readonly Record<string, unknown>[]) {
  let newest: number | null = null;
  for (const item of items) {
    for (const key of ["observedAt", "capturedAt", "updatedAt", "createdAt", "lastSeenAt"]) {
      const value = item[key];
      if (typeof value !== "string" && !(value instanceof Date)) continue;
      const time = new Date(value).getTime();
      if (Number.isFinite(time) && (newest === null || time > newest)) newest = time;
    }
  }
  return newest;
}
