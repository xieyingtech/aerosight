import assert from "node:assert/strict";
import test from "node:test";

import { decideApproval, type ApprovalRequest } from "./approval-core.ts";

const request: ApprovalRequest = {
  id: "approval-request-1", requestedByUserId: 10, status: "pending",
  requiredApprovals: 1, requireSeparation: true, expiresAt: new Date("2026-08-28T12:00:00Z")
};
const now = new Date("2026-08-28T10:00:00Z");

test("requester cannot approve a separation-of-duty request", () => {
  assert.throws(() => decideApproval(request, [], {
    approverUserId: 10, decision: "approved", reason: "self", decidedAt: now
  }, now), /SELF_DECISION_FORBIDDEN/);
});

test("the same approver cannot decide twice", () => {
  const first = { approverUserId: 20, decision: "approved" as const, reason: "checked", decidedAt: now };
  assert.throws(() => decideApproval({ ...request, requiredApprovals: 2 }, [first], first, now), /DUPLICATE_APPROVER/);
});

test("expired request fails closed", () => {
  assert.throws(() => decideApproval(request, [], {
    approverUserId: 20, decision: "approved", reason: "late", decidedAt: now
  }, new Date("2026-08-28T12:00:00Z")), /EXPIRED/);
});

test("an independent valid approval completes the request", () => {
  const result = decideApproval(request, [], {
    approverUserId: 20, decision: "approved", reason: "preflight reviewed", decidedAt: now
  }, now);
  assert.equal(result.request.status, "approved");
  assert.equal(result.decisions.length, 1);
});

test("multi-approver request stays pending until its threshold", () => {
  const twoPerson = { ...request, requiredApprovals: 2 };
  const first = decideApproval(twoPerson, [], {
    approverUserId: 20, decision: "approved", reason: "first", decidedAt: now
  }, now);
  assert.equal(first.request.status, "pending");
  const second = decideApproval(first.request, first.decisions, {
    approverUserId: 21, decision: "approved", reason: "second", decidedAt: now
  }, now);
  assert.equal(second.request.status, "approved");
});
