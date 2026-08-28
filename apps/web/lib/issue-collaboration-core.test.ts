import assert from "node:assert/strict";
import test from "node:test";
import { planIssueMutation } from "./issue-collaboration-core.ts";

test("issue handling permission and optimistic version are required", () => {
  assert.throws(() => planIssueMutation({ mutation: { action: "comment", body: "复核" }, permissions: new Set(), actualVersion: 2, expectedVersion: 2 }), /PROJECT_ACCESS_DENIED/);
  assert.throws(() => planIssueMutation({ mutation: { action: "comment", body: "复核" }, permissions: new Set(["issue:handle"]), actualVersion: 2, expectedVersion: 1 }), /ISSUE_VERSION_CONFLICT/);
});

test("comments and labels are normalized without widening permissions", () => {
  const comment = planIssueMutation({ mutation: { action: "comment", body: "  请复核原图  " }, permissions: new Set(["issue:handle"]), actualVersion: 0, expectedVersion: 0 });
  assert.equal(comment.body, "请复核原图");
  const labels = planIssueMutation({ mutation: { action: "labels", labels: [" 违建 ", "违建", "高风险"] }, permissions: new Set(["issue:handle"]), actualVersion: 1, expectedVersion: 1 });
  assert.deepEqual(labels.metadata, { labels: ["违建", "高风险"] });
  assert.throws(() => planIssueMutation({ mutation: { action: "assign", assigneeType: "agent", assigneeId: 3 }, permissions: new Set(["issue:handle"]), actualVersion: 1, expectedVersion: 1 }), /PROJECT_ACCESS_DENIED/);
});

test("user and agent assignment use one typed plan", () => {
  for (const assigneeType of ["user", "agent"] as const) {
    const result = planIssueMutation({ mutation: { action: "assign", assigneeType, assigneeId: 7 }, permissions: new Set(["issue:assign"]), actualVersion: 4, expectedVersion: 4 });
    assert.equal(result.eventType, "assignee.added");
    assert.deepEqual(result.metadata, { assigneeType, assigneeId: 7 });
  }
});
