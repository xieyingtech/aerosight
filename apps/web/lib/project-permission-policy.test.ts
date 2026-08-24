import assert from "node:assert/strict";
import test from "node:test";

import {
  effectiveProjectPermissions,
  PROJECT_PERMISSIONS
} from "./project-permission-policy.ts";

test("owner and admin receive the complete project permission set", () => {
  assert.deepEqual([...effectiveProjectPermissions("owner")].sort(), [...PROJECT_PERMISSIONS].sort());
  assert.deepEqual([...effectiveProjectPermissions("admin")].sort(), [...PROJECT_PERMISSIONS].sort());
});

test("member receives view plus only explicitly granted permissions", () => {
  const permissions = effectiveProjectPermissions("member", ["event:handle", "unknown:permission"]);
  assert(permissions.has("project:view"));
  assert(permissions.has("event:handle"));
  assert(!permissions.has("mission:operate"));
  assert(!permissions.has("mission:approve"));
  assert.equal(permissions.size, 2);
});

test("one explicit permission never expands to an adjacent capability", () => {
  const permissions = effectiveProjectPermissions("member", ["agent:use"]);
  assert(permissions.has("agent:use"));
  assert(!permissions.has("algorithm:manage"));
  assert(!permissions.has("device:configure"));
});
