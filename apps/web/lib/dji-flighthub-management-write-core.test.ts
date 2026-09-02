import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { auditHash } from "./audit-boundary.ts";
import { authorizeProjectMemberWrite, flightHubManagementTargetKey, flightHubProjectMemberWriteInputSchema,
  projectMemberPreview } from "./dji-flighthub-management-write-core.ts";

const members = [{ userId: "ORG_USER_VENDOR_ID_REDACTED", role: "project-admin" as const, nickname: "现场管理员" }];
const preview = projectMemberPreview({ projectId: 11, connectorInstanceId: 8, projectName: "项目", organizationName: "组织", members });
const digest = auditHash(preview);
const input = flightHubProjectMemberWriteInputSchema.parse({ members, confirmation: "ADD PROJECT MEMBER", previewDigest: digest,
  approvalRequestId: "00000000-0000-4000-8000-000000000008", idempotencyKey: "member-0001", connectorInstanceId: 8 });
const allowed = { teamId: 7, managementGranted: true, connectorProjectId: 11, connectorTeamId: 7, connectorStatus: "connected",
  featureEnabled: true, capabilityVerified: true, targetCount: 1, currentPreviewDigest: digest,
  approvalProjectId: 11, approvalTeamId: 7, approvalResourceType: "connector", approvalResourceId: "8",
  approvalAction: "flighthub.organization.project-member-upsert", approvalStatus: "approved", approvalUnexpired: true,
  approvalPreviewDigest: digest };

test("project member write requires management grant, exact preview, flag, field evidence and approval", () => {
  assert.equal(authorizeProjectMemberWrite(11, input, allowed).capability, "organization.project-member.write");
  for (const override of [{ managementGranted: false }, { featureEnabled: false }, { capabilityVerified: false },
    { targetCount: 0 }, { currentPreviewDigest: "b".repeat(64) }, { approvalStatus: "pending" },
    { approvalUnexpired: false }, { approvalProjectId: 99 }, { approvalResourceId: "9" },
    { approvalAction: "flighthub.organization.write" }, { approvalPreviewDigest: "b".repeat(64) }]) {
    assert.throws(() => authorizeProjectMemberWrite(11, input, { ...allowed, ...override }));
  }
});

test("ordinary project admin, cancelled confirmation and cross-organization target never reach upstream", () => {
  assert.throws(() => authorizeProjectMemberWrite(11, input, { ...allowed, managementGranted: false }));
  assert.throws(() => flightHubProjectMemberWriteInputSchema.parse({ ...input, confirmation: "CANCEL" }));
  assert.throws(() => authorizeProjectMemberWrite(11, input, { ...allowed, targetCount: 0 }));
});

test("preview exposes only irreversible target references", () => {
  assert.equal(preview.members[0]?.reference, flightHubManagementTargetKey(members[0].userId).slice(0, 12));
  assert(!JSON.stringify(preview).includes(members[0].userId));
  const service = readFileSync(new URL("./dji-flighthub-management-write.ts", import.meta.url), "utf8");
  const auditInput = service.slice(service.indexOf("return withAuditedProjectWrite"), service.indexOf("async (client)"));
  assert.doesNotMatch(auditInput, /userId|add_users|input\.members/);
});
