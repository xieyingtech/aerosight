import "server-only";

import { FlightHubProjectClient } from "./dji-flighthub-client-core.ts";
import { parseFlightHubWebConfig } from "./dji-flighthub-config.ts";

export function createFlightHubProjectClient(
  environment: Record<string, string | undefined> = process.env
) {
  const config = parseFlightHubWebConfig(environment);
  if (!config.enabled) throw new Error("DJI_FLIGHTHUB_DISABLED");
  return new FlightHubProjectClient({
    apiBaseUrl: config.apiBaseUrl,
    timeoutMs: config.timeoutMs,
    maxRetries: config.maxRetries,
    maxProjectPages: config.maxProjectPages,
    maxResponseBytes: config.maxResponseBytes,
  });
}
