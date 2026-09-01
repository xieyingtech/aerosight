import assert from "node:assert/strict";
import test from "node:test";

import { auditHash, executeWithinAuditBoundary } from "./audit-boundary.ts";

test("audit hash is stable across object key order", () => {
  assert.equal(auditHash({ alpha: 1, beta: 2 }), auditHash({ beta: 2, alpha: 1 }));
});

test("audit persistence stores only an irreversible digest of FlightHub secrets", () => {
  const secrets = ["organization-token-plaintext", "plain-serial-from-decrypt", "temporary-security-token"];
  const digest = auditHash({ token: secrets[0], mapping: { encrypted: secrets[1] }, storage_sts: { security_token: secrets[2] } });
  assert.match(digest, /^[a-f0-9]{64}$/);
  for (const secret of secrets) assert(!digest.includes(secret));
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
