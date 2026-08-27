import { z } from "zod";
import { assertAgentToolArgsDoNotContainScope, type AgentExecutionContext } from "./agent-execution-context-core.ts";

export const agentMissionStartInputSchema = z.object({
  taskVersionId: z.number().int().positive(),
  approvalRequestId: z.string().uuid(),
  idempotencyKey: z.string().uuid()
}).strict();

export type AgentMissionStartAuthorization = {
  hasPermission: boolean;
  taskProjectId: number;
  taskVersionStatus: string;
  approvalStatus: string | null;
  approvalProjectId: number | null;
  approvalResourceType: string | null;
  approvalResourceId: string | null;
  approvalAction: string | null;
  preflightAllowed: boolean;
  deviceCommandsEnabled: boolean;
  selectedDeviceId: number | null;
  safetyPolicyVersionId: number | null;
};

export function authorizeAgentMissionStart(
  context: AgentExecutionContext,
  rawInput: unknown,
  authorization: AgentMissionStartAuthorization
) {
  assertAgentToolArgsDoNotContainScope(rawInput);
  const input = agentMissionStartInputSchema.parse(rawInput);
  if (!authorization.hasPermission) throw new Error("AGENT_MISSION_PERMISSION_DENIED");
  if (authorization.taskProjectId !== context.projectId) throw new Error("AGENT_MISSION_SCOPE_MISMATCH");
  if (authorization.taskVersionStatus !== "published") throw new Error("AGENT_MISSION_VERSION_NOT_PUBLISHED");
  if (authorization.approvalStatus !== "approved") throw new Error("AGENT_MISSION_APPROVAL_REQUIRED");
  if (authorization.approvalProjectId !== context.projectId
    || authorization.approvalResourceType !== "task_version"
    || authorization.approvalResourceId !== String(input.taskVersionId)
    || authorization.approvalAction !== "mission.start") throw new Error("AGENT_MISSION_APPROVAL_SCOPE_MISMATCH");
  if (!authorization.preflightAllowed || !authorization.safetyPolicyVersionId) throw new Error("AGENT_MISSION_PREFLIGHT_FAILED");
  if (!authorization.deviceCommandsEnabled) throw new Error("AGENT_MISSION_COMMANDS_DISABLED");
  if (!authorization.selectedDeviceId) throw new Error("AGENT_MISSION_DEVICE_NOT_SELECTED");
  return Object.freeze({
    ...input,
    projectId: context.projectId,
    teamId: context.teamId,
    userId: context.userId,
    sessionId: context.sessionId,
    selectedDeviceId: authorization.selectedDeviceId,
    safetyPolicyVersionId: authorization.safetyPolicyVersionId,
    dispatchPath: "project-outbox-command-ledger" as const,
    directAdapterAccess: false as const,
    completion: "await-device-ack" as const
  });
}
