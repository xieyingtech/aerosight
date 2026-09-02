import "server-only";

import { db } from "@/lib/db";
import { requireCurrentProjectPermission } from "@/lib/data";
import { readFlightHubGeospatialCore } from "@/lib/dji-flighthub-geospatial-core";

export class FlightHubGeospatialError extends Error {
  constructor(readonly safeCode: "invalid_project" | "access_denied") {
    super(safeCode);
  }
}

export async function readFlightHubGeospatial(projectId: number) {
  if (!Number.isSafeInteger(projectId) || projectId <= 0) throw new FlightHubGeospatialError("invalid_project");
  let scope;
  try {
    scope = await requireCurrentProjectPermission(projectId, "project:view");
  } catch {
    throw new FlightHubGeospatialError("access_denied");
  }
  const workspace = await readFlightHubGeospatialCore(scope.user.id, projectId, () => db.connect());
  if (!workspace) throw new FlightHubGeospatialError("access_denied");
  return workspace;
}
