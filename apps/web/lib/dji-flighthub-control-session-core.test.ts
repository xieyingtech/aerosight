import assert from "node:assert/strict";
import test from "node:test";

import { authorizeControlSession, controlSessionExpiryReason, nextControlLease,
  normalizeControlSelection } from "./dji-flighthub-control-session-core.ts";

test("control selection is canonical and rejects empty or duplicate targets", () => {
  assert.deepEqual(normalizeControlSelection({ flight: true, payloadIndex: ["1-0", "0-0"] }),
    { flight: true, payloadIndex: ["0-0", "1-0"] });
  assert.throws(() => normalizeControlSelection({ flight: false, payloadIndex: [] }));
  assert.throws(() => normalizeControlSelection({ flight: true, payloadIndex: ["0-0", "0-0"] }));
  assert.throws(() => normalizeControlSelection({ flight: true, future: true }));
});

test("a second operator cannot acquire an occupied device", () => {
  const now = new Date("2026-09-02T10:00:00Z");
  const allowed = {
    projectId: 3, teamId: 2, deviceId: 11, connectorProjectId: 3, connectorTeamId: 2, deviceProjectId: 3,
    connectorStatus: "connected", featureEnabled: true, capabilityFieldVerified: true, deviceOnline: true,
    stateCapturedAt: new Date("2026-09-02T09:59:55Z"), now,
    requestedSafetyPolicyVersionId: 8, currentSafetyPolicyVersionId: 8,
    approvalProjectId: 3, approvalTeamId: 2, approvalResourceType: "device", approvalResourceId: "11",
    approvalAction: "flighthub.control.acquire", approvalStatus: "approved", approvalUnexpired: true,
    conflictingSessionCount: 0
  };
  assert.equal(authorizeControlSession(allowed), true);
  assert.throws(() => authorizeControlSession({ ...allowed, conflictingSessionCount: 1 }), /SESSION_CONFLICT/);
});

test("permission revocation and interrupted heartbeat require release", () => {
  const now = new Date("2026-09-02T10:00:20Z");
  const active = { status: "active", now, leaseExpiresAt: new Date("2026-09-02T10:00:30Z"),
    absoluteExpiresAt: new Date("2026-09-02T10:05:00Z"), permissionCurrent: true };
  assert.equal(controlSessionExpiryReason(active), null);
  assert.equal(controlSessionExpiryReason({ ...active, permissionCurrent: false }), "permission_revoked");
  assert.equal(controlSessionExpiryReason({ ...active, leaseExpiresAt: new Date("2026-09-02T10:00:19Z") }), "heartbeat_expired");
  assert.equal(nextControlLease(now, new Date("2026-09-02T10:00:25Z")).toISOString(), "2026-09-02T10:00:25.000Z");
});
