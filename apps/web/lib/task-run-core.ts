export type TaskRunStatus = "queued" | "blocked" | "ready" | "dispatching" | "running" | "paused"
  | "succeeded" | "failed" | "canceling" | "canceled";

const transitions: Record<TaskRunStatus, TaskRunStatus[]> = {
  queued: ["blocked", "ready", "canceled"],
  blocked: ["queued", "ready", "canceling", "canceled"],
  ready: ["blocked", "dispatching", "canceling", "canceled"],
  dispatching: ["running", "paused", "failed", "canceling"],
  running: ["paused", "succeeded", "failed", "canceling"],
  paused: ["running", "failed", "canceling"],
  canceling: ["canceled", "failed"],
  succeeded: [], failed: [], canceled: []
};

export type TaskRunState = { status: TaskRunStatus; stateVersion: number; reason?: string };

export function transitionTaskRun(
  current: TaskRunState,
  expectedVersion: number,
  nextStatus: TaskRunStatus,
  reason: string
): TaskRunState {
  if (current.stateVersion !== expectedVersion) throw new Error("TASK_RUN_VERSION_CONFLICT");
  if (!transitions[current.status].includes(nextStatus)) throw new Error(`TASK_RUN_TRANSITION_INVALID:${current.status}:${nextStatus}`);
  if (!reason.trim()) throw new Error("TASK_RUN_TRANSITION_REASON_REQUIRED");
  return { status: nextStatus, stateVersion: current.stateVersion + 1, reason };
}

export type CommandStatus = "pending" | "dispatchable" | "sent" | "acknowledged" | "nacked"
  | "timed_out" | "canceled" | "unknown";
export type CommandLedgerEntry = { commandId: string; status: CommandStatus; result?: Record<string, unknown> };
export type CommandAck = { commandId: string; outcome: "ack" | "nack" | "timeout"; result?: Record<string, unknown> };

export function applyCommandAck(entries: CommandLedgerEntry[], ack: CommandAck) {
  const index = entries.findIndex((entry) => entry.commandId === ack.commandId);
  if (index < 0) return { matched: false, duplicate: false, diagnostic: "UNKNOWN_COMMAND_ACK", entries } as const;
  const existing = entries[index];
  if (["acknowledged", "nacked", "timed_out", "canceled"].includes(existing.status)) {
    return { matched: true, duplicate: true, diagnostic: "COMMAND_ACK_TERMINAL", entries } as const;
  }
  const status: CommandStatus = ack.outcome === "ack" ? "acknowledged" : ack.outcome === "nack" ? "nacked" : "timed_out";
  const updated = [...entries];
  updated[index] = { ...existing, status, result: ack.result ?? {} };
  return { matched: true, duplicate: false, diagnostic: null, entries: updated } as const;
}
