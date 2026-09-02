import { createHmac, randomBytes, timingSafeEqual } from "node:crypto";

export type LiveStreamStatus = "requested" | "starting" | "live" | "degraded" | "failed" | "stopping" | "stopped";

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
  requested: new Set(["starting", "failed", "stopping"]),
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
  connectorLiveControl?: boolean;
}) {
  if (input.deviceStatus !== "online") throw new Error("LIVE_STREAM_DEVICE_OFFLINE");
  if (!input.capabilities.includes("stream.video.control") && !input.capabilities.includes("camera.live")
      && !input.connectorLiveControl) {
    throw new Error("LIVE_STREAM_NOT_SUPPORTED");
  }
  if (input.adapterType !== "simulator" && input.adapterType !== "dji" && input.adapterType !== "dji-flighthub2") {
    throw new Error("LIVE_STREAM_ADAPTER_UNAVAILABLE");
  }
}

export function assertLiveStreamConcurrency(input: { activeCount: number; maxConcurrentSessions: number }) {
  if (!Number.isInteger(input.maxConcurrentSessions) || input.maxConcurrentSessions < 1) {
    throw new Error("LIVE_STREAM_CONCURRENCY_INVALID");
  }
  if (input.activeCount >= input.maxConcurrentSessions) throw new Error("LIVE_STREAM_CONCURRENCY_CONFLICT");
  return true;
}

export function createLiveStreamIngestRef() {
  return `demo/aerosight/${randomBytes(24).toString("base64url")}`;
}

export function buildRTMPIngestURL(input: {
  baseURL: string;
  ingestRef: string;
  username: string;
  password: string;
}) {
  const url = new URL(input.baseURL);
  if (url.protocol !== "rtmp:" && url.protocol !== "rtmps:") throw new Error("LIVE_STREAM_RTMP_URL_REQUIRED");
  if (url.username || url.password || !input.username || !input.password) throw new Error("LIVE_STREAM_PUBLISH_CREDENTIALS_INVALID");
  url.pathname = `${url.pathname.replace(/\/$/, "")}/${input.ingestRef}`;
  url.searchParams.set("user", input.username);
  url.searchParams.set("pass", input.password);
  return url.toString();
}

export function buildRTMPIngestEndpoint(baseURL: string, ingestRef: string) {
  const url = new URL(baseURL);
  if (url.protocol !== "rtmp:" && url.protocol !== "rtmps:") throw new Error("LIVE_STREAM_RTMP_URL_REQUIRED");
  if (url.username || url.password) throw new Error("LIVE_STREAM_PUBLISH_CREDENTIALS_INVALID");
  url.pathname = `${url.pathname.replace(/\/$/, "")}/${ingestRef}`;
  return url.toString();
}

export function buildDJIVideoID(input: {
  sourceExternalId: string;
  cameraType: number;
  cameraSubtype: number;
  videoIndex?: string;
}) {
  if (!input.sourceExternalId.trim() || !Number.isInteger(input.cameraType) || input.cameraType <= 0
      || !Number.isInteger(input.cameraSubtype) || input.cameraSubtype < 0) {
    throw new Error("DJI_LIVE_VIDEO_IDENTITY_INVALID");
  }
  const videoIndex = input.videoIndex?.trim() || "normal-0";
  if (!/^[a-z0-9-]{1,32}$/i.test(videoIndex)) throw new Error("DJI_LIVE_VIDEO_INDEX_INVALID");
  return `${input.sourceExternalId}/${input.cameraType}-${input.cameraSubtype}-0/${videoIndex}`;
}

export function planLiveStreamStop(status: LiveStreamStatus, sourceType: string) {
  if (status === "stopped" || status === "stopping") return { status, replayed: true };
  if (sourceType === "dji_flighthub" && status === "failed") return { status, replayed: true };
  if (["dji", "dji_flighthub"].includes(sourceType) && ["requested", "starting", "live", "degraded"].includes(status)) {
    return { status: "stopping" as const, replayed: false };
  }
  return { status: "stopped" as const, replayed: false };
}

