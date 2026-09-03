export type FlightHubPlaybackGateInput = {
  canView: boolean;
  status: string;
  startAcceptedAt: Date | null;
  localAuthorizationRevokedAt: Date | null;
  credentialExpiresAt: Date | null;
  now: Date;
};

export function flightHubPlaybackGate(input: FlightHubPlaybackGateInput) {
  if (!input.canView) throw new Error("FLIGHTHUB_PLAYBACK_PERMISSION_REVOKED");
  if (!new Set(["starting", "live", "degraded"]).has(input.status)) return { available: false, reason: `stream-${input.status}` } as const;
  if (input.status === "starting" && !input.startAcceptedAt) return { available: false, reason: "stream-starting-unaccepted" } as const;
  if (input.localAuthorizationRevokedAt) return { available: false, reason: "playback-authorization-revoked" } as const;
  if (!input.credentialExpiresAt || input.credentialExpiresAt.getTime() <= input.now.getTime()) {
    return { available: false, reason: "playback-credential-expired" } as const;
  }
  return { available: true } as const;
}

export function safeLiveMediaSummary(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  const blocked = /url|token|credential|password|secret|username|server(ip|port)|device(password|id)|local(port|channel)|rtsp/i;
  return Object.fromEntries(Object.entries(value as Record<string, unknown>)
    .filter(([key, entry]) => !blocked.test(key) && ["string", "number", "boolean"].includes(typeof entry))
    .slice(0, 16));
}
