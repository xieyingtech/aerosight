import assert from "node:assert/strict";
import test from "node:test";
import {
  assertAgentToolArgsDoNotContainScope,
  createAgentExecutionContext
} from "./agent-execution-context-core.ts";

test("execution context is immutable and accepts only positive server identifiers", () => {
  const context = createAgentExecutionContext({ userId: 7, teamId: 11, projectId: 17, sessionId: 23 });
  assert.deepEqual(context, { userId: 7, teamId: 11, projectId: 17, sessionId: 23 });
  assert.equal(Object.isFrozen(context), true);
  assert.throws(
    () => createAgentExecutionContext({ userId: 0, teamId: 11, projectId: 17, sessionId: 23 }),
    /AGENT_CONTEXT_INVALID_USERID/
  );
});

test("forged project and user scope in model tool args fail closed", () => {
  assert.throws(
    () => assertAgentToolArgsDoNotContainScope({ projectId: 999, query: "全部告警" }),
    /AGENT_TOOL_SCOPE_ARGUMENT_FORBIDDEN:projectId/
  );
  assert.throws(
    () => assertAgentToolArgsDoNotContainScope({ filters: { user_id: 999 } }),
    /AGENT_TOOL_SCOPE_ARGUMENT_FORBIDDEN:user_id/
  );
  assert.throws(
    () => assertAgentToolArgsDoNotContainScope([{ teamId: 999 }]),
    /AGENT_TOOL_SCOPE_ARGUMENT_FORBIDDEN:teamId/
  );
});

test("ordinary domain identifiers remain valid tool arguments", () => {
  assert.doesNotThrow(() => assertAgentToolArgsDoNotContainScope({ eventId: "evt-1", deviceIds: [3, 5] }));
});
