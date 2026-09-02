export const CONTROL_LEASE_MILLISECONDS = 15_000;
export const CONTROL_SESSION_MILLISECONDS = 5 * 60_000;
export const CONTROL_HEARTBEAT_MIN_MILLISECONDS = 500;
export const CONTROL_OPERATIONS_PER_SECOND = 10;

export type FlightHubControlSelection = { flight: boolean; payloadIndex: string[] };

export function normalizeControlSelection(value: unknown): FlightHubControlSelection {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("FLIGHTHUB_CONTROL_SELECTION_INVALID");
  const input = value as Record<string, unknown>;
  if (Object.keys(input).some((key) => key !== "flight" && key !== "payloadIndex")) {
    throw new Error("FLIGHTHUB_CONTROL_SELECTION_INVALID");
  }
  const flight = input.flight === true;
  const payloadIndex = input.payloadIndex === undefined ? [] : input.payloadIndex;
  if (!Array.isArray(payloadIndex) || payloadIndex.length > 32
      || payloadIndex.some((item) => typeof item !== "string" || !/^[A-Za-z0-9._:-]{1,256}$/.test(item))) {
    throw new Error("FLIGHTHUB_CONTROL_SELECTION_INVALID");
  }
  const unique = [...new Set(payloadIndex as string[])].sort();
  if (unique.length !== payloadIndex.length || (!flight && unique.length === 0)) {
    throw new Error("FLIGHTHUB_CONTROL_SELECTION_INVALID");
  }
  return { flight, payloadIndex: unique };
}

export function authorizeControlSession(input: {
  projectId: number; teamId: number; deviceId: number;
  connectorProjectId: number; connectorTeamId: number; deviceProjectId: number;
  connectorStatus: string; featureEnabled: boolean; capabilityFieldVerified: boolean;
  deviceOnline: boolean; stateCapturedAt: Date | null; now: Date;
  requestedSafetyPolicyVersionId: number; currentSafetyPolicyVersionId: number | null;
  approvalProjectId: number | null; approvalTeamId: number | null; approvalResourceType: string | null;
  approvalResourceId: string | null; approvalAction: string | null; approvalStatus: string | null; approvalUnexpired: boolean;
  conflictingSessionCount: number;
}) {
  if (input.connectorProjectId !== input.projectId || input.connectorTeamId !== input.teamId
      || input.deviceProjectId !== input.projectId) throw new Error("FLIGHTHUB_CONTROL_SCOPE_MISMATCH");
  if (input.connectorStatus !== "connected") throw new Error("FLIGHTHUB_CONTROL_CONNECTOR_UNAVAILABLE");
  if (!input.featureEnabled || !input.capabilityFieldVerified) throw new Error("FLIGHTHUB_CONTROL_NOT_ENABLED");
  if (!input.deviceOnline || !input.stateCapturedAt
      || input.now.getTime() - input.stateCapturedAt.getTime() > 30_000
      || input.stateCapturedAt.getTime() > input.now.getTime() + 1_000) throw new Error("FLIGHTHUB_CONTROL_DEVICE_STALE");
  if (input.requestedSafetyPolicyVersionId !== input.currentSafetyPolicyVersionId) {
    throw new Error("FLIGHTHUB_CONTROL_SAFETY_POLICY_STALE");
  }
  if (input.approvalProjectId !== input.projectId || input.approvalTeamId !== input.teamId
      || input.approvalResourceType !== "device" || input.approvalResourceId !== String(input.deviceId)
      || input.approvalAction !== "flighthub.control.acquire" || input.approvalStatus !== "approved"
      || !input.approvalUnexpired) throw new Error("FLIGHTHUB_CONTROL_APPROVAL_REQUIRED");
  if (input.conflictingSessionCount !== 0) throw new Error("FLIGHTHUB_CONTROL_SESSION_CONFLICT");
  return true;
}

export function controlSessionExpiryReason(input: {
  status: string; now: Date; leaseExpiresAt: Date; absoluteExpiresAt: Date; permissionCurrent: boolean;
}) {
  if (input.status !== "active") return null;
  if (!input.permissionCurrent) return "permission_revoked";
  if (input.now >= input.absoluteExpiresAt) return "maximum_duration_reached";
  if (input.now >= input.leaseExpiresAt) return "heartbeat_expired";
  return null;
}

export function nextControlLease(now: Date, absoluteExpiresAt: Date) {
  return new Date(Math.min(now.getTime() + CONTROL_LEASE_MILLISECONDS, absoluteExpiresAt.getTime()));
}
