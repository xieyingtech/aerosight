import { z } from "zod";

export const MODEL_DELETE_POLICY = Object.freeze({
  "model-delete": {
    capability: "model.delete",
    featureFlag: "flighthub.model.delete",
    approvalAction: "flighthub.model.delete",
    targetKind: "model"
  },
  "model-resource-delete": {
    capability: "model.resource.delete",
    featureFlag: "flighthub.model-resource.delete",
    approvalAction: "flighthub.model-resource.delete",
    targetKind: "model-resource"
  }
} as const);

export const flightHubModelDeleteInputSchema = z.object({
  action: z.enum(["model-delete", "model-resource-delete"]),
  connectorInstanceId: z.number().int().positive(),
  targetResourceId: z.number().int().positive(),
  approvalRequestId: z.string().uuid(),
  expectedRemoteVersion: z.string().trim().min(1).max(512),
  previewDigest: z.string().regex(/^[a-f0-9]{64}$/),
  idempotencyKey: z.string().trim().min(8).max(200),
  request: z.object({ confirmation: z.literal("DELETE") }).strict()
}).strict();

export type FlightHubModelDeleteInput = z.infer<typeof flightHubModelDeleteInputSchema>;

export type FlightHubModelDeletePreview = {
  targetResourceId: number;
  resourceKind: string;
  remoteVersion: string;
  assetId: string | null;
  assetStatus: string | null;
  dependentReferenceCount: number;
  effect: "remote-delete-and-local-mark-missing";
};

export function modelDeletePreview(input: Omit<FlightHubModelDeletePreview, "effect">): FlightHubModelDeletePreview {
  return Object.freeze({ ...input, effect: "remote-delete-and-local-mark-missing" });
}

export type FlightHubModelDeleteAuthorization = {
  teamId: number;
  role: string;
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
  approvalProjectId: number | null;
  approvalTeamId: number | null;
  approvalResourceType: string | null;
  approvalResourceId: string | null;
  approvalAction: string | null;
  approvalStatus: string | null;
  approvalUnexpired: boolean;
  approvalPreviewDigest: string | null;
  approvalRemoteVersion: string | null;
  currentPreviewDigest: string;
};

export function authorizeFlightHubModelDelete(projectId: number, input: FlightHubModelDeleteInput,
  authorization: FlightHubModelDeleteAuthorization) {
  const policy = MODEL_DELETE_POLICY[input.action];
  if (!new Set(["owner", "admin"]).has(authorization.role)) {
    throw new Error("FLIGHTHUB_MODEL_DELETE_PERMISSION_DENIED");
  }
  if (authorization.connectorProjectId !== projectId || authorization.connectorTeamId !== authorization.teamId) {
    throw new Error("FLIGHTHUB_MODEL_DELETE_SCOPE_MISMATCH");
  }
  if (!new Set(["connecting", "connected", "degraded"]).has(authorization.connectorStatus)) {
    throw new Error("FLIGHTHUB_MODEL_DELETE_CONNECTOR_DISABLED");
  }
  if (!authorization.actionEnabled || !authorization.capabilityFieldVerified) {
    throw new Error("FLIGHTHUB_MODEL_DELETE_DISABLED");
  }
  if (authorization.targetProjectId !== projectId || authorization.targetConnectorId !== input.connectorInstanceId
    || authorization.targetKind !== policy.targetKind || authorization.targetStatus !== "active") {
    throw new Error("FLIGHTHUB_MODEL_DELETE_SCOPE_MISMATCH");
  }
  if (authorization.targetRemoteVersion !== input.expectedRemoteVersion
    || authorization.currentPreviewDigest !== input.previewDigest) {
    throw new Error("FLIGHTHUB_MODEL_DELETE_PREVIEW_CONFLICT");
  }
  if (authorization.approvalProjectId !== projectId || authorization.approvalTeamId !== authorization.teamId
    || authorization.approvalResourceType !== "connector_remote_resource"
    || authorization.approvalResourceId !== String(input.targetResourceId)
    || authorization.approvalAction !== policy.approvalAction || authorization.approvalStatus !== "approved"
    || !authorization.approvalUnexpired || authorization.approvalPreviewDigest !== input.previewDigest
    || authorization.approvalRemoteVersion !== input.expectedRemoteVersion) {
    throw new Error("FLIGHTHUB_MODEL_DELETE_APPROVAL_REQUIRED");
  }
  return Object.freeze({ ...input, projectId, teamId: authorization.teamId, ...policy,
    completion: "worker-final" as const });
}
