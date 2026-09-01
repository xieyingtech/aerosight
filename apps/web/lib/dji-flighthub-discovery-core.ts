import { z } from "zod";

import {
  FlightHubClientError,
  type FlightHubProject,
  type FlightHubProjectClient,
  type FlightHubSafeErrorCode,
} from "./dji-flighthub-client-core.ts";

export const flightHubDiscoveryInputSchema = z.object({
  token: z.string().trim().min(1).max(16_384),
}).strict();

export type FlightHubDiscoveryErrorCode =
  | FlightHubSafeErrorCode
  | "access_denied"
  | "rate_limited";

export class FlightHubDiscoveryError extends Error {
  readonly safeCode: FlightHubDiscoveryErrorCode;
  readonly retryAfterSeconds?: number;

  constructor(safeCode: FlightHubDiscoveryErrorCode, retryAfterSeconds?: number) {
    super(`DJI_FLIGHTHUB_DISCOVERY_${safeCode.toUpperCase()}`);
    this.name = "FlightHubDiscoveryError";
    this.safeCode = safeCode;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

type RateLimitBucket = { timestamps: number[] };

export class FlightHubDiscoveryRateLimiter {
  private readonly buckets = new Map<string, RateLimitBucket>();
  private readonly limit: number;
  private readonly windowMs: number;
  private readonly now: () => number;

  constructor(
    limit = 5,
    windowMs = 60_000,
    now = () => Date.now()
  ) {
    this.limit = limit;
    this.windowMs = windowMs;
    this.now = now;
  }

  consume(key: string) {
    const current = this.now();
    const cutoff = current - this.windowMs;
    const bucket = this.buckets.get(key) ?? { timestamps: [] };
    bucket.timestamps = bucket.timestamps.filter((timestamp) => timestamp > cutoff);
    if (bucket.timestamps.length >= this.limit) {
      const retryAfterSeconds = Math.max(
        1,
        Math.ceil((bucket.timestamps[0]! + this.windowMs - current) / 1_000)
      );
      throw new FlightHubDiscoveryError("rate_limited", retryAfterSeconds);
    }
    bucket.timestamps.push(current);
    this.buckets.set(key, bucket);
  }
}

export type FlightHubDiscoveryAudit = {
  status: "succeeded" | "failed";
  projectCount: number;
  errorCode?: FlightHubDiscoveryErrorCode;
};

type DiscoveryDependencies = {
  projectId: number;
  accessProjectId: number;
  actorUserId: number;
  role: "owner" | "admin" | "member" | null;
  token: string;
  client: Pick<FlightHubProjectClient, "listProjects">;
  rateLimiter: FlightHubDiscoveryRateLimiter;
  audit: (summary: FlightHubDiscoveryAudit) => Promise<void>;
  log: (summary: FlightHubDiscoveryAudit & { projectId: number; actorUserId: number }) => void;
};

function normalizeDiscoveryError(error: unknown) {
  if (error instanceof FlightHubDiscoveryError) return error;
  if (error instanceof FlightHubClientError) {
    return new FlightHubDiscoveryError(error.safeCode, error.retryAfterSeconds);
  }
  return new FlightHubDiscoveryError("upstream_error");
}

export async function discoverFlightHubProjectsCore(
  dependencies: DiscoveryDependencies
): Promise<FlightHubProject[]> {
  if (
    dependencies.accessProjectId !== dependencies.projectId ||
    (dependencies.role !== "owner" && dependencies.role !== "admin")
  ) {
    throw new FlightHubDiscoveryError("access_denied");
  }

  const auditAndLog = async (summary: FlightHubDiscoveryAudit) => {
    await dependencies.audit(summary);
    dependencies.log({
      ...summary,
      projectId: dependencies.projectId,
      actorUserId: dependencies.actorUserId,
    });
  };

  try {
    dependencies.rateLimiter.consume(`${dependencies.actorUserId}:${dependencies.projectId}`);
    const projects = await dependencies.client.listProjects(dependencies.token);
    await auditAndLog({ status: "succeeded", projectCount: projects.length });
    return projects;
  } catch (error) {
    const safeError = normalizeDiscoveryError(error);
    await auditAndLog({ status: "failed", projectCount: 0, errorCode: safeError.safeCode });
    throw safeError;
  }
}
