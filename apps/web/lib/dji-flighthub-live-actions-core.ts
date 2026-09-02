import { z } from "zod";

const common = {
  connectorInstanceId: z.number().int().positive(),
  idempotencyKey: z.string().trim().min(8).max(200)
};

const rtmpRequest = z.object({
  name: z.string().trim().min(1).max(256), cameraIndex: z.string().trim().min(1).max(256), schema: z.literal("rtmp"),
  schemaOption: z.object({ url: z.string().trim().min(1).max(4096) }).strict()
}).strict();
const gb28181Request = z.object({
  name: z.string().trim().min(1).max(256), cameraIndex: z.string().trim().min(1).max(256), schema: z.literal("gb28181"),
  schemaOption: z.object({ serverIp: z.string().trim().min(1).max(4096), serverPort: z.string().trim().min(1).max(4096),
    devicePassword: z.string().trim().min(1).max(4096), localPort: z.string().trim().min(1).max(4096),
    deviceId: z.string().trim().min(1).max(4096), localChannel: z.string().trim().min(1).max(4096) }).strict()
}).strict();
const rtspRequest = z.object({
  name: z.string().trim().min(1).max(256), cameraIndex: z.string().trim().min(1).max(256), schema: z.literal("rtsp"),
  schemaOption: z.object({ username: z.string().trim().min(1).max(4096), password: z.string().trim().min(1).max(4096),
    enableTs: z.boolean() }).strict()
}).strict();

export const flightHubLiveActionInputSchema = z.discriminatedUnion("action", [
  z.object({ ...common, action: z.literal("live-quality-set"), deviceId: z.number().int().positive(),
    request: z.object({ cameraIndex: z.string().trim().min(1).max(256), qualityType: z.enum(["adaptive", "smooth", "ultra_high_definition"]) }).strict() }).strict(),
  z.object({ ...common, action: z.literal("live-converter-create"), deviceId: z.number().int().positive(),
    request: z.union([rtmpRequest, gb28181Request, rtspRequest]) }).strict(),
  z.object({ ...common, action: z.literal("live-converter-toggle"), targetResourceId: z.number().int().positive(),
    request: z.object({ enabled: z.boolean() }).strict() }).strict(),
  z.object({ ...common, action: z.literal("live-converter-delete"), targetResourceId: z.number().int().positive(),
    request: z.object({ confirmation: z.literal("DELETE") }).strict() }).strict()
]);

export type FlightHubLiveActionInput = z.infer<typeof flightHubLiveActionInputSchema>;

export const LIVE_ACTION_POLICY = Object.freeze({
  "live-quality-set": { capability: "live.quality.set", featureFlag: "flighthub.live.quality", ownerOnly: false },
  "live-converter-create": { capability: "live.converter.create", featureFlag: "flighthub.live.converter.create", ownerOnly: false },
  "live-converter-toggle": { capability: "live.converter.toggle", featureFlag: "flighthub.live.converter.toggle", ownerOnly: false },
  "live-converter-delete": { capability: "live.converter.delete", featureFlag: "flighthub.live.converter.delete", ownerOnly: true }
} as const);

export type FlightHubLiveActionAuthorization = {
  teamId: number;
  role: string;
  hasOperatePermission: boolean;
  connectorProjectId: number;
  connectorTeamId: number;
  connectorStatus: string;
  actionEnabled: boolean;
  capabilityFieldVerified: boolean;
  deviceProjectId: number | null;
  deviceConnectorIdentityPresent: boolean;
  targetProjectId: number | null;
  targetConnectorId: number | null;
  targetKind: string | null;
  targetStatus: string | null;
};

export function authorizeFlightHubLiveAction(projectId: number, input: FlightHubLiveActionInput,
  authorization: FlightHubLiveActionAuthorization) {
  const policy = LIVE_ACTION_POLICY[input.action];
  if (!authorization.hasOperatePermission || (policy.ownerOnly && !new Set(["owner", "admin"]).has(authorization.role))) {
    throw new Error("FLIGHTHUB_LIVE_ACTION_PERMISSION_DENIED");
  }
  if (authorization.connectorProjectId !== projectId || authorization.connectorTeamId !== authorization.teamId) {
    throw new Error("FLIGHTHUB_LIVE_ACTION_SCOPE_MISMATCH");
  }
  if (!new Set(["connecting", "connected", "degraded"]).has(authorization.connectorStatus)) {
    throw new Error("FLIGHTHUB_LIVE_ACTION_CONNECTOR_DISABLED");
  }
  if (!authorization.actionEnabled || !authorization.capabilityFieldVerified) {
    throw new Error("FLIGHTHUB_LIVE_ACTION_DISABLED");
  }
  if ("deviceId" in input) {
    if (authorization.deviceProjectId !== projectId || !authorization.deviceConnectorIdentityPresent) {
      throw new Error("FLIGHTHUB_LIVE_ACTION_DEVICE_SCOPE_MISMATCH");
    }
  } else if (authorization.targetProjectId !== projectId || authorization.targetConnectorId !== input.connectorInstanceId
    || authorization.targetKind !== "stream-converter" || authorization.targetStatus !== "active") {
    throw new Error("FLIGHTHUB_LIVE_ACTION_TARGET_SCOPE_MISMATCH");
  }
  return Object.freeze({ ...input, projectId, teamId: authorization.teamId, capability: policy.capability,
    featureFlag: policy.featureFlag, completion: "worker-final" as const });
}
