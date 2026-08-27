const sensitiveKey = /(?:secret|token|authorization|api[_-]?key|password|signed[_-]?url|temporary[_-]?url)/i;
const temporaryURL = /https?:\/\/[^\s)\]}]+(?:signature|sig|token|expires)=[^\s)\]}]+/gi;
const credential = /\b(?:sk|pk)-[A-Za-z0-9_-]{12,}\b/g;
const authorization = /\b(?:authorization\s*:\s*|bearer\s+)[^\s,;]+/gi;

export type StoredAgentToolCall = {
  name: string;
  status: "queued" | "running" | "confirmation_required" | "succeeded" | "failed";
  summary?: string;
  truncated?: boolean;
  evidenceRefs?: Array<{ type: string; id: string; version: string; href?: string }>;
};

export function sanitizeAgentMessageForStorage(input: { content: string; toolCalls?: unknown }) {
  return {
    content: redactText(input.content).slice(0, 20_000),
    toolCalls: sanitizeToolCalls(input.toolCalls)
  };
}

function redactText(value: string) {
  return value.replace(temporaryURL, "[temporary-url-redacted]").replace(credential, "[credential-redacted]").replace(authorization, "[authorization-redacted]");
}

function sanitizeToolCalls(value: unknown): StoredAgentToolCall[] {
  if (!Array.isArray(value)) return [];
  return value.slice(0, 50).flatMap((raw) => {
    if (!raw || typeof raw !== "object") return [];
    const item = raw as Record<string, unknown>;
    if (typeof item.name !== "string" || typeof item.status !== "string") return [];
    const allowedStatuses = new Set(["queued", "running", "confirmation_required", "succeeded", "failed"]);
    if (!allowedStatuses.has(item.status)) return [];
    const evidenceRefs = Array.isArray(item.evidenceRefs) ? item.evidenceRefs.slice(0, 100).flatMap((rawReference) => {
      if (!rawReference || typeof rawReference !== "object") return [];
      const reference = rawReference as Record<string, unknown>;
      if (typeof reference.type !== "string" || typeof reference.id !== "string" || typeof reference.version !== "string") return [];
      const href = typeof reference.href === "string" && !temporaryURL.test(reference.href) ? reference.href : undefined;
      temporaryURL.lastIndex = 0;
      return [{ type: reference.type, id: reference.id, version: reference.version, ...(href ? { href } : {}) }];
    }) : undefined;
    const summary = typeof item.summary === "string" ? redactText(item.summary).slice(0, 2_000) : undefined;
    return [{
      name: item.name.slice(0, 100),
      status: item.status as StoredAgentToolCall["status"],
      ...(summary ? { summary } : {}),
      ...(item.truncated === true ? { truncated: true } : {}),
      ...(evidenceRefs?.length ? { evidenceRefs } : {})
    }];
  });
}

export function assertAgentSessionScope(input: { projectId: number; userId: number }, session: { projectId: number; startedByUserId: number }) {
  if (session.projectId !== input.projectId || session.startedByUserId !== input.userId) throw new Error("AGENT_SESSION_NOT_FOUND");
}
