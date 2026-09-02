import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { authorizeFlightHubModelDelete, flightHubModelDeleteInputSchema, modelDeletePreview,
  type FlightHubModelDeleteAuthorization } from "./dji-flighthub-model-actions-core.ts";

const digest = "a".repeat(64);
const input = flightHubModelDeleteInputSchema.parse({ action: "model-delete", connectorInstanceId: 12,
  targetResourceId: 91, approvalRequestId: "00000000-0000-4000-8000-000000000001",
  expectedRemoteVersion: "version-2", previewDigest: digest, idempotencyKey: "model-delete-0001",
  request: { confirmation: "DELETE" } });
const allowed: FlightHubModelDeleteAuthorization = {
  teamId: 7, role: "admin", connectorProjectId: 41, connectorTeamId: 7, connectorStatus: "connected",
  actionEnabled: true, capabilityFieldVerified: true, targetProjectId: 41, targetConnectorId: 12,
  targetKind: "model", targetStatus: "active", targetRemoteVersion: "version-2",
  approvalProjectId: 41, approvalTeamId: 7, approvalResourceType: "connector_remote_resource",
  approvalResourceId: "91", approvalAction: "flighthub.model.delete", approvalStatus: "approved",
  approvalUnexpired: true, approvalPreviewDigest: digest, approvalRemoteVersion: "version-2",
  currentPreviewDigest: digest
};

test("model delete requires owner/admin, field-write evidence, flag, confirmation and exact approval", () => {
  for (const denied of [
    { ...allowed, role: "member" }, { ...allowed, actionEnabled: false },
    { ...allowed, capabilityFieldVerified: false }, { ...allowed, approvalStatus: "pending" },
    { ...allowed, approvalUnexpired: false }, { ...allowed, approvalProjectId: 99 },
    { ...allowed, approvalResourceId: "92" }, { ...allowed, approvalPreviewDigest: "b".repeat(64) }
  ]) assert.throws(() => authorizeFlightHubModelDelete(41, input, denied), /FLIGHTHUB_MODEL_DELETE_/);
  assert.equal(authorizeFlightHubModelDelete(41, input, allowed).capability, "model.delete");
  assert.throws(() => flightHubModelDeleteInputSchema.parse({ ...input, request: { confirmation: "yes" } }));
});

test("model resource delete uses a distinct capability, feature flag, target kind and approval action", () => {
  const resource = flightHubModelDeleteInputSchema.parse({ ...input, action: "model-resource-delete" });
  const plan = authorizeFlightHubModelDelete(41, resource, { ...allowed, targetKind: "model-resource",
    approvalAction: "flighthub.model-resource.delete" });
  assert.equal(plan.capability, "model.resource.delete");
  assert.equal(plan.featureFlag, "flighthub.model-resource.delete");
});

test("stale preview and cross-tenant target fail closed", () => {
  assert.throws(() => authorizeFlightHubModelDelete(41, input, { ...allowed, currentPreviewDigest: "b".repeat(64) }),
    /PREVIEW_CONFLICT/);
  assert.throws(() => authorizeFlightHubModelDelete(41, input, { ...allowed, targetProjectId: 99 }), /SCOPE_MISMATCH/);
  assert.deepEqual(modelDeletePreview({ targetResourceId: 91, resourceKind: "model", remoteVersion: "version-2",
    assetId: "31", assetStatus: "available", dependentReferenceCount: 1 }).effect,
  "remote-delete-and-local-mark-missing");
});

test("model delete API never exposes encrypted payloads or remote identifiers", () => {
  const route = readFileSync(new URL("../app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/model-actions/route.ts", import.meta.url), "utf8");
  const service = readFileSync(new URL("./dji-flighthub-model-actions.ts", import.meta.url), "utf8");
  const publicProjection = service.slice(service.indexOf("export async function readFlightHubModelDeleteJob"));
  assert.match(service, /withAuditedProjectWrite/);
  assert.match(service, /flighthub\.model_delete\.requested/);
  assert.doesNotMatch(route, /request_envelope|remote_id/i);
  assert.doesNotMatch(publicProjection, /request_envelope|requestDigest|remote_id/i);
});
