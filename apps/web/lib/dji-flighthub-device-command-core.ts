export const flightHubDiscretePolicies = {
  return_home: { capabilityCode: "flight.return_home", approvalAction: "flighthub.device.return_home" },
  return_home_cancel: { capabilityCode: "flight.return_home", approvalAction: "flighthub.device.return_home_cancel" },
  flighttask_pause: { capabilityCode: "mission.execute", approvalAction: "flighthub.device.flighttask_pause" },
  flighttask_recovery: { capabilityCode: "mission.execute", approvalAction: "flighthub.device.flighttask_recovery" }
} as const;

export type FlightHubDiscreteCommandKey = keyof typeof flightHubDiscretePolicies;

export function authorizeFlightHubDiscreteCommand(input: {
  projectId: number;
  teamId: number;
  deviceId: number;
  capabilityCode: string;
  commandKey: string;
  parametersEmpty: boolean;
  connectorStatus: string;
  featureEnabled: boolean;
  capabilityFieldVerified: boolean;
  stateCapturedAt: Date | null;
  now: Date;
  safetyPolicyVersionId: number | null;
  currentSafetyPolicyVersionId: number | null;
  approvalProjectId: number | null;
  approvalTeamId: number | null;
  approvalResourceType: string | null;
  approvalResourceId: string | null;
  approvalAction: string | null;
  approvalStatus: string | null;
  approvalUnexpired: boolean;
}) {
  const policy = flightHubDiscretePolicies[input.commandKey as FlightHubDiscreteCommandKey];
  if (!policy || policy.capabilityCode !== input.capabilityCode) throw new Error("FLIGHTHUB_COMMAND_POLICY_MISMATCH");
  if (!input.parametersEmpty) throw new Error("FLIGHTHUB_COMMAND_PARAMETERS_INVALID");
  if (input.connectorStatus !== "connected") throw new Error("FLIGHTHUB_CONNECTOR_NOT_CONNECTED");
  if (!input.featureEnabled) throw new Error("FLIGHTHUB_COMMAND_FEATURE_DISABLED");
  if (!input.capabilityFieldVerified) throw new Error("FLIGHTHUB_COMMAND_NOT_FIELD_VERIFIED");
  if (!input.stateCapturedAt || input.now.getTime() - input.stateCapturedAt.getTime() > 30_000
      || input.stateCapturedAt.getTime() > input.now.getTime() + 1_000) {
    throw new Error("FLIGHTHUB_COMMAND_STATE_STALE");
  }
  if (!input.safetyPolicyVersionId || input.safetyPolicyVersionId !== input.currentSafetyPolicyVersionId) {
    throw new Error("FLIGHTHUB_COMMAND_SAFETY_POLICY_STALE");
  }
  if (input.approvalProjectId !== input.projectId || input.approvalTeamId !== input.teamId
      || input.approvalResourceType !== "device" || input.approvalResourceId !== String(input.deviceId)
      || input.approvalAction !== policy.approvalAction || input.approvalStatus !== "approved"
      || !input.approvalUnexpired) {
    throw new Error("FLIGHTHUB_COMMAND_APPROVAL_REQUIRED");
  }
  return { policy, stateFresh: true as const, safetyPolicyCurrent: true as const, approvalValid: true as const };
}
