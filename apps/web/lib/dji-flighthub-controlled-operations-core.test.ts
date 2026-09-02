import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { buildFlightHubControlledOperations, FLIGHTHUB_CONTROLLED_OPERATIONS } from "./dji-flighthub-controlled-operations-core.ts";
import { authorizeProjectMemberWrite, bindProjectMemberWriteRequest } from "./dji-flighthub-management-write-core.ts";

const definition = FLIGHTHUB_CONTROLLED_OPERATIONS.find((item) => item.capabilityCode === "organization.project-member.write")!;

test("controlled operation availability is the server-side capability intersection", () => {
  const base = { projectId: 11, connectorStatus: "connected", role: "owner", permissions: new Set(["mission:operate"]),
    managementGranted: true, manifestCapabilities: new Set([definition.capabilityCode]),
    featureFlags: { [definition.featureFlag]: true }, fieldWriteCapabilities: new Set([definition.capabilityCode]), jobs: [] };
  assert.equal(buildFlightHubControlledOperations(base).actions[0]?.available, true);
  for (const override of [{ connectorStatus: "disabled" }, { managementGranted: false }, { featureFlags: {} },
    { fieldWriteCapabilities: new Set<string>() }, { manifestCapabilities: new Set<string>() }]) {
    const result = buildFlightHubControlledOperations({ ...base, ...override });
    assert(!result.actions[0]?.available);
  }
});

test("controlled operation catalog declares risk, prerequisites, approval, flag and final evidence", () => {
  assert(FLIGHTHUB_CONTROLLED_OPERATIONS.length >= 20);
  for (const item of FLIGHTHUB_CONTROLLED_OPERATIONS) {
    assert.match(item.featureFlag, /^[a-z][a-z0-9.-]+$/);
    assert(item.prerequisites.length > 0 && item.approval && item.resultEvidence);
  }
  const component = readFileSync(new URL("../components/dji-flighthub-controlled-operations-panel.tsx", import.meta.url), "utf8");
  for (const label of ["风险", "前置条件", "审批", "功能开关", "最终结果"]) assert(component.includes(label));
});

test("old clients cannot forge connector scope or capability gates through the write API", () => {
  const raw = { connectorInstanceId: 999, members: [{ userId: "ORG_USER_VENDOR_ID_REDACTED", role: "project-member", nickname: "成员" }],
    confirmation: "ADD PROJECT MEMBER", previewDigest: "a".repeat(64), approvalRequestId: "00000000-0000-4000-8000-000000000008",
    idempotencyKey: "member-0002" };
  assert.equal(bindProjectMemberWriteRequest(8, raw).connectorInstanceId, 8);
  assert.throws(() => bindProjectMemberWriteRequest(8, { ...raw, featureEnabled: true, capabilityVerified: true, projectId: 11 }));
  const bound = bindProjectMemberWriteRequest(8, raw), calls: string[] = [];
  try {
    authorizeProjectMemberWrite(11, bound, { teamId: 7, managementGranted: false, connectorProjectId: 11,
      connectorTeamId: 7, connectorStatus: "connected", featureEnabled: true, capabilityVerified: true, targetCount: 1,
      currentPreviewDigest: bound.previewDigest, approvalProjectId: 11, approvalTeamId: 7, approvalResourceType: "connector",
      approvalResourceId: "8", approvalAction: "flighthub.organization.project-member-upsert", approvalStatus: "approved",
      approvalUnexpired: true, approvalPreviewDigest: bound.previewDigest });
    calls.push("upstream");
  } catch { /* the service-side gate rejects before enqueue/upstream */ }
  assert.deepEqual(calls, []);
});

test("controlled operation result API omits encrypted requests and vendor result payloads", () => {
  const service = readFileSync(new URL("./dji-flighthub-controlled-operations.ts", import.meta.url), "utf8");
  const route = readFileSync(new URL("../app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/controlled-operations/route.ts", import.meta.url), "utf8");
  assert.doesNotMatch(service, /request_envelope_json|result_envelope_json|credential_envelope_json|result_json/);
  assert.match(route, /readFlightHubControlledOperations/);
});
