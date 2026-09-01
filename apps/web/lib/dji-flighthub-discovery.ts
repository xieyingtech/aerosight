import "server-only";

import { correlationId, structuredLog } from "@/lib/observability";
import { withAuditedProjectWrite } from "@/lib/audit";
import { canManageDeviceAdapters } from "@/lib/device-adapter-policy";
import { requireCurrentProjectPermission } from "@/lib/data";
import { createFlightHubProjectClient } from "@/lib/dji-flighthub-client";
import {
  discoverFlightHubProjectsCore,
  FlightHubDiscoveryError,
  FlightHubDiscoveryRateLimiter,
  flightHubDiscoveryInputSchema,
  type FlightHubDiscoveryAudit,
} from "@/lib/dji-flighthub-discovery-core";

const discoveryRateLimiter = new FlightHubDiscoveryRateLimiter();

export async function discoverFlightHubProjects(
  projectId: number,
  rawInput: unknown,
  requestId?: string | null
) {
  if (!Number.isSafeInteger(projectId) || projectId <= 0) {
    throw new FlightHubDiscoveryError("access_denied");
  }
  const { user, access } = await requireCurrentProjectPermission(projectId, "device:configure");
  if (!canManageDeviceAdapters(access.role)) throw new FlightHubDiscoveryError("access_denied");
  const input = flightHubDiscoveryInputSchema.parse(rawInput);
  const safeRequestId = correlationId(requestId);

  return discoverFlightHubProjectsCore({
    projectId,
    accessProjectId: access.projectId,
    actorUserId: user.id,
    role: access.role,
    token: input.token,
    client: createFlightHubProjectClient(),
    rateLimiter: discoveryRateLimiter,
    audit: async (summary: FlightHubDiscoveryAudit) => {
      await withAuditedProjectWrite(
        {
          projectId,
          teamId: access.teamId,
          requestId: safeRequestId,
          actorUserId: user.id,
          action: "connector.flighthub.discover",
          resourceType: "connector",
          input: { operation: "project-discovery" },
          policyResult: {
            permission: "device:configure",
            role: access.role,
            status: summary.status,
            projectCount: summary.projectCount,
            errorCode: summary.errorCode,
          },
        },
        async () => summary
      );
    },
    log: (summary) => {
      structuredLog(
        summary.status === "succeeded" ? "info" : "warn",
        "DJI FlightHub project discovery completed",
        summary
      );
    },
  });
}
