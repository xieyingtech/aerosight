import { createHash } from "node:crypto";

import { z } from "zod";

import {
  FlightHubClientError,
  type FlightHubProject,
  type FlightHubProjectClient,
  type FlightHubSafeErrorCode,
} from "./dji-flighthub-client-core.ts";

export const flightHubConnectionInputSchema = z.object({
  token: z.string().trim().min(1).max(16_384),
  projectUuid: z.string().uuid(),
}).strict();

export type FlightHubConnectionErrorCode =
  | FlightHubSafeErrorCode
  | "access_denied"
  | "project_access_changed"
  | "duplicate_connection"
  | "connector_not_found"
  | "connector_disabled"
  | "configuration_unavailable";

export class FlightHubConnectionError extends Error {
  readonly safeCode: FlightHubConnectionErrorCode;
  readonly retryAfterSeconds?: number;

  constructor(safeCode: FlightHubConnectionErrorCode, retryAfterSeconds?: number) {
    super(`DJI_FLIGHTHUB_CONNECTION_${safeCode.toUpperCase()}`);
    this.name = "FlightHubConnectionError";
    this.safeCode = safeCode;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

export type FlightHubConnectionPlan = {
  connectorKey: "dji.flighthub2";
  connectorVersion: "1.0.0";
  adapterType: "dji-flighthub2";
  vendor: "dji";
  protocolVersion: "flighthub-openapi-v2";
  name: string;
  externalScopeKey: string;
  discoveryScope: { projectUuid: string; projectName: string };
  config: { region: "cn"; readOnly: true };
  capabilities: { inventoryRead: true; stateRead: true };
};

export async function revalidateSelectedFlightHubProject(
  client: Pick<FlightHubProjectClient, "listProjects">,
  token: string,
  selectedProjectUuid: string
): Promise<FlightHubProject> {
  let projects: FlightHubProject[];
  try {
    projects = await client.listProjects(token);
  } catch (error) {
    if (error instanceof FlightHubClientError) {
      throw new FlightHubConnectionError(error.safeCode, error.retryAfterSeconds);
    }
    throw new FlightHubConnectionError("upstream_error");
  }
  const normalized = selectedProjectUuid.toLowerCase();
  const selected = projects.find((project) => project.uuid === normalized);
  if (!selected) throw new FlightHubConnectionError("project_access_changed");
  return selected;
}

export function buildFlightHubConnectionPlan(project: FlightHubProject): FlightHubConnectionPlan {
  const displayName = `DJI 司空 2 · ${project.name}`;
  return {
    connectorKey: "dji.flighthub2",
    connectorVersion: "1.0.0",
    adapterType: "dji-flighthub2",
    vendor: "dji",
    protocolVersion: "flighthub-openapi-v2",
    name: displayName.length <= 100 ? displayName : `${displayName.slice(0, 97)}...`,
    externalScopeKey: project.uuid,
    discoveryScope: { projectUuid: project.uuid, projectName: project.name },
    config: { region: "cn", readOnly: true },
    capabilities: { inventoryRead: true, stateRead: true },
  };
}

export function flightHubScopeFingerprint(projectUuid: string) {
  return createHash("sha256").update(projectUuid.toLowerCase()).digest("hex").slice(0, 12);
}
