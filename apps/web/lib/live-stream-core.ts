import { createHmac, timingSafeEqual } from "node:crypto";

export type LiveStreamStatus = "starting" | "live" | "degraded" | "failed" | "stopping" | "stopped";

export type LiveStreamSession = {
  id: number;
  projectId: number;
  deviceId: number;
  streamKey: string;
  sourceType: string;
  status: LiveStreamStatus;
  playbackRef: string | null;
  lastActiveAt: string | null;
  statusReason: string | null;
};

const transitions: Record<LiveStreamStatus, ReadonlySet<LiveStreamStatus>> = {
  starting: new Set(["live", "degraded", "failed", "stopping"]),
  live: new Set(["degraded", "failed", "stopping"]),
  degraded: new Set(["live", "failed", "stopping"]),
  failed: new Set(["starting", "stopped"]),
  stopping: new Set(["stopped", "failed"]),
  stopped: new Set()
};

export function transitionLiveStream(current: LiveStreamStatus, next: LiveStreamStatus) {
  if (!transitions[current].has(next)) throw new Error("INVALID_LIVE_STREAM_TRANSITION");
  return next;
}

export function normalizeAdapterLiveStatus(value: string): LiveStreamStatus {
  switch (value) {
    case "started": return "live";
    case "interrupted": return "degraded";
    case "failed": return "failed";
    case "stopping": return "stopping";
    case "stopped": return "stopped";
    default: throw new Error("UNKNOWN_ADAPTER_LIVE_STATUS");
  }
}

export function assertStreamCanStart(input: {
  deviceStatus: string;
  capabilities: readonly string[];
  adapterType: string | null;
}) {
  if (input.deviceStatus !== "online") throw new Error("LIVE_STREAM_DEVICE_OFFLINE");
  if (!input.capabilities.includes("camera.live")) throw new Error("LIVE_STREAM_NOT_SUPPORTED");
  if (input.adapterType !== "simulator" && input.adapterType !== "dji") {
    throw new Error("LIVE_STREAM_ADAPTER_UNAVAILABLE");
  }
}

export class SimulatorPlaybackLocator {
  readonly #signingSecret: string;
  readonly #now: () => Date;

  constructor(signingSecret: string, now: () => Date = () => new Date()) {
    if (signingSecret.length < 16) throw new Error("PLAYBACK_SIGNING_SECRET_TOO_SHORT");
    this.#signingSecret = signingSecret;
    this.#now = now;
  }

  issue(input: { projectId: number; streamId: number; playbackRef: string; ttlSeconds?: number }) {
    const ttlSeconds = input.ttlSeconds ?? 60;
    if (!Number.isInteger(ttlSeconds) || ttlSeconds < 1 || ttlSeconds > 300) throw new Error("INVALID_PLAYBACK_TTL");
    const expiresAt = new Date(this.#now().getTime() + ttlSeconds * 1000);
    const expires = String(expiresAt.getTime());
    const signature = this.#signature(input.projectId, input.streamId, input.playbackRef, expires);
    const path = `/api/projects/${input.projectId}/live-streams/${input.streamId}/simulator-playback`;
    return {
      url: `${path}?expires=${expires}&signature=${signature}`,
      expiresAt: expiresAt.toISOString()
    };
  }

  verify(input: { projectId: number; streamId: number; playbackRef: string; expires: string; signature: string }) {
    if (!/^\d+$/.test(input.expires) || Number(input.expires) <= this.#now().getTime()) return false;
    const expected = Buffer.from(this.#signature(input.projectId, input.streamId, input.playbackRef, input.expires));
    const actual = Buffer.from(input.signature);
    return actual.length === expected.length && timingSafeEqual(actual, expected);
  }

  #signature(projectId: number, streamId: number, playbackRef: string, expires: string) {
    return createHmac("sha256", this.#signingSecret)
      .update(`${projectId}\n${streamId}\n${playbackRef}\n${expires}`)
      .digest("hex");
  }
}

export function playbackAvailability(session: LiveStreamSession, now: Date, locatorExpiresAt: Date | null) {
  if (session.status !== "live" && session.status !== "degraded") return { available: false, reason: `stream-${session.status}` };
  if (!session.playbackRef) return { available: false, reason: "playback-unavailable" };
  if (locatorExpiresAt && locatorExpiresAt.getTime() <= now.getTime()) return { available: false, reason: "locator-expired" };
  return { available: true, reason: null };
}

export function assertLiveStreamProjectScope(session: LiveStreamSession, projectId: number) {
  if (session.projectId !== projectId) throw new Error("LIVE_STREAM_NOT_FOUND");
  return session;
}
