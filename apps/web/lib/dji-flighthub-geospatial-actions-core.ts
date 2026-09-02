import { z } from "zod";

const common = {
  connectorInstanceId: z.number().int().positive(),
  idempotencyKey: z.string().trim().min(8).max(200)
};

const geoJSONFeature = z.object({
  type: z.literal("Feature"),
  properties: z.record(z.string(), z.unknown()),
  geometry: z.object({
    type: z.enum(["Point", "LineString", "Polygon"]),
    coordinates: z.unknown()
  }).strict()
}).strict();

const mapElementCreateRequest = z.object({
  name: z.string().trim().min(1).max(256),
  desc: z.string().trim().max(4096).optional(),
  resource: z.object({
    type: z.number().int().min(0).max(2),
    remark: z.string().trim().max(4096).optional(),
    content: geoJSONFeature
  }).strict()
}).strict();

const mapElementUpdateRequest = z.object({
  name: z.string().trim().min(1).max(256).optional(),
  status: z.number().int().nonnegative().optional(),
  display: z.number().int().min(0).max(1).optional(),
  content: geoJSONFeature.optional(),
  remark: z.string().trim().max(4096).optional(),
  elevation_load_status: z.number().int().nonnegative().optional(),
  target_layer_id: z.string().trim().min(1).max(256).optional()
}).strict().refine((value) => Object.keys(value).length > 0, "an update field is required");

export const flightHubGeospatialActionInputSchema = z.discriminatedUnion("action", [
  z.object({ ...common, action: z.literal("map-element-create"), request: mapElementCreateRequest }).strict(),
  z.object({ ...common, action: z.literal("map-element-update"), targetResourceId: z.number().int().positive(),
    expectedRemoteVersion: z.string().trim().min(1).max(512), request: mapElementUpdateRequest }).strict(),
  z.object({ ...common, action: z.literal("map-element-delete"), targetResourceId: z.number().int().positive(),
    expectedRemoteVersion: z.string().trim().min(1).max(512),
    request: z.object({ confirmation: z.literal("DELETE") }).strict() }).strict()
]);

export type FlightHubGeospatialActionInput = z.infer<typeof flightHubGeospatialActionInputSchema>;

export const GEOSPATIAL_ACTION_POLICY = Object.freeze({
  "map-element-create": { capability: "geospatial.write", featureFlag: "flighthub.actions", permission: "mission:operate", ownerOnly: false },
  "map-element-update": { capability: "geospatial.write", featureFlag: "flighthub.actions", permission: "mission:operate", ownerOnly: false },
  "map-element-delete": { capability: "geospatial.element.delete", featureFlag: "flighthub.geospatial.delete", permission: "project:admin", ownerOnly: true }
} as const);

export type FlightHubGeospatialActionAuthorization = {
  teamId: number;
  role: string;
  hasOperatePermission: boolean;
  connectorProjectId: number;
  connectorTeamId: number;
  connectorStatus: string;
  actionEnabled: boolean;
  capabilityFieldVerified: boolean;
  targetProjectId: number | null;
  targetConnectorId: number | null;
  targetKind: string | null;
  targetStatus: string | null;
  targetRemoteVersion: string | null;
};

export function authorizeFlightHubGeospatialAction(projectId: number, input: FlightHubGeospatialActionInput,
  authorization: FlightHubGeospatialActionAuthorization) {
  const policy = GEOSPATIAL_ACTION_POLICY[input.action];
  if (!authorization.hasOperatePermission || (policy.ownerOnly && !new Set(["owner", "admin"]).has(authorization.role))) {
    throw new Error("FLIGHTHUB_GEOSPATIAL_ACTION_PERMISSION_DENIED");
  }
  if (authorization.connectorProjectId !== projectId || authorization.connectorTeamId !== authorization.teamId) {
    throw new Error("FLIGHTHUB_GEOSPATIAL_ACTION_SCOPE_MISMATCH");
  }
  if (!new Set(["connecting", "connected", "degraded"]).has(authorization.connectorStatus)) {
    throw new Error("FLIGHTHUB_GEOSPATIAL_ACTION_CONNECTOR_DISABLED");
  }
  if (!authorization.actionEnabled || !authorization.capabilityFieldVerified) {
    throw new Error("FLIGHTHUB_GEOSPATIAL_ACTION_DISABLED");
  }
  if (input.action !== "map-element-create") {
    if (authorization.targetProjectId !== projectId || authorization.targetConnectorId !== input.connectorInstanceId
      || authorization.targetKind !== "map-element" || authorization.targetStatus !== "active") {
      throw new Error("FLIGHTHUB_GEOSPATIAL_ACTION_TARGET_SCOPE_MISMATCH");
    }
    if (authorization.targetRemoteVersion !== input.expectedRemoteVersion) {
      throw new Error("FLIGHTHUB_GEOSPATIAL_ACTION_VERSION_CONFLICT");
    }
  }
  return Object.freeze({ ...input, projectId, teamId: authorization.teamId, capability: policy.capability,
    featureFlag: policy.featureFlag, completion: "worker-final" as const });
}
