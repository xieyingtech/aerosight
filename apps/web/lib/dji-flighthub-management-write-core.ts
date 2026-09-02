import { createHash } from "node:crypto";
import { z } from "zod";

const member = z.object({
  userId: z.string().trim().min(1).max(256).refine((value) => !/[\0\r\n]/.test(value)),
  role: z.enum(["project-member", "project-admin"]),
  nickname: z.string().trim().max(128).refine((value) => !/[\0\r\n]/.test(value)),
}).strict();

export const flightHubProjectMemberPreviewInputSchema = z.object({
  members: z.array(member).min(1).max(100),
}).strict().superRefine((value, context) => {
  const seen = new Set<string>();
  value.members.forEach((item, index) => {
    if (seen.has(item.userId)) context.addIssue({ code: "custom", path: ["members", index, "userId"], message: "duplicate user" });
    seen.add(item.userId);
  });
});

export const flightHubProjectMemberWriteInputSchema = z.object({
  connectorInstanceId: z.number().int().positive(),
  members: z.array(member).min(1).max(100),
  confirmation: z.literal("ADD PROJECT MEMBER"),
  previewDigest: z.string().regex(/^[a-f0-9]{64}$/),
  approvalRequestId: z.string().uuid(),
  idempotencyKey: z.string().trim().min(8).max(200),
}).strict().superRefine((value, context) => {
  const seen = new Set<string>();
  value.members.forEach((item, index) => {
    if (seen.has(item.userId)) context.addIssue({ code: "custom", path: ["members", index, "userId"], message: "duplicate user" });
    seen.add(item.userId);
  });
});

export type FlightHubProjectMemberPreviewInput = z.infer<typeof flightHubProjectMemberPreviewInputSchema>;
export type FlightHubProjectMemberWriteInput = z.infer<typeof flightHubProjectMemberWriteInputSchema>;

export function bindProjectMemberWriteRequest(connectorInstanceId: number, rawInput: unknown) {
  const raw = rawInput && typeof rawInput === "object" && !Array.isArray(rawInput) ? rawInput as Record<string, unknown> : {};
  const { connectorInstanceId: _ignored, ...untrusted } = raw;
  return flightHubProjectMemberWriteInputSchema.parse({ ...untrusted, connectorInstanceId });
}

export const PROJECT_MEMBER_WRITE_POLICY = Object.freeze({
  capability: "organization.project-member.write",
  featureFlag: "flighthub.organization.project-member",
  approvalAction: "flighthub.organization.project-member-upsert",
});

export function flightHubManagementTargetKey(value: string) {
  return createHash("sha256").update(value.trim()).digest("hex").slice(0, 32);
}

export function projectMemberPreview(input: { projectId: number; connectorInstanceId: number; projectName: string;
  organizationName: string; members: FlightHubProjectMemberPreviewInput["members"] }) {
  return Object.freeze({
    projectId: input.projectId,
    connectorInstanceId: input.connectorInstanceId,
    projectName: input.projectName.slice(0, 256),
    organizationName: input.organizationName.slice(0, 256),
    members: input.members.map((item) => Object.freeze({
      reference: flightHubManagementTargetKey(item.userId).slice(0, 12), role: item.role, nickname: item.nickname,
    })),
    impact: "add-or-update-project-members",
  });
}

export function authorizeProjectMemberWrite(projectId: number, input: FlightHubProjectMemberWriteInput,
  authorization: { teamId: number; managementGranted: boolean; connectorProjectId: number; connectorTeamId: number;
    connectorStatus: string; featureEnabled: boolean; capabilityVerified: boolean; targetCount: number;
    currentPreviewDigest: string; approvalProjectId: number | null; approvalTeamId: number | null;
    approvalResourceType: string | null; approvalResourceId: string | null; approvalAction: string | null;
    approvalStatus: string | null; approvalUnexpired: boolean; approvalPreviewDigest: string | null }) {
  if (!authorization.managementGranted) throw new Error("FLIGHTHUB_MANAGEMENT_WRITE_PERMISSION_DENIED");
  if (authorization.connectorProjectId !== projectId || authorization.connectorTeamId !== authorization.teamId) {
    throw new Error("FLIGHTHUB_MANAGEMENT_WRITE_SCOPE_MISMATCH");
  }
  if (authorization.connectorStatus !== "connected") throw new Error("FLIGHTHUB_MANAGEMENT_WRITE_CONNECTOR_OFFLINE");
  if (!authorization.featureEnabled || !authorization.capabilityVerified) throw new Error("FLIGHTHUB_MANAGEMENT_WRITE_DISABLED");
  if (authorization.targetCount !== input.members.length || authorization.currentPreviewDigest !== input.previewDigest) {
    throw new Error("FLIGHTHUB_MANAGEMENT_WRITE_TARGET_MISMATCH");
  }
  if (authorization.approvalProjectId !== projectId || authorization.approvalTeamId !== authorization.teamId
      || authorization.approvalResourceType !== "connector"
      || authorization.approvalResourceId !== String(input.connectorInstanceId)
      || authorization.approvalAction !== PROJECT_MEMBER_WRITE_POLICY.approvalAction
      || authorization.approvalStatus !== "approved" || !authorization.approvalUnexpired
      || authorization.approvalPreviewDigest !== input.previewDigest) {
    throw new Error("FLIGHTHUB_MANAGEMENT_WRITE_APPROVAL_REQUIRED");
  }
  return Object.freeze({ projectId, teamId: authorization.teamId, ...PROJECT_MEMBER_WRITE_POLICY });
}