export function recoverExpiredLiveStream(status: LiveStreamStatus, sourceType = "legacy") {
  if (sourceType === "dji_flighthub" && ["requested", "starting", "live", "degraded", "stopping"].includes(status)) {
    return { status, reason: "flighthub-remote-evidence-required" };
  }
  switch (status) {
    case "requested":
    case "starting":
      return { status: "failed" as const, reason: "session-start-lease-expired" };
    case "stopping":
      return { status: "stopped" as const, reason: "session-stop-lease-expired" };
    case "live":
    case "degraded":
      return { status: "failed" as const, reason: "session-owner-lease-expired" };
    case "failed":
    case "stopped":
      return { status, reason: null };
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

export function playbackAvailability(session: LiveStreamSession, _now: Date, _locatorExpiresAt: Date | null) {
  if (session.status !== "live" && session.status !== "degraded") return { available: false, reason: `stream-${session.status}` };
  if (!session.playbackRef) return { available: false, reason: "playback-unavailable" };
  return { available: true, reason: null };
}

export function assertLiveStreamProjectScope(session: LiveStreamSession, projectId: number) {
  if (session.projectId !== projectId) throw new Error("LIVE_STREAM_NOT_FOUND");
  return session;
}

export type BrowserPlaybackProtocol = "webrtc" | "hls";

type MediaPlaybackClaims = {
  projectId: number;
  streamId: number;
  path: string;
  protocols: BrowserPlaybackProtocol[];
  exp: number;
};

export class MediaPlaybackTokenIssuer {
  readonly #signingSecret: string;
  readonly #now: () => Date;

  constructor(signingSecret: string, now: () => Date = () => new Date()) {
    if (signingSecret.length < 16) throw new Error("PLAYBACK_SIGNING_SECRET_TOO_SHORT");
    this.#signingSecret = signingSecret;
    this.#now = now;
  }

  issue(input: Omit<MediaPlaybackClaims, "exp"> & { ttlSeconds?: number }) {
    const ttlSeconds = input.ttlSeconds ?? 60;
    if (!Number.isInteger(ttlSeconds) || ttlSeconds < 1 || ttlSeconds > 300 || input.protocols.length === 0) {
      throw new Error("INVALID_PLAYBACK_TOKEN_INPUT");
    }
    const claims: MediaPlaybackClaims = { projectId: input.projectId, streamId: input.streamId,
      path: input.path, protocols: [...new Set(input.protocols)], exp: this.#now().getTime() + ttlSeconds * 1000 };
    const payload = Buffer.from(JSON.stringify(claims)).toString("base64url");
    const signature = createHmac("sha256", this.#signingSecret).update(payload).digest("base64url");
    return { token: `${payload}.${signature}`, expiresAt: new Date(claims.exp).toISOString() };
  }

  verify(token: string, input: { path: string; protocol: BrowserPlaybackProtocol }) {
    const [payload, signature, extra] = token.split(".");
    if (!payload || !signature || extra) return null;
    const expected = Buffer.from(createHmac("sha256", this.#signingSecret).update(payload).digest("base64url"));
    const actual = Buffer.from(signature);
    if (actual.length !== expected.length || !timingSafeEqual(actual, expected)) return null;
    try {
      const claims = JSON.parse(Buffer.from(payload, "base64url").toString("utf8")) as MediaPlaybackClaims;
      if (!Number.isSafeInteger(claims.projectId) || !Number.isSafeInteger(claims.streamId)
          || claims.exp <= this.#now().getTime() || claims.path !== input.path
          || !claims.protocols.includes(input.protocol)) return null;
      return claims;
    } catch {
      return null;
    }
  }
}

export function buildPlaybackCandidates(input: {
  path: string;
  token: string;
  hlsBaseURL: string | null;
  webrtcBaseURL: string | null;
}) {
  const result: { protocol: BrowserPlaybackProtocol; url: string }[] = [];
  if (input.webrtcBaseURL) {
    result.push({ protocol: "webrtc", url: `${input.webrtcBaseURL.replace(/\/$/, "")}/${input.path}?token=${encodeURIComponent(input.token)}` });
  }
  if (input.hlsBaseURL) {
    result.push({ protocol: "hls", url: `${input.hlsBaseURL.replace(/\/$/, "")}/${input.path}/index.m3u8?token=${encodeURIComponent(input.token)}` });
  }
  if (result.length === 0) throw new Error("PLAYBACK_PROTOCOL_UNAVAILABLE");
  return result;
}

export function nextPlaybackCandidate(
  candidates: { protocol: BrowserPlaybackProtocol; url: string }[],
  failedProtocols: BrowserPlaybackProtocol[]
) {
  return candidates.find((candidate) => !failedProtocols.includes(candidate.protocol)) ?? null;
}
