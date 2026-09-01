import { z } from "zod";

const common = {
  connectorInstanceId: z.number().int().positive(),
  taskRunId: z.number().int().positive(),
  approvalRequestId: z.string().uuid(),
  idempotencyKey: z.string().trim().min(8).max(200)
};

const repeatOptionSchema = z.object({
  interval: z.number().int().positive().optional(),
  daysOfWeek: z.array(z.number().int().min(0).max(6)).max(7).optional(),
  daysOfMonth: z.array(z.number().int().min(1).max(31)).max(31).optional(),
  weekOfMonth: z.number().int().min(1).max(4).optional()
}).strict();

export const flightHubActionInputSchema = z.discriminatedUnion("action", [
  z.object({
    ...common,
    action: z.literal("flight-task-create"),
    waylineResourceId: z.number().int().positive(),
    request: z.object({
      name: z.string().trim().min(1).max(200),
      timeZone: z.string().trim().min(1).max(100),
      taskType: z.enum(["immediate", "timed", "recurring", "continuous"]),
      rthAltitude: z.number().int().min(0).max(1000).optional(),
      rthMode: z.enum(["optimal", "preset"]).optional(),
      outOfControlActionInFlight: z.enum(["return_home", "continue_task"]).optional(),
      waylinePrecisionType: z.enum(["gps", "rtk"]).optional(),
      resumableStatus: z.enum(["auto", "manual"]).optional(),
      repeatType: z.enum(["nonrepeating", "daily", "weekly", "absolute_monthly", "relative_monthly"]).optional(),
      repeatOption: repeatOptionSchema.optional(),
      landingDeviceId: z.number().int().positive().optional(),
      beginAt: z.number().int().positive().optional(),
      endAt: z.number().int().positive().optional(),
      recurringTaskStartTimes: z.array(z.number().int().positive()).max(24).optional(),
      continuousTaskPeriods: z.array(z.tuple([z.number().int().positive(), z.number().int().positive()])).max(24).optional(),
      minimumBatteryCapacity: z.number().int().min(50).max(100).optional()
    }).strict()
  }).strict(),
  z.object({
    ...common,
    action: z.literal("flight-task-status"),
    targetResourceId: z.number().int().positive(),
    request: z.object({ desiredStatus: z.enum(["suspended", "restored"]) }).strict()
  }).strict(),
  z.object({
    ...common,
    action: z.literal("flight-task-resumption"),
    targetResourceId: z.number().int().positive(),
    request: z.object({}).strict()
  }).strict()
]);

export type FlightHubActionInput = z.infer<typeof flightHubActionInputSchema>;

export type FlightHubActionAuthorization = {
  hasPermission: boolean;
  teamId: number;
  connectorProjectId: number;
  connectorTeamId: number;
  connectorStatus: string;
  actionEnabled: boolean;
  capabilityFieldVerified: boolean;
  taskRunProjectId: number;
  taskRunTeamId: number;
  taskRunStatus: string;
  selectedDeviceId: number | null;
  safetyPolicyVersionId: number | null;
  preflightAllowed: boolean;
  deviceIdentityPresent: boolean;
  approvalProjectId: number | null;
  approvalTeamId: number | null;
  approvalStatus: string | null;
  approvalResourceType: string | null;
  approvalResourceId: string | null;
  approvalAction: string | null;
  approvalUnexpired: boolean;
  approvalPreflightAllowed: boolean;
  waylineProjectId: number | null;
  waylineConnectorId: number | null;
  waylineKind: string | null;
  targetProjectId: number | null;
  targetConnectorId: number | null;
  targetKind: string | null;
  targetTaskRunId: string | null;
};

export function approvalActionForFlightHubAction(action: FlightHubActionInput["action"]) {
  if (action === "flight-task-create") return "flighthub.flight-task.create";
  if (action === "flight-task-status") return "flighthub.flight-task.status";
  return "flighthub.flight-task.resume";
}

export function authorizeFlightHubAction(
  projectId: number,
  input: FlightHubActionInput,
  authorization: FlightHubActionAuthorization
) {
  if (!authorization.hasPermission) throw new Error("FLIGHTHUB_ACTION_PERMISSION_DENIED");
  if (authorization.connectorProjectId !== projectId || authorization.taskRunProjectId !== projectId
    || authorization.connectorTeamId !== authorization.teamId || authorization.taskRunTeamId !== authorization.teamId) {
    throw new Error("FLIGHTHUB_ACTION_SCOPE_MISMATCH");
  }
  if (!new Set(["connecting", "connected", "degraded"]).has(authorization.connectorStatus)) {
    throw new Error("FLIGHTHUB_ACTION_CONNECTOR_DISABLED");
  }
  if (!authorization.actionEnabled || !authorization.capabilityFieldVerified) {
    throw new Error("FLIGHTHUB_ACTION_DISABLED");
  }
  if (!authorization.selectedDeviceId || !authorization.deviceIdentityPresent) {
    throw new Error("FLIGHTHUB_ACTION_DEVICE_SCOPE_MISMATCH");
  }
  if (!authorization.safetyPolicyVersionId || !authorization.preflightAllowed || !authorization.approvalPreflightAllowed) {
    throw new Error("FLIGHTHUB_ACTION_PREFLIGHT_FAILED");
  }
  if (authorization.approvalStatus !== "approved" || !authorization.approvalUnexpired) {
    throw new Error("FLIGHTHUB_ACTION_APPROVAL_REQUIRED");
  }
  if (authorization.approvalProjectId !== projectId || authorization.approvalTeamId !== authorization.teamId
    || authorization.approvalResourceType !== "task_run"
    || authorization.approvalResourceId !== String(input.taskRunId)
    || authorization.approvalAction !== approvalActionForFlightHubAction(input.action)) {
    throw new Error("FLIGHTHUB_ACTION_APPROVAL_SCOPE_MISMATCH");
  }
  if (input.action === "flight-task-create") {
    if (!new Set(["ready", "dispatching"]).has(authorization.taskRunStatus)) {
      throw new Error("FLIGHTHUB_ACTION_TASK_STATE_INVALID");
    }
    if (authorization.waylineProjectId !== projectId || authorization.waylineConnectorId !== input.connectorInstanceId
      || authorization.waylineKind !== "wayline") throw new Error("FLIGHTHUB_ACTION_WAYLINE_SCOPE_MISMATCH");
  } else {
    if (authorization.targetProjectId !== projectId || authorization.targetConnectorId !== input.connectorInstanceId
      || authorization.targetKind !== "flight-task" || authorization.targetTaskRunId !== String(input.taskRunId)) {
      throw new Error("FLIGHTHUB_ACTION_REMOTE_TASK_SCOPE_MISMATCH");
    }
    const allowedStates = input.action === "flight-task-status" ? new Set(["running", "paused", "dispatching"]) : new Set(["paused", "blocked"]);
    if (!allowedStates.has(authorization.taskRunStatus)) throw new Error("FLIGHTHUB_ACTION_TASK_STATE_INVALID");
  }
  return Object.freeze({
    ...input,
    projectId,
    teamId: authorization.teamId,
    deviceId: authorization.selectedDeviceId,
    safetyPolicyVersionId: authorization.safetyPolicyVersionId,
    dispatchPath: "project-outbox-connector-action-job" as const,
    completion: "await-remote-reconciliation" as const
  });
}
