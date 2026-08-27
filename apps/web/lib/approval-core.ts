export type ApprovalRequest = {
  id: string;
  requestedByUserId: number;
  status: "pending" | "approved" | "rejected" | "expired";
  requiredApprovals: number;
  requireSeparation: boolean;
  expiresAt: Date;
};

export type ApprovalDecision = {
  approverUserId: number;
  decision: "approved" | "rejected";
  reason: string;
  decidedAt: Date;
};

export function decideApproval(
  request: ApprovalRequest,
  decisions: ApprovalDecision[],
  input: ApprovalDecision,
  now = new Date()
): { request: ApprovalRequest; decisions: ApprovalDecision[] } {
  if (request.status !== "pending") throw new Error("APPROVAL_REQUEST_NOT_PENDING");
  if (request.expiresAt <= now) throw new Error("APPROVAL_REQUEST_EXPIRED");
  if (request.requireSeparation && request.requestedByUserId === input.approverUserId) {
    throw new Error("APPROVAL_SELF_DECISION_FORBIDDEN");
  }
  if (decisions.some((decision) => decision.approverUserId === input.approverUserId)) {
    throw new Error("APPROVAL_DUPLICATE_APPROVER");
  }
  const nextDecisions = [...decisions, input];
  const status = input.decision === "rejected"
    ? "rejected"
    : nextDecisions.filter((decision) => decision.decision === "approved").length >= request.requiredApprovals
      ? "approved" : "pending";
  return { request: { ...request, status }, decisions: nextDecisions };
}
