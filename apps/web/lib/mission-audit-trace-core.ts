export type MissionAuditInput = {
  projectId: number;
  taskRunId: number;
  triggerSource: string;
  request?: {
    requestId: string;
    action: string;
    actorType: "user" | "agent";
    actorId: number;
    createdAt: string;
  } | null;
  preflight: { policyVersionId?: string | number | null; allowed: boolean; checks?: unknown[] };
  approval?: { id: string; status: string; requiredApprovals: number; receivedApprovals: number } | null;
  commands: Array<{
    id: string;
    action: string;
    capabilityCode: string;
    status: string;
    priority: number;
    attempt?: number | null;
    attemptStatus?: string | null;
    errorCode?: string | null;
  }>;
};

export type MissionSafetyState = "not_requested" | "confirmed" | "rejected" | "unknown";

export function buildMissionAuditTrace(input: MissionAuditInput) {
  if (input.projectId <= 0 || input.taskRunId <= 0) throw new Error("AUDIT_TRACE_SCOPE_INVALID");
  const emergency = [...input.commands].reverse().find((command) => command.priority >= 90
    || command.action === "safety.emergency_stop" || command.action === "flight.return_home");
  const safetyState: MissionSafetyState = !emergency ? "not_requested"
    : emergency.status === "acknowledged" || emergency.attemptStatus === "acknowledged" ? "confirmed"
      : emergency.status === "nacked" || emergency.attemptStatus === "nacked" ? "rejected" : "unknown";
  const missing: string[] = [];
  if (!input.request) missing.push("request");
  if (!input.preflight.policyVersionId) missing.push("preflight_policy_version");
  if (!input.preflight.allowed) missing.push("preflight_pass");
  if (input.approval && input.approval.status !== "approved") missing.push("approval");
  if (!input.commands.length) missing.push("command");
  if (input.commands.some((command) => !command.attemptStatus)) missing.push("command_attempt");

  return {
    projectId: input.projectId,
    taskRunId: input.taskRunId,
    triggerSource: input.triggerSource,
    correlation: {
      requestId: input.request?.requestId ?? null,
      approvalRequestId: input.approval?.id ?? null,
      commandIds: input.commands.map((command) => command.id)
    },
    stages: {
      request: input.request ?? null,
      preflight: input.preflight,
      approval: input.approval ?? null,
      commands: input.commands
    },
    safetyState,
    complete: missing.length === 0,
    missing
  };
}

export function planEmergencyStopDrill(input: {
  projectId: number;
  taskRunId: number;
  requestId: string;
  actorUserId: number;
  deviceConnected: boolean;
  capabilityDeclared: boolean;
  outcome: "ack" | "nack" | "timeout" | "disconnected";
}) {
  const dispatchable = input.deviceConnected && input.capabilityDeclared && input.outcome !== "disconnected";
  const commandStatus = !dispatchable || input.outcome === "timeout" ? "unknown"
    : input.outcome === "ack" ? "acknowledged" : "nacked";
  return buildMissionAuditTrace({
    projectId: input.projectId,
    taskRunId: input.taskRunId,
    triggerSource: "emergency_stop_drill",
    request: {
      requestId: input.requestId,
      action: "safety.emergency_stop_drill",
      actorType: "user",
      actorId: input.actorUserId,
      createdAt: new Date().toISOString()
    },
    preflight: { policyVersionId: "drill-policy", allowed: true, checks: [{ code: "DRY_RUN_ONLY", severity: "pass" }] },
    approval: null,
    commands: [{
      id: `drill:${input.taskRunId}:${input.requestId}`,
      action: "safety.emergency_stop",
      capabilityCode: "safety.emergency_stop",
      status: commandStatus,
      priority: 100,
      attempt: dispatchable ? 1 : null,
      attemptStatus: dispatchable ? commandStatus : "transport_unavailable",
      errorCode: commandStatus === "unknown" ? (input.deviceConnected ? "ACK_TIMEOUT" : "DEVICE_DISCONNECTED") : null
    }]
  });
}
