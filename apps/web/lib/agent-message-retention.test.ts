import assert from "node:assert/strict";
import test from "node:test";
import { assertAgentSessionScope, sanitizeAgentMessageForStorage } from "./agent-message-retention.ts";

test("temporary URLs and secrets never enter agent message history", () => {
  const stored = sanitizeAgentMessageForStorage({
    content: "查看 https://media.example/a.jpg?expires=99&signature=abc 使用 sk-secretcredential123",
    toolCalls: [{
      name: "query_assets", status: "succeeded", summary: "Authorization: secret-token",
      rawResult: { bytes: "must-not-persist" }, apiKey: "must-not-persist",
      evidenceRefs: [{ type: "asset", id: "4", version: "sha256:abc", href: "https://media.example/a?token=secret" }]
    }]
  });
  assert.equal(JSON.stringify(stored).includes("must-not-persist"), false);
  assert.equal(JSON.stringify(stored).includes("secretcredential123"), false);
  assert.equal(JSON.stringify(stored).includes("secret-token"), false);
  assert.equal(JSON.stringify(stored).includes("token=secret"), false);
  assert.deepEqual(stored.toolCalls[0].evidenceRefs, [{ type: "asset", id: "4", version: "sha256:abc" }]);
});

test("cross-project and cross-user session access is indistinguishable", () => {
  const session = { projectId: 17, startedByUserId: 7 };
  assert.throws(() => assertAgentSessionScope({ projectId: 18, userId: 7 }, session), /AGENT_SESSION_NOT_FOUND/);
  assert.throws(() => assertAgentSessionScope({ projectId: 17, userId: 8 }, session), /AGENT_SESSION_NOT_FOUND/);
  assert.doesNotThrow(() => assertAgentSessionScope({ projectId: 17, userId: 7 }, session));
});
