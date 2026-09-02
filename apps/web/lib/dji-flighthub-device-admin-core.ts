import { z } from "zod";

const common = {
  connectorInstanceId: z.number().int().positive(), idempotencyKey: z.string().trim().min(8).max(200),
  approvalRequestId: z.string().uuid()
};
const identifier = z.string().trim().min(1).max(256).regex(/^[A-Za-z0-9._:-]+$/);

export const flightHubDeviceAdminInputSchema = z.discriminatedUnion("action", [
  z.object({ ...common, action: z.literal("rtk-calibrate"), deviceId: z.number().int().positive(), confirmation: z.literal("CALIBRATE RTK"),
    request: z.object({ host: z.string().trim().min(1).max(253), port: z.number().int().min(1).max(65535), account: z.string().min(1).max(256),
      password: z.string().min(1).max(4096), mountPoint: z.string().min(1).max(256) }).strict() }).strict(),
  z.object({ ...common, action: z.literal("relay-pair"), deviceId: z.number().int().positive(), confirmation: z.literal("PAIR RELAY"),
    request: z.object({ pairEnable: z.boolean(), pairType: z.enum(["drone", "relay"]) }).strict() }).strict(),
  z.object({ ...common, action: z.literal("active-project-update"), deviceId: z.number().int().positive(), confirmation: z.literal("MOVE DEVICE"),
    request: z.object({ activeProjectUuid: identifier }).strict() }).strict(),
  z.object({ ...common, action: z.literal("sn-decrypt"), confirmation: z.literal("DECRYPT SN"),
    request: z.object({ encryptedSNs: z.array(z.string().trim().min(1).max(4096)).min(1).max(100) }).strict() }).strict()
]);

export type FlightHubDeviceAdminInput = z.infer<typeof flightHubDeviceAdminInputSchema>;

export const DEVICE_ADMIN_POLICY = Object.freeze({
  "rtk-calibrate": { capability: "device.rtk.calibrate", featureFlag: "flighthub.rtk.calibrate" },
  "relay-pair": { capability: "device.relay.pair", featureFlag: "flighthub.relay.pair" },
  "active-project-update": { capability: "device.active-project.update", featureFlag: "flighthub.device-migration" },
  "sn-decrypt": { capability: "security.sn.decrypt", featureFlag: "flighthub.sn-decrypt" }
} as const);

export function authorizeFlightHubDeviceAdmin(projectId: number, input: FlightHubDeviceAdminInput, authorization: {
  teamId: number; role: string; connectorProjectId: number; connectorTeamId: number; connectorStatus: string;
  featureEnabled: boolean; capabilityVerified: boolean; deviceProjectId: number | null; identityPresent: boolean;
  deviceOnline: boolean; stateFresh: boolean; approvalProjectId: number | null; approvalTeamId: number | null;
  approvalResourceType: string | null; approvalResourceId: string | null; approvalAction: string | null;
  approvalStatus: string | null; approvalUnexpired: boolean;
}) {
  const policy = DEVICE_ADMIN_POLICY[input.action];
  if (!new Set(["owner", "admin"]).has(authorization.role)) throw new Error("FLIGHTHUB_DEVICE_ADMIN_PERMISSION_DENIED");
  if (authorization.connectorProjectId !== projectId || authorization.connectorTeamId !== authorization.teamId) throw new Error("FLIGHTHUB_DEVICE_ADMIN_SCOPE_MISMATCH");
  if (authorization.connectorStatus !== "connected") throw new Error("FLIGHTHUB_DEVICE_ADMIN_CONNECTOR_OFFLINE");
  if (!authorization.featureEnabled || !authorization.capabilityVerified) throw new Error("FLIGHTHUB_DEVICE_ADMIN_DISABLED");
  const deviceId = "deviceId" in input ? input.deviceId : null;
  if (deviceId !== null && (authorization.deviceProjectId !== projectId || !authorization.identityPresent
      || !authorization.deviceOnline || !authorization.stateFresh)) throw new Error("FLIGHTHUB_DEVICE_ADMIN_DEVICE_UNAVAILABLE");
  const resourceType = deviceId === null ? "connector" : "device";
  const resourceId = String(deviceId ?? input.connectorInstanceId);
  if (authorization.approvalProjectId !== projectId || authorization.approvalTeamId !== authorization.teamId
      || authorization.approvalResourceType !== resourceType || authorization.approvalResourceId !== resourceId
      || authorization.approvalAction !== `flighthub.admin.${input.action}` || authorization.approvalStatus !== "approved"
      || !authorization.approvalUnexpired) throw new Error("FLIGHTHUB_DEVICE_ADMIN_APPROVAL_REQUIRED");
  return Object.freeze({ ...input, projectId, teamId: authorization.teamId, capability: policy.capability, featureFlag: policy.featureFlag });
}
