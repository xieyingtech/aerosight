export type DeviceCommandSafetyInput = {
  requestProjectId: number;
  deviceProjectId: number;
  deviceId: number;
  capabilityCode: string;
  riskLevel: "low" | "medium" | "high" | "critical";
  capabilityAvailability: "available" | "degraded" | "unavailable";
  deviceStatus: "online" | "degraded" | "offline" | "unknown" | "unavailable";
  activeTaskCount: number;
  confirmation: string | null;
};

export function confirmationPhrase(deviceId: number, capabilityCode: string) {
  return `CONFIRM ${deviceId} ${capabilityCode}`;
}

export function assertDeviceCommandSafety(input: DeviceCommandSafetyInput) {
  if (input.requestProjectId !== input.deviceProjectId) throw new Error("DEVICE_COMMAND_SCOPE_DENIED");
  if (input.capabilityAvailability !== "available") throw new Error("DEVICE_CAPABILITY_UNAVAILABLE");
  if (input.deviceStatus !== "online") throw new Error("DEVICE_COMMAND_DEVICE_NOT_ONLINE");
  const permitsActiveTask = input.capabilityCode === "flight.return_home";
  if (input.activeTaskCount > 0 && !permitsActiveTask) throw new Error("DEVICE_COMMAND_ACTIVE_TASK_CONFLICT");
  if (["high", "critical"].includes(input.riskLevel)
      && input.confirmation !== confirmationPhrase(input.deviceId, input.capabilityCode)) {
    throw new Error("DEVICE_COMMAND_CONFIRMATION_REQUIRED");
  }
  return {
    allowed: true as const,
    confirmationRequired: ["high", "critical"].includes(input.riskLevel),
    activeTaskOverride: permitsActiveTask && input.activeTaskCount > 0
  };
}

export function actionPatternMatches(pattern: string, action: string) {
  if (pattern === "*" || pattern === action) return true;
  return pattern.endsWith(".*") && action.startsWith(pattern.slice(0, -1));
}

export function authorizeCapabilityAction(input: {
  role: "owner" | "admin" | "member";
  action: string;
  grants: { actionPattern: string; effect: "allow" | "deny" }[];
}) {
  const matching = input.grants.filter((grant) => actionPatternMatches(grant.actionPattern, input.action));
  if (matching.some((grant) => grant.effect === "deny")) throw new Error("DEVICE_CAPABILITY_EXPLICITLY_DENIED");
  if (input.role === "owner" || input.role === "admin" || matching.some((grant) => grant.effect === "allow")) return true;
  throw new Error("DEVICE_CAPABILITY_NOT_GRANTED");
}
