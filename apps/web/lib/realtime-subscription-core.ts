import { actionPatternMatches } from "./device-command-core.ts";

export type RealtimeSubscriptionTarget = {
  requestProjectId: number;
  channelProjectId: number;
  deviceId: number;
  deviceTypeId: string;
  capabilityCode: string;
  availability: "available" | "degraded" | "unavailable";
};

export type RealtimeSubscriptionGrant = {
  scopeType: "project" | "device_type" | "device";
  deviceTypeId: string | null;
  deviceId: number | null;
  actionPattern: string;
  effect: "allow" | "deny";
};

function grantApplies(grant: RealtimeSubscriptionGrant, target: RealtimeSubscriptionTarget) {
  if (!actionPatternMatches(grant.actionPattern, target.capabilityCode)) return false;
  if (grant.scopeType === "project") return true;
  if (grant.scopeType === "device_type") return grant.deviceTypeId === target.deviceTypeId;
  return grant.deviceId === target.deviceId;
}

export function authorizeRealtimeSubscription(input: {
  role: "owner" | "admin" | "member";
  target: RealtimeSubscriptionTarget;
  grants: RealtimeSubscriptionGrant[];
}) {
  if (input.target.requestProjectId !== input.target.channelProjectId) {
    throw new Error("REALTIME_SUBSCRIPTION_SCOPE_DENIED");
  }
  if (!input.target.capabilityCode.startsWith("stream.")) {
    throw new Error("REALTIME_SUBSCRIPTION_ACTION_INVALID");
  }
  if (input.target.availability === "unavailable") {
    throw new Error("REALTIME_CHANNEL_UNAVAILABLE");
  }
  const applicable = input.grants.filter((grant) => grantApplies(grant, input.target));
  if (applicable.some((grant) => grant.effect === "deny")) {
    throw new Error("REALTIME_SUBSCRIPTION_EXPLICITLY_DENIED");
  }
  if (input.role === "owner" || input.role === "admin" || applicable.some((grant) => grant.effect === "allow")) {
    return true;
  }
  throw new Error("REALTIME_SUBSCRIPTION_NOT_GRANTED");
}

export function parseRealtimeCursor(value: string | null) {
  if (!value || !/^\d+$/.test(value)) return "0";
  return BigInt(value).toString();
}

export type RealtimeSample = {
  cursor: string;
  eventId: string;
  capturedAt: Date | string;
  payload: Record<string, unknown>;
  quality: Record<string, unknown>;
};

export function encodeRealtimeSample(channelId: string, sample: RealtimeSample) {
  return `id: ${sample.cursor}\nevent: channel.sample\ndata: ${JSON.stringify({
    channelId,
    eventId: sample.eventId,
    capturedAt: sample.capturedAt instanceof Date ? sample.capturedAt.toISOString() : sample.capturedAt,
    payload: sample.payload,
    quality: sample.quality
  })}\n\n`;
}

export function encodeRealtimeAccessRevoked(channelId: string) {
  return `event: access.revoked\ndata: ${JSON.stringify({ channelId, reason: "capability_revoked" })}\n\n`;
}

export function realtimeFlowDecision(desiredSize: number | null, pendingSamples: number, hardLimit = 500) {
  if (pendingSamples > hardLimit) return "terminate" as const;
  if (desiredSize !== null && desiredSize <= 0) return "pause" as const;
  return "deliver" as const;
}
