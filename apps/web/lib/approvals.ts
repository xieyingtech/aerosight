import "server-only";

import { randomUUID } from "node:crypto";

import { withAuditedProjectWrite } from "@/lib/audit";
import { requireCurrentProjectPermission } from "@/lib/data";
import { correlationId } from "@/lib/observability";

export async function requestProjectApproval(input: {
  projectId: number;
  resourceType: string;
  resourceId: string;
  action: string;
  requiredApprovals?: number;
  requireSeparation?: boolean;
  expiresInMinutes: number;
  context?: Record<string, unknown>;
  requestId?: string | null;
}) {
  const { user, access } = await requireCurrentProjectPermission(input.projectId, "mission:operate");
  if (!input.resourceType.trim() || !input.resourceId.trim() || !input.action.trim()) throw new Error("APPROVAL_TARGET_REQUIRED");
  if (!Number.isInteger(input.expiresInMinutes) || input.expiresInMinutes < 1 || input.expiresInMinutes > 24 * 60) {
    throw new Error("APPROVAL_EXPIRY_INVALID");
  }
  const id = randomUUID();
  return withAuditedProjectWrite({
    projectId: input.projectId, teamId: access.teamId, requestId: correlationId(input.requestId),
    actorUserId: user.id, action: "approval.request", resourceType: input.resourceType,
    resourceId: input.resourceId, input, policyResult: { permission: "mission:operate" }
  }, async (client) => (await client.query(
    `insert into approval_requests (
       id, project_id, team_id, resource_type, resource_id, action, requested_by_user_id,
       required_approvals, require_separation, expires_at, context_json
     ) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, now() + ($10 * interval '1 minute'), $11)
     returning id, status, expires_at as "expiresAt", required_approvals as "requiredApprovals"`,
    [id, input.projectId, access.teamId, input.resourceType, input.resourceId, input.action, user.id,
      input.requiredApprovals ?? 1, input.requireSeparation ?? true, input.expiresInMinutes, input.context ?? {}]
  )).rows[0]);
}

export async function decideProjectApproval(input: {
  projectId: number;
  approvalRequestId: string;
  decision: "approved" | "rejected";
  reason: string;
  requestId?: string | null;
}) {
  const { user, access } = await requireCurrentProjectPermission(input.projectId, "mission:approve");
  if (!input.reason.trim()) throw new Error("APPROVAL_REASON_REQUIRED");
  return withAuditedProjectWrite({
    projectId: input.projectId, teamId: access.teamId, requestId: correlationId(input.requestId),
    actorUserId: user.id, action: `approval.${input.decision}`, resourceType: "approval_request",
    resourceId: input.approvalRequestId, input, policyResult: { permission: "mission:approve" }
  }, async (client) => {
    const inserted = await client.query(
      `insert into approvals (
         project_id, team_id, approval_request_id, approver_user_id, decision, reason
       ) values ($1, $2, $3, $4, $5, $6)
       returning id, decision, decided_at as "decidedAt"`,
      [input.projectId, access.teamId, input.approvalRequestId, user.id, input.decision, input.reason]
    );
    const request = await client.query(
      `select id, status, required_approvals as "requiredApprovals", expires_at as "expiresAt"
         from approval_requests where project_id = $1 and id = $2`,
      [input.projectId, input.approvalRequestId]
    );
    if (!request.rows[0]) throw new Error("APPROVAL_REQUEST_NOT_FOUND");
    return { decision: inserted.rows[0], request: request.rows[0] };
  });
}
