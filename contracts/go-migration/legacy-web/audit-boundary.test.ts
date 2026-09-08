import assert from "node:assert/strict";
import test from "node:test";

import { auditHash, executeWithinAuditBoundary } from "./audit-boundary.ts";

test("audit hash is stable across object key order", () => {
  assert.equal(auditHash({ alpha: 1, beta: 2 }), auditHash({ beta: 2, alpha: 1 }));
});

test("audit failure closes before the protected effect", async () => {
  let effectExecuted = false;
  let rolledBack = false;
  await assert.rejects(
    executeWithinAuditBoundary({
      begin: async () => {},
      writeAudit: async () => { throw new Error("audit unavailable"); },
      execute: async () => { effectExecuted = true; },
      completeAudit: async () => {},
      commit: async () => {},
      rollback: async () => { rolledBack = true; }
    }),
    /audit unavailable/
  );
  assert.equal(effectExecuted, false);
  assert.equal(rolledBack, true);
});
