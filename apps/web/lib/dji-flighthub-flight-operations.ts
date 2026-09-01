import "server-only";

import { db } from "@/lib/db";
import { requireCurrentProjectPermission } from "@/lib/data";
import { readFlightHubFlightOperationsCore } from "@/lib/dji-flighthub-flight-operations-core";

export class FlightHubFlightOperationsError extends Error {
  constructor(readonly safeCode: "invalid_project" | "access_denied") {
    super(safeCode);
  }
}

export async function readFlightHubFlightOperations(projectId: number) {
  if (!Number.isSafeInteger(projectId) || projectId <= 0) throw new FlightHubFlightOperationsError("invalid_project");
  let scope;
  try {
    scope = await requireCurrentProjectPermission(projectId, "project:view");
  } catch {
    throw new FlightHubFlightOperationsError("access_denied");
  }
  const operations = await readFlightHubFlightOperationsCore(scope.user.id, projectId, () => db.connect());
  if (!operations) throw new FlightHubFlightOperationsError("access_denied");
  return operations;
}
