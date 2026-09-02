export const flightHubDiscretePolicies = {
  return_home: { capabilityCode: "flight.return_home", connectorCapabilityCode: "device.control", featureFlag: "device.control", approvalAction: "flighthub.device.return_home", deviceTypes: null },
  return_home_cancel: { capabilityCode: "flight.return_home", connectorCapabilityCode: "device.control", featureFlag: "device.control", approvalAction: "flighthub.device.return_home_cancel", deviceTypes: null },
  flighttask_pause: { capabilityCode: "mission.execute", connectorCapabilityCode: "device.control", featureFlag: "device.control", approvalAction: "flighthub.device.flighttask_pause", deviceTypes: null },
  flighttask_recovery: { capabilityCode: "mission.execute", connectorCapabilityCode: "device.control", featureFlag: "device.control", approvalAction: "flighthub.device.flighttask_recovery", deviceTypes: null },
  "camera.change": { capabilityCode: "camera.change", connectorCapabilityCode: "device.camera.change", featureFlag: "flighthub.camera.change", approvalAction: "flighthub.device.camera.change", deviceTypes: ["dji.dock2", "dji.dock3"] },
  "camera.change_lens": { capabilityCode: "camera.lens.change", connectorCapabilityCode: "device.lens.change", featureFlag: "flighthub.lens.change", approvalAction: "flighthub.device.camera.change_lens", deviceTypes: ["dji.matrice3d", "dji.matrice3td", "dji.matrice4d", "dji.matrice4td"] }
} as const;

export type FlightHubDiscreteCommandKey = keyof typeof flightHubDiscretePolicies;

const identifier = /^[A-Za-z0-9._:-]{1,256}$/;

export function flightHubDiscretePolicy(commandKey: string) {
  return flightHubDiscretePolicies[commandKey as FlightHubDiscreteCommandKey];
}

export function validateFlightHubCommandParameters(commandKey: string, parameters: Record<string, unknown>) {
  const keys = Object.keys(parameters).sort();
  if (commandKey === "camera.change") {
    return !keys.some((key) => key !== "cameraIndex" && key !== "cameraPosition")
      && identifier.test(String(parameters.cameraIndex ?? ""))
      && (parameters.cameraPosition === undefined || identifier.test(String(parameters.cameraPosition)));
  }
  if (commandKey === "camera.change_lens") {
    return keys.length === 2 && keys[0] === "cameraIndex" && keys[1] === "lensType"
      && identifier.test(String(parameters.cameraIndex ?? "")) && identifier.test(String(parameters.lensType ?? ""));
  }
  return keys.length === 0;
}

export function authorizeFlightHubDiscreteCommand(input: {
  projectId: number;
  teamId: number;
  deviceId: number;
  capabilityCode: string;
  commandKey: string;
  parametersValid: boolean;
  deviceTypeKey: string;
  deviceOnline: boolean;
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
  const policy = flightHubDiscretePolicy(input.commandKey);
  if (!policy || policy.capabilityCode !== input.capabilityCode) throw new Error("FLIGHTHUB_COMMAND_POLICY_MISMATCH");
  if (!input.parametersValid) throw new Error("FLIGHTHUB_COMMAND_PARAMETERS_INVALID");
  if (policy.deviceTypes && !(policy.deviceTypes as readonly string[]).includes(input.deviceTypeKey)) throw new Error("FLIGHTHUB_COMMAND_MODEL_UNSUPPORTED");
  if (!input.deviceOnline) throw new Error("FLIGHTHUB_COMMAND_DEVICE_OFFLINE");
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
