import { z } from "zod";

export const flightHubTokenUpdateSchema = z.object({
  token: z.string().trim().min(1).max(16_384),
}).strict();

export type FlightHubSyncRequest = {
  connectorInstanceId: string;
  connectorKey: "dji.flighthub2";
  discoveryMode: "poll";
  trigger: "initial" | "manual" | "credential-update";
};

export function buildFlightHubSyncRequest(
  connectorInstanceId: string,
  trigger: FlightHubSyncRequest["trigger"]
): FlightHubSyncRequest {
  if (!/^\d+$/.test(connectorInstanceId)) throw new Error("CONNECTOR_INSTANCE_ID_INVALID");
  return {
    connectorInstanceId,
    connectorKey: "dji.flighthub2",
    discoveryMode: "poll",
    trigger,
  };
}

export function assertFlightHubConnectorEnabled(status: string) {
  if (status === "disabled") throw new Error("DJI_FLIGHTHUB_CONNECTOR_DISABLED");
}
