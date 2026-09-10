import "server-only";

import { db } from "@/lib/db";
import { requireCurrentProjectPermission } from "@/lib/data";
import { FlightHubConnectionError } from "@/lib/dji-flighthub-connection-core";
import { readFlightHubConnectorDiagnostics as readDiagnosticsCore } from "@/lib/dji-flighthub-diagnostics-core";

export async function readFlightHubConnectorDiagnostics(projectId: number, connectorId: string) {
  if (!Number.isSafeInteger(projectId) || projectId <= 0 || !/^\d+$/.test(connectorId)) {
    throw new FlightHubConnectionError("connector_not_found");
  }
  let scope;
  try {
    scope = await requireCurrentProjectPermission(projectId, "project:view");
  } catch {
    throw new FlightHubConnectionError("access_denied");
  }
  const diagnostics = await readDiagnosticsCore(scope.user.id, projectId, connectorId, () => db.connect());
  if (!diagnostics) throw new FlightHubConnectionError("connector_not_found");
  return diagnostics;
}
