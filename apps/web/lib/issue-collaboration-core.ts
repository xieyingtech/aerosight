export type IssueMutation =
  | { action: "comment"; body: string }
  | { action: "status"; status: "open" | "closed" }
  | { action: "labels"; labels: string[] }
  | { action: "assign" | "unassign"; assigneeType: "user" | "agent"; assigneeId: number };

export function planIssueMutation(input: {
  mutation: IssueMutation;
  permissions: ReadonlySet<string>;
  actualVersion: number;
  expectedVersion: number;
}) {
  const assignment = input.mutation.action === "assign" || input.mutation.action === "unassign";
  const permission = assignment ? "issue:assign" : "issue:handle";
  if (!input.permissions.has(permission)) throw new Error("PROJECT_ACCESS_DENIED");
  if (input.actualVersion !== input.expectedVersion) throw new Error("ISSUE_VERSION_CONFLICT");
  if (input.mutation.action === "comment") {
    const body = input.mutation.body.trim();
    if (!body || body.length > 5000) throw new Error("ISSUE_COMMENT_INVALID");
    return { eventType: "comment.created", body, metadata: {}, nextVersion: input.actualVersion + 1 };
  }
  if (input.mutation.action === "status") {
    return { eventType: "status.changed", body: null, metadata: { status: input.mutation.status }, status: input.mutation.status, nextVersion: input.actualVersion + 1 };
  }
  if (input.mutation.action === "labels") {
    const labels = [...new Set(input.mutation.labels.map((label) => label.trim()).filter(Boolean))];
    if (labels.length > 20 || labels.some((label) => label.length > 50)) throw new Error("ISSUE_LABELS_INVALID");
    return { eventType: "labels.changed", body: null, metadata: { labels }, labels, nextVersion: input.actualVersion + 1 };
  }
  if (!Number.isSafeInteger(input.mutation.assigneeId) || input.mutation.assigneeId <= 0) throw new Error("ISSUE_ASSIGNEE_INVALID");
  return {
    eventType: input.mutation.action === "assign" ? "assignee.added" : "assignee.removed", body: null,
    metadata: { assigneeType: input.mutation.assigneeType, assigneeId: input.mutation.assigneeId },
    assignment: input.mutation, nextVersion: input.actualVersion + 1
  };
}
