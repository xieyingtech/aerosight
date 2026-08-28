import { createHmac, timingSafeEqual } from "node:crypto";

export type MediaAccessAction = "preview" | "play" | "download";

export function parseMediaAccessAction(value: string | null): MediaAccessAction {
  if (value === "preview" || value === "play" || value === "download") return value;
  throw new Error("INVALID_MEDIA_ACCESS_ACTION");
}

export function assertMediaActionAllowed(input: {
  action: MediaAccessAction;
  role: "owner" | "admin" | "member";
  permissions: ReadonlySet<string>;
  sensitive: boolean;
}) {
  if (input.action !== "download" || !input.sensitive) return;
  if (input.role === "owner" || input.role === "admin" || input.permissions.has("issue:handle") || input.permissions.has("event:handle")) return;
  throw new Error("SENSITIVE_MEDIA_DOWNLOAD_DENIED");
}

export class MediaAccessSigner {
  readonly #secret: string;
  readonly #now: () => Date;

  constructor(secret: string, now: () => Date = () => new Date()) {
    if (secret.length < 16) throw new Error("MEDIA_SIGNING_SECRET_TOO_SHORT");
    this.#secret = secret;
    this.#now = now;
  }

  issue(input: { projectId: number; assetId: number; action: MediaAccessAction; ttlSeconds?: number }) {
    const ttlSeconds = input.ttlSeconds ?? 120;
    if (!Number.isInteger(ttlSeconds) || ttlSeconds < 1 || ttlSeconds > 300) throw new Error("INVALID_MEDIA_ACCESS_TTL");
    const expiresAt = new Date(this.#now().getTime() + ttlSeconds * 1000);
    const expires = String(expiresAt.getTime());
    const signature = this.#signature(input.projectId, input.assetId, input.action, expires);
    return {
      url: `/api/projects/${input.projectId}/assets/${input.assetId}/content?action=${input.action}&expires=${expires}&signature=${signature}`,
      expiresAt: expiresAt.toISOString()
    };
  }

  verify(input: { projectId: number; assetId: number; action: MediaAccessAction; expires: string; signature: string }) {
    if (!/^\d+$/.test(input.expires) || Number(input.expires) <= this.#now().getTime()) return false;
    const expected = Buffer.from(this.#signature(input.projectId, input.assetId, input.action, input.expires));
    const actual = Buffer.from(input.signature);
    return actual.length === expected.length && timingSafeEqual(actual, expected);
  }

  #signature(projectId: number, assetId: number, action: MediaAccessAction, expires: string) {
    return createHmac("sha256", this.#secret).update(`${projectId}\n${assetId}\n${action}\n${expires}`).digest("hex");
  }
}

export function safeDownloadName(value: string | null, fallback: string) {
  const normalized = (value ?? fallback).normalize("NFKC").replace(/[^a-zA-Z0-9._-]+/g, "-").slice(0, 128);
  return normalized || fallback;
}
