import "server-only";

import { db } from "@/lib/db";
import { requireCurrentProjectPermission } from "@/lib/data";
import { readFlightHubModelsCore } from "@/lib/dji-flighthub-models-core";

export class FlightHubModelsError extends Error {
  constructor(readonly safeCode: "invalid_project" | "access_denied") { super(safeCode); }
}

export async function readFlightHubModels(projectId: number) {
  if (!Number.isSafeInteger(projectId) || projectId <= 0) throw new FlightHubModelsError("invalid_project");
  let scope;
  try { scope = await requireCurrentProjectPermission(projectId, "project:view"); }
  catch { throw new FlightHubModelsError("access_denied"); }
  const models = await readFlightHubModelsCore(scope.user.id, projectId, () => db.connect());
  if (!models) throw new FlightHubModelsError("access_denied");
  return models;
}
